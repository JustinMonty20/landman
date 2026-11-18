package union

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/JustinMonty20/landman/internal/taxartifact"
	"github.com/JustinMonty20/landman/internal/taxsource"
)

const (
	DelinquentURL    = "https://www.unioncountync.gov/home/showdocument?id=4792"
	delinquentSource = "union_delinquent"
)

// DelinquentRow holds a real-property balance in cents until its parcel is found.
type DelinquentRow struct {
	ParcelKey     string
	Year          int16
	Balance       int64
	PrimarySource string
}

// DelinquentList retains exact file bytes before it parses and yields tax rows.
type DelinquentList struct {
	path  string
	store taxartifact.Store
}

var _ taxsource.Source[DelinquentRow] = (*DelinquentList)(nil)

// NewDelinquentList requires an operator-supplied file and a selected artifact store.
func NewDelinquentList(path string, store taxartifact.Store) (*DelinquentList, error) {
	if store == nil || path == "" {
		return nil, errors.New("delinquent file path and artifact store required")
	}
	return &DelinquentList{path: path, store: store}, nil
}

// ReadRetained parses verified capture bytes without opening an operator input path.
func (s *DelinquentList) ReadRetained(ctx context.Context, artifact taxartifact.Artifact, yield func(DelinquentRow) error) (int, int, error) {
	if !artifact.Verified() || yield == nil {
		return 0, 0, errors.New("verified tax capture and yield required")
	}
	counts, err := s.scan(ctx, artifact, yield)
	return counts.pp, counts.rr, err
}

// Read checks the whole file before it yields a row. It returns verified file evidence even on a parse error.
func (s *DelinquentList) Read(ctx context.Context, yield func(DelinquentRow) error) (taxsource.Meta, error) {
	if yield == nil {
		return taxsource.Meta{}, errors.New("delinquent yield required")
	}
	if err := ctx.Err(); err != nil {
		return taxsource.Meta{}, err
	}
	body, err := os.Open(s.path)
	if err != nil {
		return taxsource.Meta{}, fmt.Errorf("open operator-supplied delinquent file: %w", err)
	}
	artifact, putErr := s.store.Put(ctx, delinquentSource, body)
	if err := errors.Join(putErr, body.Close()); err != nil {
		return taxsource.Meta{}, fmt.Errorf("retain delinquent file: %w", err)
	}
	meta := taxsource.Meta{URL: DelinquentURL, Name: delinquentSource, SHA256: artifact.SHA256(), FetchedAt: time.Now().UTC(), Artifact: artifact}
	if _, err := s.scan(ctx, artifact, func(DelinquentRow) error { return nil }); err != nil {
		return meta, err
	}
	_, err = s.scan(ctx, artifact, yield)
	return meta, err
}

// Skipped counts the personal-property and RR rows in the retained file.
func (s *DelinquentList) Skipped(ctx context.Context, meta taxsource.Meta) (int, int, error) {
	if !meta.Artifact.Verified() || meta.Name != delinquentSource {
		return 0, 0, errors.New("verified delinquent artifact required")
	}
	counts, err := s.scan(ctx, meta.Artifact, func(DelinquentRow) error { return nil })
	return counts.pp, counts.rr, err
}

type delinquentCounts struct{ pp, rr int }

func (s *DelinquentList) scan(ctx context.Context, artifact taxartifact.Artifact, yield func(DelinquentRow) error) (delinquentCounts, error) {
	body, err := s.store.Get(ctx, artifact)
	if err != nil {
		return delinquentCounts{}, err
	}
	counts, parseErr := parseDelinquent(ctx, body, yield)
	return counts, errors.Join(parseErr, body.Close())
}

func parseDelinquent(ctx context.Context, body io.Reader, yield func(DelinquentRow) error) (delinquentCounts, error) {
	reader := csv.NewReader(body)
	reader.FieldsPerRecord = -1
	headers, err := reader.Read()
	if err != nil {
		return delinquentCounts{}, err
	}
	required := []string{"Property Key", "Year", "Primary Source", "Balance Due"}
	columns := make(map[string]int, len(headers))
	for i, header := range headers {
		columns[header] = i
	}
	for _, header := range required {
		if _, ok := columns[header]; !ok {
			return delinquentCounts{}, fmt.Errorf("missing required column %q; headers seen: %q", header, headers)
		}
	}
	var counts delinquentCounts
	for {
		if err := ctx.Err(); err != nil {
			return counts, err
		}
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			return counts, nil
		}
		if err != nil {
			return counts, fmt.Errorf("read delinquent CSV: %w", err)
		}
		field := func(name string) (string, error) {
			index := columns[name]
			if index >= len(record) {
				return "", fmt.Errorf("short delinquent row: missing %q", name)
			}
			return strings.TrimSpace(record[index]), nil
		}
		source, err := field("Primary Source")
		if err != nil {
			return counts, err
		}
		switch source {
		case "PP":
			counts.pp++
			continue
		case "RR":
			counts.rr++
			continue
		case "RE":
		default:
			return counts, fmt.Errorf("unexpected Primary Source %q", source)
		}
		key, err := field("Property Key")
		if err != nil {
			return counts, err
		}
		yearText, err := field("Year")
		if err != nil {
			return counts, err
		}
		balanceText, err := field("Balance Due")
		if err != nil {
			return counts, err
		}
		year, err := strconv.ParseInt(yearText, 10, 16)
		if err != nil || year < 1900 || year > 2200 {
			return counts, fmt.Errorf("invalid delinquent Year %q", yearText)
		}
		balance, err := parseDelinquentBalance(balanceText)
		if err != nil {
			return counts, err
		}
		key = normalizeDelinquentKey(key)
		if key == "" {
			return counts, errors.New("empty delinquent Property Key")
		}
		if err := yield(DelinquentRow{ParcelKey: key, Year: int16(year), Balance: balance, PrimarySource: source}); err != nil {
			return counts, err
		}
	}
}

func normalizeDelinquentKey(key string) string {
	return strings.TrimSpace(strings.ToUpper(strings.ReplaceAll(strings.ReplaceAll(key, "-", ""), ".", "")))
}

func parseDelinquentBalance(raw string) (int64, error) {
	value := strings.ReplaceAll(strings.TrimPrefix(strings.TrimSpace(raw), "$"), ",", "")
	parts := strings.Split(value, ".")
	if len(parts) != 2 || len(parts[1]) != 2 || parts[0] == "" {
		return 0, fmt.Errorf("invalid Balance Due %q", raw)
	}
	dollars, err := strconv.ParseUint(parts[0], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid Balance Due %q", raw)
	}
	cents, err := strconv.ParseUint(parts[1], 10, 8)
	if err != nil || cents > 99 || dollars > uint64((math.MaxInt64-int64(cents))/100) {
		return 0, fmt.Errorf("invalid Balance Due %q", raw)
	}
	return int64(dollars)*100 + int64(cents), nil
}
