package union

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/JustinMonty20/landman/internal/taxartifact"
	"github.com/JustinMonty20/landman/internal/taxsource"
)

const (
	// BillingURL identifies the county reference for operator-supplied billing files; it is not a download record.
	BillingURL    = "https://www.unioncountync.gov/home/showpublisheddocument/6045/639234409945470000"
	billingSource = "union_billing"
	billingLimit  = 256 << 20
)

// BillRow holds billed tax in cents until its parcel is found. It is not a payoff quote.
type BillRow struct {
	ParcelKey         string
	Year              int16
	TotalTaxes        int64
	PropertyAddress   string
	TaxpayerName      string
	MailingLine       string
	MailingCity       string
	MailingState      string
	MailingPostalCode string
}

// BillingFile retains an operator-supplied ZIP before it parses the CSV entry.
type BillingFile struct {
	path      string
	sourceURL string
	store     taxartifact.Store
}

var _ taxsource.Source[BillRow] = (*BillingFile)(nil)

// NewBillingFile requires a local ZIP, its declared county URL, and one artifact store.
func NewBillingFile(path, sourceURL string, store taxartifact.Store) (*BillingFile, error) {
	parsed, err := url.Parse(sourceURL)
	if path == "" || store == nil || err != nil || parsed == nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return nil, errors.New("billing ZIP path, HTTPS source URL, and artifact store required")
	}
	return &BillingFile{path: path, sourceURL: sourceURL, store: store}, nil
}

// Read validates the complete entry before it yields a bill and returns retained-file evidence.
func (s *BillingFile) Read(ctx context.Context, yield func(BillRow) error) (taxsource.Meta, error) {
	if yield == nil {
		return taxsource.Meta{}, errors.New("billing yield required")
	}
	if err := ctx.Err(); err != nil {
		return taxsource.Meta{}, err
	}
	body, err := os.Open(s.path)
	if err != nil {
		return taxsource.Meta{}, fmt.Errorf("open operator-supplied billing ZIP: %w", err)
	}
	info, statErr := body.Stat()
	if statErr != nil {
		return taxsource.Meta{}, errors.Join(fmt.Errorf("stat billing ZIP: %w", statErr), body.Close())
	}
	if info.Size() > billingLimit {
		return taxsource.Meta{}, errors.Join(fmt.Errorf("billing ZIP exceeds %d-byte compressed cap", billingLimit), body.Close())
	}
	artifact, putErr := s.store.Put(ctx, billingSource, &billingSizeReader{body: body, remaining: billingLimit})
	if err := errors.Join(putErr, body.Close()); err != nil {
		return taxsource.Meta{}, fmt.Errorf("retain billing ZIP: %w", err)
	}
	meta := taxsource.Meta{URL: s.sourceURL, Name: billingSource, SHA256: artifact.SHA256(), FetchedAt: time.Now().UTC(), Artifact: artifact}
	if _, err := s.scan(ctx, artifact, func(BillRow) error { return nil }); err != nil {
		return meta, err
	}
	_, err = s.scan(ctx, artifact, yield)
	return meta, err
}

// Skipped counts personal-property rows in the retained ZIP.
func (s *BillingFile) Skipped(ctx context.Context, meta taxsource.Meta) (int, error) {
	if !meta.Artifact.Verified() || meta.Name != billingSource || meta.URL != s.sourceURL {
		return 0, errors.New("verified billing artifact required")
	}
	return s.scan(ctx, meta.Artifact, func(BillRow) error { return nil })
}

func (s *BillingFile) scan(ctx context.Context, artifact taxartifact.Artifact, yield func(BillRow) error) (skipped int, err error) {
	file, entry, err := s.openEntry(ctx, artifact)
	if err != nil {
		return 0, err
	}
	defer func() { err = errors.Join(err, file.Close(), os.Remove(file.Name())) }()
	return parseBilling(ctx, entry, yield)
}

func (s *BillingFile) openEntry(ctx context.Context, artifact taxartifact.Artifact) (*os.File, *zip.File, error) {
	body, err := s.store.Get(ctx, artifact)
	if err != nil {
		return nil, nil, err
	}
	file, err := os.CreateTemp("", ".billing-*.zip")
	if err != nil {
		return nil, nil, errors.Join(err, body.Close())
	}
	failed := func(err error) (*os.File, *zip.File, error) {
		file.Close()
		os.Remove(file.Name())
		return nil, nil, err
	}
	_, copyErr := io.Copy(file, body)
	if err := errors.Join(copyErr, body.Close()); err != nil {
		return failed(fmt.Errorf("copy retained billing ZIP: %w", err))
	}
	info, err := file.Stat()
	if err != nil {
		return failed(err)
	}
	archive, err := zip.NewReader(file, info.Size())
	if err != nil {
		return failed(fmt.Errorf("invalid billing ZIP: %w", err))
	}
	var selected *zip.File
	for _, entry := range archive.File {
		if !entry.FileInfo().IsDir() && strings.Contains(entry.Name, "WKBILA") && strings.HasSuffix(entry.Name, ".csv") {
			if selected != nil {
				return failed(errors.New("billing ZIP has multiple WKBILA CSV entries"))
			}
			selected = entry
		}
	}
	if selected == nil {
		return failed(errors.New("billing ZIP has no WKBILA CSV entry"))
	}
	if selected.UncompressedSize64 > billingLimit {
		return failed(fmt.Errorf("billing CSV entry exceeds %d-byte uncompressed cap", billingLimit))
	}
	return file, selected, nil
}

func parseBilling(ctx context.Context, entry *zip.File, yield func(BillRow) error) (skipped int, err error) {
	body, err := entry.Open()
	if err != nil {
		return 0, err
	}
	defer func() { err = errors.Join(err, body.Close()) }()
	limited := &io.LimitedReader{R: billingContextReader{ctx: ctx, body: body}, N: billingLimit + 1}
	lines := bufio.NewScanner(limited)
	lines.Buffer(make([]byte, 64<<10), 1<<20)
	if !lines.Scan() {
		return 0, fmt.Errorf("read billing header: %w", errors.Join(lines.Err(), io.EOF))
	}
	headers, err := billingRecord(lines.Bytes())
	if err != nil {
		return 0, fmt.Errorf("read billing header: %w", err)
	}
	required := []string{"property_key", "source", "year", "TOTAL TAXES AND CHARGES", "PROPERTY ADDRESS", "Taxpayer Name 1", "Taypayer Address 1"}
	columns := make(map[string]int, len(headers))
	for i, header := range headers {
		columns[header] = i
	}
	for _, header := range required {
		if _, ok := columns[header]; !ok {
			return 0, fmt.Errorf("missing required column %q; headers seen: %q", header, headers)
		}
	}
	for {
		if err := ctx.Err(); err != nil {
			return skipped, err
		}
		if !lines.Scan() {
			if err := lines.Err(); err != nil {
				return skipped, fmt.Errorf("read billing CSV: %w", err)
			}
			if limited.N == 0 {
				return skipped, fmt.Errorf("billing CSV entry exceeds %d-byte uncompressed cap", billingLimit)
			}
			return skipped, nil
		}
		record, err := billingRecord(lines.Bytes())
		if err != nil {
			return skipped, fmt.Errorf("read billing CSV: %w", err)
		}
		if len(record) != len(headers) {
			for _, name := range required {
				if columns[name] >= len(record) {
					return skipped, fmt.Errorf("short billing row: missing %q", name)
				}
			}
			return skipped, fmt.Errorf("billing row has %d fields; expected %d", len(record), len(headers))
		}
		field := func(name string) (string, error) {
			index := columns[name]
			if index >= len(record) {
				return "", fmt.Errorf("short billing row: missing %q", name)
			}
			return strings.TrimSpace(record[index]), nil
		}
		source, err := field("source")
		if err != nil {
			return skipped, err
		}
		if source == "PP" {
			skipped++
			continue
		}
		if source != "RE" {
			return skipped, fmt.Errorf("unexpected billing source %q", source)
		}
		key, err := field("property_key")
		if err != nil {
			return skipped, err
		}
		yearText, err := field("year")
		if err != nil {
			return skipped, err
		}
		amount, err := field("TOTAL TAXES AND CHARGES")
		if err != nil {
			return skipped, err
		}
		year, err := strconv.ParseInt(yearText, 10, 16)
		if err != nil || year < 1900 || year > 2200 {
			return skipped, fmt.Errorf("invalid billing year %q", yearText)
		}
		cents, err := parseBillingMoney(amount)
		if err != nil {
			return skipped, err
		}
		key = normalizeDelinquentKey(key)
		if key == "" {
			return skipped, errors.New("empty billing property_key")
		}
		address, err := field("PROPERTY ADDRESS")
		if err != nil {
			return skipped, err
		}
		name, err := field("Taxpayer Name 1")
		if err != nil {
			return skipped, err
		}
		mailing, err := field("Taypayer Address 1")
		if err != nil {
			return skipped, err
		}
		optional := func(name string) string {
			index, ok := columns[name]
			if !ok || index >= len(record) {
				return ""
			}
			return strings.TrimSpace(record[index])
		}
		if err := ctx.Err(); err != nil {
			return skipped, err
		}
		if err := yield(BillRow{ParcelKey: key, Year: int16(year), TotalTaxes: cents, PropertyAddress: address,
			TaxpayerName: name, MailingLine: mailing, MailingCity: optional("Taxpayer City"),
			MailingState: optional("Taxpayer State"), MailingPostalCode: optional("Taxpayer ZIP Code")}); err != nil {
			return skipped, err
		}
	}
}

func billingRecord(line []byte) ([]string, error) {
	quoted, atStart := false, true
	for i := 0; i < len(line); i++ {
		switch {
		case quoted && line[i] == '"':
			if i+1 < len(line) && line[i+1] == '"' {
				i++
			} else if i+1 == len(line) || line[i+1] == ',' {
				quoted = false
			}
		case !quoted && atStart && line[i] == '"':
			quoted, atStart = true, false
		case !quoted && line[i] == '"':
			return nil, errors.New("quote in unquoted billing field")
		case !quoted && line[i] == ',':
			atStart = true
		case !quoted:
			atStart = false
		}
	}
	if quoted {
		return nil, errors.New("unterminated quoted billing field")
	}
	reader := csv.NewReader(bytes.NewReader(line))
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true
	return reader.Read()
}

// billingSizeReader rejects excess bytes before the artifact store can publish them.
type billingSizeReader struct {
	body      io.Reader
	remaining int64
}

func (r *billingSizeReader) Read(p []byte) (int, error) {
	if r.remaining == 0 {
		var extra [1]byte
		n, err := r.body.Read(extra[:])
		if n > 0 {
			return 0, fmt.Errorf("billing ZIP exceeds %d-byte compressed cap", billingLimit)
		}
		return 0, err
	}
	if int64(len(p)) > r.remaining {
		p = p[:int(r.remaining)]
	}
	n, err := r.body.Read(p)
	r.remaining -= int64(n)
	return n, err
}

func parseBillingMoney(raw string) (int64, error) {
	value := strings.TrimSpace(raw)
	if strings.HasPrefix(value, ".") {
		value = "0" + value
	}
	cents, err := parseDelinquentBalance(value)
	if err != nil || cents > 99999999999999 {
		return 0, fmt.Errorf("invalid TOTAL TAXES AND CHARGES %q", raw)
	}
	return cents, nil
}

type billingContextReader struct {
	ctx  context.Context
	body io.Reader
}

func (r billingContextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := r.body.Read(p)
	if cancelErr := r.ctx.Err(); cancelErr != nil {
		return n, cancelErr
	}
	return n, err
}
