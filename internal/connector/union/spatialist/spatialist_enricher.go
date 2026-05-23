package spatialist

import (
	"context"
	"errors"
	"fmt"

	"github.com/JustinMonty20/landman/internal/connector"
)

// RawBatchEnricher is the minimal interface needed for spatialist enrichment.
type RawBatchEnricher interface {
	EnrichBatch(ctx context.Context, parcelIDs []string) (map[string]map[string]connector.RawRecord, error)
}

// SpatialistTransformer converts raw spatialist data into a typed, flattened record.
type SpatialistTransformer func(raw map[string]interface{}) (*SpatialistFlatEnvelope, error)

// SpatialistRecordFilter returns true when a transformed record should be kept.
type SpatialistRecordFilter func(raw map[string]interface{}, transformed *SpatialistFlatEnvelope) bool

// SpatialistEnricherOption customizes SpatialistEnricher behavior.
type SpatialistEnricherOption func(*SpatialistEnricher)

// SpatialistEnricher fetches spatialist records and applies the spatialist transformer.
type SpatialistEnricher struct {
	batchEnricher RawBatchEnricher
	sourceName    string
	transform     SpatialistTransformer
	filter        SpatialistRecordFilter
}

// NewSpatialistEnricher creates a new spatialist enrichment service.
func NewSpatialistEnricher(batchEnricher RawBatchEnricher, sourceName string, transform SpatialistTransformer, options ...SpatialistEnricherOption) (*SpatialistEnricher, error) {
	if batchEnricher == nil {
		return nil, fmt.Errorf("batch enricher cannot be nil")
	}
	if sourceName == "" {
		return nil, fmt.Errorf("source name cannot be empty")
	}
	if transform == nil {
		return nil, fmt.Errorf("transform cannot be nil")
	}

	svc := &SpatialistEnricher{
		batchEnricher: batchEnricher,
		sourceName:    sourceName,
		transform:     transform,
	}

	for _, option := range options {
		if option != nil {
			option(svc)
		}
	}

	return svc, nil
}

// WithRecordFilter configures a filter that controls which transformed records
// are eligible for downstream processing.
func WithRecordFilter(filter SpatialistRecordFilter) SpatialistEnricherOption {
	return func(s *SpatialistEnricher) {
		s.filter = filter
	}
}

// EnrichBatch fetches spatialist data and returns typed flattened records per parcel.
func (s *SpatialistEnricher) EnrichBatch(ctx context.Context, parcelIDs []string) (map[string]*SpatialistFlatEnvelope, error) {
	if len(parcelIDs) == 0 {
		return map[string]*SpatialistFlatEnvelope{}, nil
	}

	rawRecords, err := s.batchEnricher.EnrichBatch(ctx, parcelIDs)
	var errs []error
	if err != nil {
		errs = append(errs, err)
	}

	out := make(map[string]*SpatialistFlatEnvelope, len(parcelIDs))

	for _, parcelID := range parcelIDs {
		parcelRecords, ok := rawRecords[parcelID]
		if !ok {
			errs = append(errs, fmt.Errorf("parcel %s missing records", parcelID))
			continue
		}
		record, ok := parcelRecords[s.sourceName]
		if !ok {
			errs = append(errs, fmt.Errorf("parcel %s missing source %s", parcelID, s.sourceName))
			continue
		}
		if record.RawData == nil {
			errs = append(errs, fmt.Errorf("parcel %s source %s missing raw data", parcelID, s.sourceName))
			continue
		}

		flat, transformErr := s.transform(record.RawData)
		if transformErr != nil {
			errs = append(errs, fmt.Errorf("parcel %s source %s transform: %w", parcelID, s.sourceName, transformErr))
			continue
		}
		if s.filter != nil && !s.filter(record.RawData, flat) {
			continue
		}
		out[parcelID] = flat
	}

	if len(errs) > 0 {
		return out, fmt.Errorf("spatialist enrichment completed with %d errors: %w", len(errs), errors.Join(errs...))
	}

	return out, nil
}
