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
var pdfShow = regexp.MustCompile(`(?s)(\[(?:[^\]]|\\.)*\]|\((?:\\.|[^\\)])*\))\s*(TJ|Tj|'|")`)
var pdfTextToken = regexp.MustCompile(`(?s)(?:\[(?:[^\]]|\\.)*\]|\((?:\\.|[^\\)])*\))\s*(?:TJ|Tj|'|")|(?:[-+\d.]+\s+){2}Td\b|\bT\*\b`)
var pdfString = regexp.MustCompile(`\((?:\\.|[^\\)])*\)`)
var pdfAlternative = regexp.MustCompile(`/(?:ActualText|Alt)\s*(\((?:\\.|[^\\)])*\)|<([0-9A-Fa-f]+)>)`)

func pipelinePDF(ctx context.Context, data []byte) ([][]string, error) {
	objects := map[string][]byte{}
	order := []string{}
	for _, match := range pdfObject.FindAllSubmatch(data, -1) {
		key := string(match[1])
		objects[key] = match[2]
		order = append(order, key)
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
			pages = append(pages, nil)
			continue
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
			for _, token := range pdfTextToken.FindAll(content, -1) {
				match := pdfShow.FindSubmatch(token)
				if match == nil {
					flush()
					continue
				}
				if string(match[2]) == "'" || string(match[2]) == "\"" {
					flush()
				}
				for _, part := range pdfString.FindAll(match[1], -1) {
					line.WriteString(pdfDecode(part[1 : len(part)-1]))
				}
			}
			flush()
			for _, match := range pdfAlternative.FindAllSubmatch(content, -1) {
				value := match[1]
				if len(match[2]) != 0 {
					value, _ = hex.DecodeString(string(match[2]))
				} else {
					value = []byte(pdfDecode(value[1 : len(value)-1]))
				}
				if len(value) >= 2 && value[0] == 0xfe && value[1] == 0xff {
					units := make([]uint16, 0, (len(value)-2)/2)
					for i := 2; i+1 < len(value); i += 2 {
						units = append(units, uint16(value[i])<<8|uint16(value[i+1]))
					}
					lines = append(lines, string(utf16.Decode(units)))
				} else {
					lines = append(lines, string(value))
				}
			}
		}
		pages = append(pages, lines)
	}
	if len(pages) == 0 {
		return nil, errors.New("PDF has no pages")
	}
	return pages, nil
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
	valid := false
	for _, file := range archive.File {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if file.Name == "[Content_Types].xml" {
			b, e := readPipelineEntry(file)
			if e != nil {
				return nil, e
			}
			valid = bytes.Contains(b, []byte("presentationml.presentation"))
			continue
		}
		if !slideFile.MatchString(file.Name) && !noteFile.MatchString(file.Name) {
			continue
		}
		b, e := readPipelineEntry(file)
		if e != nil {
			return nil, e
		}
		lines, e := pipelineXML(b)
		if e != nil {
			return nil, e
		}
		if slideFile.MatchString(file.Name) {
			slides[file.Name] = lines
		} else {
			notes[file.Name] = lines
		}
	}
	if !valid || len(slides) == 0 {
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
func pipelineXML(data []byte) ([]string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var lines []string
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return lines, nil
		}
		if err != nil {
			return nil, err
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		if start.Name.Local == "cNvPr" {
			for _, a := range start.Attr {
				if (a.Name.Local == "descr" || a.Name.Local == "title") && a.Value != "" {
					lines = append(lines, a.Value)
				}
			}
		}
		if start.Name.Local == "t" {
			var value string
			if err := decoder.DecodeElement(&value, &start); err != nil {
				return nil, err
			}
			lines = append(lines, value)
		}
	}
}

var parcelToken = regexp.MustCompile(`(?i)\b(?:\d{2}[.\-])?\d{3}[.\-]\d{3}[A-Z0-9]*\b|\b\d{8}[A-Z0-9]*\b`)
var moneyPipeline = regexp.MustCompile(`^\d+\.\d{2}$`)
var fullDatePipeline = regexp.MustCompile(`(?i)\b(January|February|March|April|May|June|July|August|September|October|November|December)\s+\d{1,2},?\s+\d{4}\b`)

func pipelineProperties(lines []string) ([]PipelineProperty, error) {
	var rows []PipelineProperty
	var current PipelineProperty
	flush := func() {
		if current.ParcelKey != "" {
			rows = append(rows, current)
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
				for _, token := range tokens {
					if current.ParcelKey != "" {
						flush()
					}
					previous := current
					current = PipelineProperty{ParcelKey: normalizeDelinquentKey(token)}
					if len(rows) == 0 {
						current.OwnerName, current.SitusLine, current.Township, current.Acreage = previous.OwnerName, previous.SitusLine, previous.Township, previous.Acreage
						current.TaxValue, current.CaseNumber, current.SaleOn, current.ScheduleText, current.Status, current.Redeemed = previous.TaxValue, previous.CaseNumber, previous.SaleOn, previous.ScheduleText, previous.Status, previous.Redeemed
					}
				}
				continue
			}
			value := func() string {
				if i := strings.Index(part, ":"); i >= 0 {
					return strings.TrimSpace(part[i+1:])
				}
				return part
			}()
			switch {
			case strings.Contains(lower, "owner"):
				current.OwnerName = value
			case strings.Contains(lower, "situs") || strings.Contains(lower, "property address"):
				current.SitusLine = value
			case strings.Contains(lower, "township"):
				current.Township = value
			case strings.Contains(lower, "acreage"):
				current.Acreage = value
			case strings.Contains(lower, "tax value"):
				if !strings.Contains(part, ":") {
					value = strings.TrimSpace(part[strings.Index(lower, "tax value")+len("tax value"):])
				}
				if value == "" {
					continue
				}
				amount := strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(value, "$", ""), ",", ""), " ", "")
				if !moneyPipeline.MatchString(amount) {
					return nil, fmt.Errorf("invalid pipeline Tax Value %q", value)
				}
				cents, err := parseDelinquentBalance(amount)
				if err != nil {
					return nil, err
				}
				current.TaxValue = &cents
			case strings.Contains(lower, "complaint filed") || strings.Contains(lower, "judicial foreclosure case") || strings.Contains(lower, "court case"):
				current.CaseNumber = value
			case strings.Contains(lower, "redeemed") || strings.Contains(lower, "upset bid") || strings.Contains(lower, "status"):
				current.Status = part
				current.Redeemed = strings.Contains(lower, "redeemed")
			case strings.Contains(lower, "sale") || fullDatePipeline.MatchString(part):
				current.ScheduleText = part
				if date := fullDatePipeline.FindString(part); date != "" {
					date = strings.ReplaceAll(date, ",", "")
					parsed, err := time.Parse("January 2 2006", date)
					if err == nil {
						current.SaleOn = &parsed
					}
				}
			}
		}
	}
	flush()
	// All PINs on one slide share its property evidence.
	if len(rows) > 1 {
		shared := PipelineProperty{}
		for _, row := range rows {
			if row.OwnerName != "" {
				shared.OwnerName = row.OwnerName
			}
			if row.SitusLine != "" {
				shared.SitusLine = row.SitusLine
			}
			if row.Township != "" {
				shared.Township = row.Township
			}
			if row.Acreage != "" {
				shared.Acreage = row.Acreage
			}
			if row.TaxValue != nil {
				shared.TaxValue = row.TaxValue
			}
			if row.CaseNumber != "" {
				shared.CaseNumber = row.CaseNumber
			}
			if row.ScheduleText != "" {
				shared.ScheduleText, shared.SaleOn = row.ScheduleText, row.SaleOn
			}
			if row.Status != "" {
				shared.Status = row.Status
			}
			shared.Redeemed = shared.Redeemed || row.Redeemed
		}
		for i := range rows {
			rows[i].OwnerName, rows[i].SitusLine, rows[i].Township, rows[i].Acreage = shared.OwnerName, shared.SitusLine, shared.Township, shared.Acreage
			rows[i].TaxValue, rows[i].CaseNumber, rows[i].ScheduleText, rows[i].SaleOn = shared.TaxValue, shared.CaseNumber, shared.ScheduleText, shared.SaleOn
			rows[i].Status, rows[i].Redeemed = shared.Status, shared.Redeemed
		}
	}
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
