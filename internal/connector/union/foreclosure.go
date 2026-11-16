package union

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/JustinMonty20/landman/internal/taxartifact"
	"github.com/JustinMonty20/landman/internal/taxsource"
)

const (
	ForeclosureURL    = "https://www.unioncountync.gov/home/showdocument?id=4787"
	foreclosureSource = "union_foreclosure_sales"
	foreclosureLimit  = 16 << 20
)

// ForeclosurePIN carries a parcel lookup key and the details printed for that PIN.
type ForeclosurePIN struct {
	ParcelKey        string
	SitusLine        string
	LegalDescription string
	TaxValue         *int64 // cents; nil when unspecified
}

// ForeclosureCase represents one sale, including every PIN named by that sale.
type ForeclosureCase struct {
	CaseNumber    string // empty for TBD
	OwnerName     string
	SaleOn        *time.Time
	Status        string
	OpeningBid    *int64
	CurrentBid    *int64
	UpsetBid      *int64
	Redeemed      bool
	BiddingClosed bool
	PINs          []ForeclosurePIN
}

// ForeclosureList reads a locally supplied sales PDF and retains its exact bytes.
type ForeclosureList struct {
	path  string
	store taxartifact.Store
}

var _ taxsource.Source[ForeclosureCase] = (*ForeclosureList)(nil)

func NewForeclosureList(path string, store taxartifact.Store) (*ForeclosureList, error) {
	if path == "" || store == nil {
		return nil, errors.New("foreclosure PDF path and artifact store required")
	}
	return &ForeclosureList{path: path, store: store}, nil
}

// Read verifies the whole PDF and all amounts before yielding any case.
func (s *ForeclosureList) Read(ctx context.Context, yield func(ForeclosureCase) error) (taxsource.Meta, error) {
	if yield == nil {
		return taxsource.Meta{}, errors.New("foreclosure yield required")
	}
	if err := ctx.Err(); err != nil {
		return taxsource.Meta{}, err
	}
	body, err := os.Open(s.path)
	if err != nil {
		return taxsource.Meta{}, fmt.Errorf("open operator-supplied foreclosure PDF: %w", err)
	}
	info, err := body.Stat()
	if err != nil {
		return taxsource.Meta{}, errors.Join(err, body.Close())
	}
	if info.Size() > foreclosureLimit {
		return taxsource.Meta{}, errors.Join(errors.New("foreclosure PDF exceeds size limit"), body.Close())
	}
	magic := make([]byte, 4)
	if _, err := io.ReadFull(body, magic); err != nil || string(magic) != "%PDF" {
		return taxsource.Meta{}, errors.Join(errors.New("foreclosure file is not a PDF"), body.Close())
	}
	if _, err := body.Seek(0, io.SeekStart); err != nil {
		return taxsource.Meta{}, errors.Join(err, body.Close())
	}
	artifact, putErr := s.store.Put(ctx, foreclosureSource, &foreclosureSizeReader{body: body, remaining: foreclosureLimit})
	if err := errors.Join(putErr, body.Close()); err != nil {
		return taxsource.Meta{}, fmt.Errorf("retain foreclosure PDF: %w", err)
	}
	meta := taxsource.Meta{URL: ForeclosureURL, Name: foreclosureSource, SHA256: artifact.SHA256(), FetchedAt: time.Now().UTC(), Artifact: artifact}
	text, err := s.extract(ctx, artifact)
	if err != nil {
		return meta, err
	}
	cases, _, err := parseForeclosure(ctx, text)
	if err != nil {
		return meta, err
	}
	for _, sale := range cases {
		if err := ctx.Err(); err != nil {
			return meta, err
		}
		if err := yield(sale); err != nil {
			return meta, err
		}
	}
	return meta, nil
}

// Skipped counts case blocks without a PIN in the retained file.
func (s *ForeclosureList) Skipped(ctx context.Context, meta taxsource.Meta) (int, error) {
	if !meta.Artifact.Verified() || meta.Name != foreclosureSource || meta.URL != ForeclosureURL {
		return 0, errors.New("verified foreclosure artifact required")
	}
	text, err := s.extract(ctx, meta.Artifact)
	if err != nil {
		return 0, err
	}
	_, skipped, err := parseForeclosure(ctx, text)
	return skipped, err
}

func (s *ForeclosureList) extract(ctx context.Context, artifact taxartifact.Artifact) (string, error) {
	body, err := s.store.Get(ctx, artifact)
	if err != nil {
		return "", err
	}
	file, err := os.CreateTemp("", ".foreclosure-*.pdf")
	if err != nil {
		return "", errors.Join(err, body.Close())
	}
	defer os.Remove(file.Name())
	defer file.Close()
	_, copyErr := io.Copy(file, &billingContextReader{ctx: ctx, body: body})
	if err := errors.Join(copyErr, body.Close()); err != nil {
		return "", fmt.Errorf("read retained foreclosure PDF: %w", err)
	}
	var output boundedForeclosureText
	extractCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(extractCtx, "pdftotext", "-layout", file.Name(), "-")
	cmd.Stdout = &output
	var stderr boundedForeclosureText
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if extractCtx.Err() != nil {
			return "", extractCtx.Err()
		}
		return "", fmt.Errorf("extract foreclosure PDF: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return output.String(), nil
}

type foreclosureSizeReader struct {
	body      io.Reader
	remaining int64
}

func (r *foreclosureSizeReader) Read(p []byte) (int, error) {
	if r.remaining == 0 {
		var extra [1]byte
		n, err := r.body.Read(extra[:])
		if n > 0 {
			return 0, errors.New("foreclosure PDF exceeds size limit")
		}
		return 0, err
	}
	if int64(len(p)) > r.remaining {
		p = p[:r.remaining]
	}
	n, err := r.body.Read(p)
	r.remaining -= int64(n)
	return n, err
}

type boundedForeclosureText struct{ bytes.Buffer }

func (b *boundedForeclosureText) Write(p []byte) (int, error) {
	if len(p) > foreclosureLimit-b.Len() {
		return 0, errors.New("foreclosure PDF text exceeds size limit")
	}
	return b.Buffer.Write(p)
}

var (
	caseStart    = regexp.MustCompile(`(?m)^[\f \t]*Sale Date:[ \t]*`)
	pinLabel     = regexp.MustCompile(`(?i)Parcel Identification Number\s*\([^)]*\)\s*:`)
	pinToken     = regexp.MustCompile(`\b[0-9]{2}[-.][0-9]{3}[-.][0-9]{3}[A-Za-z]?(?: [0-9]{2})?\b`)
	caseNumber   = regexp.MustCompile(`(?i)Case\s*#\s*([A-Za-z0-9-]+)`)
	moneyField   = regexp.MustCompile(`(?im)^\s*(?:ESTIMATED\s+)?(Opening Bid|Current Bid|Required Min\. Upset Bid)\s*:?\s*\$?\s*([^\r\n]*)`)
	taxField     = regexp.MustCompile(`(?im)^\s*Tax Value:\s*\$?\s*([^\r\n]*)`)
	ownerField   = regexp.MustCompile(`(?im)^\s*Current Property Owner:\s*(.+)`)
	addressField = regexp.MustCompile(`(?im)^\s*Address:\s*(.+)`)
	dateField    = regexp.MustCompile(`(?i)^([^\r\n]*?)(?:\s+Case\s*#|\r?\n)`)
	taxPINValue  = regexp.MustCompile(`\$([0-9,]+(?:\.[0-9]{2})?)\s+\(+([0-9]{2}[-.][0-9]{3}[-.][0-9]{3}[A-Za-z]?)\)`)
)

func parseForeclosure(ctx context.Context, text string) ([]ForeclosureCase, int, error) {
	if !pinLabel.MatchString(text) {
		return nil, 0, errors.New("foreclosure PDF missing Parcel Identification Number label")
	}
	starts := caseStart.FindAllStringIndex(text, -1)
	if len(starts) == 0 {
		return nil, 0, errors.New("foreclosure PDF has no sale cases")
	}
	var cases []ForeclosureCase
	skipped := 0
	for i, start := range starts {
		if err := ctx.Err(); err != nil {
			return nil, 0, err
		}
		end := len(text)
		if i+1 < len(starts) {
			end = starts[i+1][0]
		}
		block := text[start[1]:end]
		pins, err := foreclosurePINs(block)
		if err != nil {
			return nil, 0, err
		}
		if len(pins) == 0 {
			skipped++
			continue
		}
		sale := ForeclosureCase{PINs: pins}
		if m := caseNumber.FindStringSubmatch(block); m != nil && !strings.EqualFold(m[1], "TBD") {
			sale.CaseNumber = m[1]
		}
		if m := ownerField.FindStringSubmatch(block); m != nil {
			sale.OwnerName = strings.TrimSpace(m[1])
		}
		if m := dateField.FindStringSubmatch(block); m != nil {
			sale.SaleOn = foreclosureDate(m[1])
		}
		for _, m := range moneyField.FindAllStringSubmatch(block, -1) {
			amount, err := foreclosureMoney(m[2])
			if err != nil {
				return nil, 0, fmt.Errorf("case %s %s: %w", sale.CaseNumber, m[1], err)
			}
			switch strings.ToLower(m[1]) {
			case "opening bid":
				sale.OpeningBid = amount
			case "current bid":
				sale.CurrentBid = amount
			default:
				sale.UpsetBid = amount
			}
		}
		upper := strings.ToUpper(block)
		sale.Redeemed = strings.Contains(upper, "REDEEMED")
		sale.BiddingClosed = strings.Contains(upper, "BIDDING") && strings.Contains(upper, "CLOSED")
		switch {
		case sale.Redeemed:
			sale.Status = "redeemed"
		case sale.BiddingClosed:
			sale.Status = "bidding_closed"
		case strings.Contains(upper, "UPSET BID PERIOD ENDS"):
			sale.Status = "upset_bid"
		}
		cases = append(cases, sale)
	}
	return cases, skipped, nil
}

func foreclosurePINs(block string) ([]ForeclosurePIN, error) {
	label := pinLabel.FindStringIndex(block)
	if label == nil {
		return nil, nil
	}
	tail := block[label[1]:]
	lines := strings.Split(tail, "\n")
	pinLine := strings.TrimSpace(lines[0])
	used := 1
	for strings.HasSuffix(pinLine, "-") && used < len(lines) {
		pinLine += strings.TrimSpace(lines[used])
		used++
	}
	tokens := pinToken.FindAllString(pinLine, -1)
	if len(tokens) == 0 {
		return nil, nil
	}
	legalLines := lines[used:]
	legal := make([]string, 0, len(legalLines))
	for _, line := range legalLines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Address:") || strings.HasPrefix(line, "Tax Value:") || strings.HasPrefix(line, "ESTIMATED Opening Bid") {
			break
		}
		if line != "" {
			legal = append(legal, line)
		}
	}
	address := ""
	if m := addressField.FindStringSubmatch(block); m != nil {
		address = strings.TrimSpace(m[1])
	}
	var taxValue *int64
	pinValues := make(map[string]*int64)
	if m := taxField.FindStringSubmatch(block); m != nil {
		// Parenthesized PIN-specific amounts are handled below; otherwise the listed value applies to each PIN.
		value := strings.TrimSpace(strings.Split(m[1], "per lot")[0])
		if idx := strings.Index(value, "("); idx >= 0 {
			value = strings.TrimSpace(value[:idx])
		}
		var err error
		taxValue, err = foreclosureMoney(value)
		if err != nil {
			return nil, fmt.Errorf("tax value: %w", err)
		}
		for _, match := range taxPINValue.FindAllStringSubmatch(block, -1) {
			amount, err := foreclosureMoney(match[1])
			if err != nil {
				return nil, fmt.Errorf("PIN tax value: %w", err)
			}
			pinValues[strings.NewReplacer("-", "", ".", "").Replace(match[2])] = amount
		}
	}
	pins := make([]ForeclosurePIN, 0, len(tokens))
	for _, token := range tokens {
		key := strings.TrimSpace(strings.NewReplacer("-", "", ".", "").Replace(token))
		value := taxValue
		if specific, ok := pinValues[key]; ok {
			value = specific
		}
		pins = append(pins, ForeclosurePIN{ParcelKey: key, SitusLine: address, LegalDescription: strings.Join(legal, " "), TaxValue: value})
	}
	return pins, nil
}

var dollars = regexp.MustCompile(`^\$?\s*(?:[0-9]+|[0-9]{1,3}(?:,[0-9]{3})+)(?:\.[0-9]{2})?$`)

func foreclosureMoney(raw string) (*int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.EqualFold(raw, "TBD") {
		return nil, nil
	}
	if !dollars.MatchString(raw) {
		return nil, fmt.Errorf("invalid dollar amount %q", raw)
	}
	value := strings.ReplaceAll(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "$")), ",", "")
	if !strings.Contains(value, ".") {
		value += ".00"
	}
	amount, err := parseDelinquentBalance(value)
	if err != nil || amount > 99999999999999 {
		return nil, fmt.Errorf("invalid dollar amount %q", raw)
	}
	return &amount, nil
}

var ordinal = regexp.MustCompile(`(?i)([0-9]{1,2})(st|nd|rd|th)\b`)

func foreclosureDate(raw string) *time.Time {
	raw = strings.TrimSpace(ordinal.ReplaceAllString(raw, "$1"))
	raw = strings.ReplaceAll(raw, "Sept.", "Sep")
	if idx := strings.Index(raw, "@"); idx >= 0 {
		raw = strings.TrimSpace(raw[:idx])
	}
	for _, layout := range []string{"January 2, 2006", "Jan 2, 2006"} {
		if date, err := time.Parse(layout, raw); err == nil {
			return &date
		}
	}
	return nil
}
