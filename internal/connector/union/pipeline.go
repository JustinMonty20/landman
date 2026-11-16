package union

import (
	"archive/zip"
	"bytes"
	"compress/zlib"
	"context"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/JustinMonty20/landman/internal/taxartifact"
	"github.com/JustinMonty20/landman/internal/taxsource"
)

const pipelineSource = "union_foreclosure_pipeline"
const pipelineLimit = 64 << 20

// PipelineProperty describes one parcel on the dated upcoming foreclosure list.
type PipelineProperty struct {
	ParcelKey, OwnerName, SitusLine, Township, Acreage, CaseNumber, ScheduleText, Status string
	TaxValue                                                                             *int64
	SaleOn                                                                               *time.Time
	Redeemed                                                                             bool
}

type PipelineList struct {
	path, sourceURL string
	store           taxartifact.Store
}

var _ taxsource.Source[PipelineProperty] = (*PipelineList)(nil)

func NewPipelineList(path, sourceURL string, store taxartifact.Store) (*PipelineList, error) {
	u, err := url.Parse(sourceURL)
	if path == "" || store == nil || err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return nil, errors.New("pipeline path, HTTPS source URL, and artifact store required")
	}
	return &PipelineList{path, sourceURL, store}, nil
}

// Read retains exact operator-supplied bytes and validates the whole file before yielding.
func (s *PipelineList) Read(ctx context.Context, yield func(PipelineProperty) error) (taxsource.Meta, error) {
	if yield == nil {
		return taxsource.Meta{}, errors.New("pipeline yield required")
	}
	if err := ctx.Err(); err != nil {
		return taxsource.Meta{}, err
	}
	file, err := os.Open(s.path)
	if err != nil {
		return taxsource.Meta{}, fmt.Errorf("open operator-supplied pipeline file: %w", err)
	}
	info, statErr := file.Stat()
	if statErr != nil {
		return taxsource.Meta{}, errors.Join(statErr, file.Close())
	}
	if info.Size() > pipelineLimit {
		return taxsource.Meta{}, errors.Join(errors.New("pipeline file exceeds size limit"), file.Close())
	}
	artifact, putErr := s.store.Put(ctx, pipelineSource, io.LimitReader(file, pipelineLimit+1))
	if err := errors.Join(putErr, file.Close()); err != nil {
		return taxsource.Meta{}, err
	}
	meta := taxsource.Meta{URL: s.sourceURL, Name: pipelineSource, SHA256: artifact.SHA256(), FetchedAt: time.Now().UTC(), Artifact: artifact}
	if artifact.Size() > pipelineLimit {
		return meta, errors.New("pipeline file exceeds size limit")
	}
	rows, skipped, err := s.parse(ctx, artifact)
	if err != nil {
		return meta, err
	}
	_ = skipped
	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return meta, err
		}
		if err := yield(row); err != nil {
			return meta, err
		}
	}
	return meta, nil
}

func (s *PipelineList) Skipped(ctx context.Context, meta taxsource.Meta) (int, error) {
	if !meta.Artifact.Verified() || meta.Name != pipelineSource || meta.URL != s.sourceURL {
		return 0, errors.New("verified pipeline artifact required")
	}
	_, skipped, err := s.parse(ctx, meta.Artifact)
	return skipped, err
}

func (s *PipelineList) parse(ctx context.Context, artifact taxartifact.Artifact) ([]PipelineProperty, int, error) {
	body, err := s.store.Get(ctx, artifact)
	if err != nil {
		return nil, 0, err
	}
	data, readErr := io.ReadAll(io.LimitReader(body, pipelineLimit+1))
	if err := errors.Join(readErr, body.Close()); err != nil {
		return nil, 0, err
	}
	if len(data) > pipelineLimit {
		return nil, 0, errors.New("pipeline file exceeds size limit")
	}
	var pages [][]string
	switch {
	case bytes.HasPrefix(data, []byte("%PDF")):
		pages, err = pipelinePDF(ctx, data)
	case bytes.HasPrefix(data, []byte("PK")):
		pages, err = pipelinePPTX(ctx, data)
	default:
		err = errors.New("pipeline file is neither PDF nor presentation ZIP")
	}
	if err != nil {
		return nil, 0, err
	}
	var rows []PipelineProperty
	skipped := 0
	for _, lines := range pages {
		if err := ctx.Err(); err != nil {
			return nil, 0, err
		}
		parsed, err := pipelineProperties(lines)
		if err != nil {
			return nil, 0, err
		}
		if len(parsed) == 0 {
			skipped++
		}
		rows = append(rows, parsed...)
	}
	return rows, skipped, nil
}

var pdfObject = regexp.MustCompile(`(?s)(\d+)\s+\d+\s+obj\b(.*?)endobj`)
var pdfContents = regexp.MustCompile(`/Contents\s+(?:\[((?:\s*\d+\s+\d+\s+R)+)\]|(\d+\s+\d+\s+R))`)
var pdfReference = regexp.MustCompile(`(\d+)\s+\d+\s+R`)
var pdfPage = regexp.MustCompile(`/Type\s*/Page\b`)
var pdfOperand = `(?:\[(?:[^\]]|\\.)*\]|\((?:\\.|[^\\)])*\)|<[0-9A-Fa-f\s]+>)`
var pdfShow = regexp.MustCompile(`(?s)(` + pdfOperand + `)\s*(TJ|Tj|'|")`)
var pdfTextToken = regexp.MustCompile(`(?s)` + pdfOperand + `\s*(?:TJ|Tj|'|")|(?:[-+\d.]+\s+){2}(?:Td|TD)\b|(?:[-+\d.]+\s+){6}Tm\b|T\*\b`)
var pdfString = regexp.MustCompile(`\((?:\\.|[^\\)])*\)|<[0-9A-Fa-f\s]+>`)
var pdfTJSpacing = regexp.MustCompile(`^(?:\s*[-+]?(?:\d+(?:\.\d*)?|\.\d+)\s*)*$`)
var pdfRoot = regexp.MustCompile(`/Root\s+(\d+)\s+\d+\s+R`)
var pdfPagesRef = regexp.MustCompile(`/Pages\s+(\d+)\s+\d+\s+R`)
var pdfLength = regexp.MustCompile(`/Length\s+(\d+)\b`)
var pdfAlternative = regexp.MustCompile(`/(?:ActualText|Alt)\s*(\((?:\\.|[^\\)])*\)|<([0-9A-Fa-f]+)>)`)
var pdfAlternativeName = regexp.MustCompile(`/(?:ActualText|Alt)\b`)
var pdfStartXref = regexp.MustCompile(`(?s)startxref\s+(\d+)\s+%%EOF\s*$`)
var pdfShowingOperator = regexp.MustCompile(`(?:\s|\)|\]|>)(?:Tj|TJ)\b`)

func pipelinePDF(ctx context.Context, data []byte) ([][]string, error) {
	start := pdfStartXref.FindSubmatch(data)
	if start == nil {
		return nil, errors.New("invalid PDF structure")
	}
	offset, err := strconv.Atoi(string(start[1]))
	if err != nil || offset < 0 || offset >= len(data) || !(bytes.HasPrefix(data[offset:], []byte("xref\n")) || bytes.HasPrefix(data[offset:], []byte("xref\r\n"))) || !bytes.Contains(data[offset:], []byte("trailer")) {
		return nil, errors.New("invalid PDF cross-reference")
	}
	objects := map[string][]byte{}
	order := []string{}
	for _, match := range pdfObject.FindAllSubmatch(data, -1) {
		key := string(match[1])
		objects[key] = match[2]
		order = append(order, key)
	}
	root := pdfRoot.FindSubmatch(data[bytes.LastIndex(data, []byte("trailer")):])
	if root == nil || !bytes.Contains(objects[string(root[1])], []byte("/Type /Catalog")) {
		return nil, errors.New("invalid PDF catalog")
	}
	pagesRef := pdfPagesRef.FindSubmatch(objects[string(root[1])])
	if pagesRef == nil || !bytes.Contains(objects[string(pagesRef[1])], []byte("/Type /Pages")) {
		return nil, errors.New("invalid PDF page tree")
	}
	var pages [][]string
	for _, id := range order {
		if !pdfPage.Match(objects[id]) {
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		ref := pdfContents.FindSubmatch(objects[id])
		if ref == nil {
			return nil, errors.New("PDF page missing content stream")
		}
		refs := pdfReference.FindAllSubmatch(ref[0], -1)
		var lines []string
		for _, reference := range refs {
			stream := objects[string(reference[1])]
			start := bytes.Index(stream, []byte("stream"))
			end := bytes.LastIndex(stream, []byte("endstream"))
			if start < 0 || end <= start {
				return nil, errors.New("invalid PDF page content stream")
			}
			content := bytes.TrimPrefix(stream[start+6:end], []byte("\r\n"))
			content = bytes.TrimPrefix(content, []byte("\n"))
			length := pdfLength.FindSubmatch(stream[:start])
			if length == nil {
				return nil, errors.New("PDF content stream missing length")
			}
			want, err := strconv.Atoi(string(length[1]))
			if err != nil || want < 0 || want > pipelineLimit || len(content) < want || len(bytes.TrimSpace(content[want:])) != 0 {
				return nil, errors.New("invalid PDF content length")
			}
			content = content[:want]
			if bytes.Contains(stream[:start], []byte("/Filter")) && !bytes.Contains(stream[:start], []byte("/FlateDecode")) {
				return nil, errors.New("unsupported PDF content filter")
			}
			if bytes.Contains(stream[:start], []byte("/FlateDecode")) {
				decoder, err := zlib.NewReader(bytes.NewReader(content))
				if err != nil {
					return nil, err
				}
				decoded, err := io.ReadAll(io.LimitReader(decoder, pipelineLimit+1))
				closeErr := decoder.Close()
				if err := errors.Join(err, closeErr); err != nil {
					return nil, err
				}
				if len(decoded) > pipelineLimit {
					return nil, errors.New("PDF content exceeds limit")
				}
				content = decoded
			}
			var line strings.Builder
			flush := func() {
				if value := strings.TrimSpace(line.String()); value != "" {
					lines = append(lines, value)
				}
				line.Reset()
			}
			if len(pdfShowingOperator.FindAll(content, -1)) > len(pdfShow.FindAll(content, -1)) || len(pdfAlternativeName.FindAll(content, -1)) > len(pdfAlternative.FindAll(content, -1)) {
				return nil, errors.New("unsupported PDF text operand")
			}
			for _, token := range pdfTextToken.FindAll(content, -1) {
				match := pdfShow.FindSubmatch(token)
				if match == nil {
					flush()
					continue
				}
				if string(match[2]) == "'" || string(match[2]) == "\"" {
					flush()
				}
				operand := match[1]
				parts := pdfString.FindAllIndex(operand, -1)
				if len(parts) == 0 {
					return nil, errors.New("unsupported PDF text operand")
				}
				if operand[0] == '[' {
					inner := operand[1 : len(operand)-1]
					previous := 0
					for _, part := range parts {
						begin, end := part[0]-1, part[1]-1
						if !pdfTJSpacing.Match(inner[previous:begin]) {
							return nil, errors.New("unsupported PDF text array")
						}
						previous = end
					}
					if !pdfTJSpacing.Match(inner[previous:]) {
						return nil, errors.New("unsupported PDF text array")
					}
				} else if len(parts) != 1 || parts[0][0] != 0 || parts[0][1] != len(operand) {
					return nil, errors.New("unsupported PDF text operand")
				}
				for _, part := range parts {
					value, err := decodePDFText(operand[part[0]:part[1]])
					if err != nil {
						return nil, err
					}
					line.WriteString(value)
				}
			}
			flush()
			for _, match := range pdfAlternative.FindAllSubmatch(content, -1) {
				value, err := decodePDFText(match[1])
				if err != nil {
					return nil, err
				}
				lines = append(lines, value)
			}
		}
		pages = append(pages, lines)
	}
	if len(pages) == 0 {
		return nil, errors.New("PDF has no pages")
	}
	return pages, nil
}
func decodePDFText(operand []byte) (string, error) {
	if len(operand) < 2 {
		return "", errors.New("unsupported PDF text operand")
	}
	var raw []byte
	if operand[0] == '(' && operand[len(operand)-1] == ')' {
		raw = []byte(pdfDecode(operand[1 : len(operand)-1]))
	} else if operand[0] == '<' && operand[len(operand)-1] == '>' {
		encoded := bytes.Join(bytes.Fields(operand[1:len(operand)-1]), nil)
		if len(encoded)%2 == 1 {
			encoded = append(encoded, '0')
		}
		var err error
		raw, err = hex.DecodeString(string(encoded))
		if err != nil {
			return "", err
		}
	} else {
		return "", errors.New("unsupported PDF text operand")
	}
	if len(raw) >= 2 && raw[0] == 0xfe && raw[1] == 0xff {
		if len(raw)%2 != 0 {
			return "", errors.New("invalid PDF UTF-16 text")
		}
		units := make([]uint16, 0, (len(raw)-2)/2)
		for i := 2; i < len(raw); i += 2 {
			units = append(units, uint16(raw[i])<<8|uint16(raw[i+1]))
		}
		return string(utf16.Decode(units)), nil
	}
	if !utf8.Valid(raw) {
		return "", errors.New("unsupported PDF text encoding")
	}
	return string(raw), nil
}
func pdfDecode(raw []byte) string {
	var b strings.Builder
	for i := 0; i < len(raw); i++ {
		if raw[i] != '\\' || i+1 == len(raw) {
			b.WriteByte(raw[i])
			continue
		}
		i++
		switch raw[i] {
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case '\n':
		case '\r':
			if i+1 < len(raw) && raw[i+1] == '\n' {
				i++
			}
		default:
			if raw[i] >= '0' && raw[i] <= '7' {
				end := i + 1
				for end < len(raw) && end < i+3 && raw[end] >= '0' && raw[end] <= '7' {
					end++
				}
				value, _ := strconv.ParseUint(string(raw[i:end]), 8, 8)
				b.WriteByte(byte(value))
				i = end - 1
			} else {
				b.WriteByte(raw[i])
			}
		}
	}
	return b.String()
}

var slideFile = regexp.MustCompile(`^ppt/slides/slide[0-9]+\.xml$`)
var noteFile = regexp.MustCompile(`^ppt/notesSlides/notesSlide[0-9]+\.xml$`)

func pipelinePPTX(ctx context.Context, data []byte) ([][]string, error) {
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	slides := map[string][]string{}
	notes := map[string][]string{}
	valid, presentation := false, false
	for _, file := range archive.File {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if file.Name == "[Content_Types].xml" {
			b, e := readPipelineEntry(file)
			if e != nil {
				return nil, e
			}
			var types struct {
				XMLName   xml.Name `xml:"Types"`
				Overrides []struct {
					PartName    string `xml:"PartName,attr"`
					ContentType string `xml:"ContentType,attr"`
				} `xml:"Override"`
			}
			if err := xml.Unmarshal(b, &types); err != nil || types.XMLName.Local != "Types" {
				return nil, errors.New("invalid PPTX content types XML")
			}
			for _, item := range types.Overrides {
				if item.PartName == "/ppt/presentation.xml" && item.ContentType == "application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml" {
					valid = true
				}
			}
			continue
		}
		if file.Name == "ppt/presentation.xml" {
			b, e := readPipelineEntry(file)
			if e != nil {
				return nil, e
			}
			var doc struct{ XMLName xml.Name }
			if e := xml.Unmarshal(b, &doc); e != nil || doc.XMLName.Local != "presentation" || doc.XMLName.Space != "http://schemas.openxmlformats.org/presentationml/2006/main" {
				return nil, errors.New("invalid PPTX presentation XML")
			}
			presentation = true
			continue
		}
		if !slideFile.MatchString(file.Name) && !noteFile.MatchString(file.Name) {
			continue
		}
		b, e := readPipelineEntry(file)
		if e != nil {
			return nil, e
		}
		expected := "notes"
		if slideFile.MatchString(file.Name) {
			expected = "sld"
		}
		lines, e := pipelineXML(b, expected)
		if e != nil {
			return nil, e
		}
		if slideFile.MatchString(file.Name) {
			slides[file.Name] = lines
		} else {
			notes[file.Name] = lines
		}
	}
	if !valid || !presentation || len(slides) == 0 {
		return nil, errors.New("ZIP is not a PPTX presentation")
	}
	var pages [][]string
	for i := 1; i <= len(slides); i++ {
		name := fmt.Sprintf("ppt/slides/slide%d.xml", i)
		lines, ok := slides[name]
		if !ok {
			return nil, errors.New("nonconsecutive PPTX slides")
		}
		lines = append(lines, notes[fmt.Sprintf("ppt/notesSlides/notesSlide%d.xml", i)]...)
		pages = append(pages, lines)
	}
	return pages, nil
}
func readPipelineEntry(file *zip.File) ([]byte, error) {
	if file.UncompressedSize64 > pipelineLimit {
		return nil, errors.New("PPTX entry exceeds limit")
	}
	r, e := file.Open()
	if e != nil {
		return nil, e
	}
	b, e := io.ReadAll(io.LimitReader(r, pipelineLimit+1))
	if err := errors.Join(e, r.Close()); err != nil {
		return nil, err
	}
	if len(b) > pipelineLimit {
		return nil, errors.New("PPTX entry exceeds limit")
	}
	return b, nil
}
func pipelineXML(data []byte, expected string) ([]string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	root := false
	var lines []string
	var paragraph strings.Builder
	inParagraph := false
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			if !root {
				return nil, errors.New("missing PPTX slide XML")
			}
			return lines, nil
		}
		if err != nil {
			return nil, err
		}
		switch element := token.(type) {
		case xml.StartElement:
			if !root {
				if element.Name.Local != expected || element.Name.Space != "http://schemas.openxmlformats.org/presentationml/2006/main" {
					return nil, errors.New("invalid PPTX slide XML")
				}
				root = true
			}
			switch element.Name.Local {
			case "p":
				inParagraph = true
				paragraph.Reset()
			case "cNvPr":
				for _, a := range element.Attr {
					if (a.Name.Local == "descr" || a.Name.Local == "title") && a.Value != "" {
						lines = append(lines, a.Value)
					}
				}
			case "t":
				var value string
				if err := decoder.DecodeElement(&value, &element); err != nil {
					return nil, err
				}
				if inParagraph {
					paragraph.WriteString(value)
				} else {
					lines = append(lines, value)
				}
			}
		case xml.EndElement:
			if element.Name.Local == "p" && inParagraph {
				if text := strings.TrimSpace(paragraph.String()); text != "" {
					lines = append(lines, text)
				}
				inParagraph = false
			}
		}
	}
}

var parcelToken = regexp.MustCompile(`(?i)\b(?:\d{2}[.\-])?\d{3}[.\-]\d{3}[A-Z0-9]*(?: +\d{1,3})?\b|\b\d{8}[A-Z0-9]*(?: +\d{1,3})?\b`)
var moneyPipeline = regexp.MustCompile(`^\d+\.\d{2}$`)
var fullDatePipeline = regexp.MustCompile(`(?i)\b(January|February|March|April|May|June|July|August|September|October|November|December)\s+\d{1,2},?\s+\d{4}\b`)

func pipelineProperties(lines []string) ([]PipelineProperty, error) {
	var rows []PipelineProperty
	var property PipelineProperty
	groupStart, hasFields := 0, false
	finish := func() {
		for i := groupStart; i < len(rows); i++ {
			key := rows[i].ParcelKey
			rows[i] = property
			rows[i].ParcelKey = key
		}
	}
	for _, line := range lines {
		for _, part := range strings.Split(line, "\n") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			lower := strings.ToLower(part)
			if strings.Contains(lower, "parcel") {
				tokens := parcelToken.FindAllString(part, -1)
				if len(tokens) == 0 {
					continue
				}
				if hasFields && len(rows) > groupStart {
					finish()
					groupStart = len(rows)
					property = PipelineProperty{}
					hasFields = false
				}
				for _, token := range tokens {
					rows = append(rows, PipelineProperty{ParcelKey: normalizeDelinquentKey(token)})
				}
				continue
			}
			value := part
			if index := strings.Index(part, ":"); index >= 0 {
				value = strings.TrimSpace(part[index+1:])
			}
			switch {
			case strings.Contains(lower, "owner"):
				property.OwnerName = value
			case strings.Contains(lower, "situs") || strings.Contains(lower, "property address"):
				property.SitusLine = value
			case strings.Contains(lower, "township"):
				property.Township = value
			case strings.Contains(lower, "acreage"):
				property.Acreage = value
			case strings.Contains(lower, "tax value"):
				if !strings.Contains(part, ":") {
					value = strings.TrimSpace(part[strings.Index(lower, "tax value")+len("tax value"):])
				}
				if value != "" {
					amount := strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(value, "$", ""), ",", ""), " ", "")
					if !moneyPipeline.MatchString(amount) {
						return nil, fmt.Errorf("invalid pipeline Tax Value %q", value)
					}
					cents, err := parseDelinquentBalance(amount)
					if err != nil {
						return nil, err
					}
					property.TaxValue = &cents
				}
			case strings.Contains(lower, "complaint filed") || strings.Contains(lower, "judicial foreclosure case") || strings.Contains(lower, "court case"):
				property.CaseNumber = value
			case strings.Contains(lower, "redeemed") || strings.Contains(lower, "upset bid") || strings.Contains(lower, "status"):
				property.Status = part
				property.Redeemed = property.Redeemed || strings.Contains(lower, "redeemed")
			case strings.Contains(lower, "sale") || fullDatePipeline.MatchString(part):
				property.ScheduleText = part
				if date := fullDatePipeline.FindString(part); date != "" {
					parsed, err := time.Parse("January 2 2006", strings.ReplaceAll(date, ",", ""))
					if err == nil {
						property.SaleOn = &parsed
					}
				}
			default:
				continue
			}
			hasFields = len(rows) > groupStart
		}
	}
	finish()
	seen := make(map[string]bool, len(rows))
	unique := rows[:0]
	for _, row := range rows {
		if seen[row.ParcelKey] {
			continue
		}
		seen[row.ParcelKey] = true
		unique = append(unique, row)
	}
	return unique, nil
}
