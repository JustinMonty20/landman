package service

import (
	"context"
	"fmt"

	"github.com/JustinMonty20/landman/internal/connector"
)

// ParcelIDBatchHook is invoked with a batch of parcel IDs.
// batchIndex is 1-based and tracks the upstream batch order.
type ParcelIDBatchHook func(ctx context.Context, batchIndex int, parcelIDs []string) error

// ParcelIDImportService streams parcel IDs from a source in batches.
// It prefers batch-capable sources and falls back to Fetch + chunking.
type ParcelIDImportService struct {
	source    connector.DataSource
	batchSize int
}

// NewParcelIDImportService creates a new importer.
// batchSize defaults to 50 when <= 0.
func NewParcelIDImportService(source connector.DataSource, batchSize int) *ParcelIDImportService {
	if batchSize <= 0 {
		batchSize = 50
	}
	return &ParcelIDImportService{
		source:    source,
		batchSize: batchSize,
	}
}

// Run emits parcel ID batches via the hook.
func (s *ParcelIDImportService) Run(ctx context.Context, onBatch ParcelIDBatchHook) error {
	if s.source == nil {
		return fmt.Errorf("source cannot be nil")
	}
	if onBatch == nil {
		return fmt.Errorf("onBatch cannot be nil")
	}

	if batchSource, ok := s.source.(connector.BatchFetchSource); ok {
		batchIndex := 0
		return batchSource.FetchBatches(ctx, func(batch []connector.RawRecord) error {
			batchIndex++
			ids := extractParcelIDs(batch)
			if len(ids) == 0 {
				return nil
			}
			return onBatch(ctx, batchIndex, ids)
		})
	}

	records, err := s.source.Fetch(ctx)
	if err != nil {
		return err
	}

	return s.emitBatches(ctx, records, onBatch)
}

func (s *ParcelIDImportService) emitBatches(ctx context.Context, records []connector.RawRecord, onBatch ParcelIDBatchHook) error {
	ids := extractParcelIDs(records)
	if len(ids) == 0 {
		return nil
	}

	batchIndex := 0
	for i := 0; i < len(ids); i += s.batchSize {
		end := i + s.batchSize
		if end > len(ids) {
			end = len(ids)
		}
		batchIndex++
		if err := onBatch(ctx, batchIndex, ids[i:end]); err != nil {
			return err
		}
	}
	return nil
}

func extractParcelIDs(records []connector.RawRecord) []string {
	if len(records) == 0 {
		return nil
	}

	ids := make([]string, 0, len(records))
	seen := make(map[string]struct{}, len(records))
	for _, record := range records {
		if record.ParcelID == "" {
			continue
		}
		if _, exists := seen[record.ParcelID]; exists {
			continue
		}
		seen[record.ParcelID] = struct{}{}
		ids = append(ids, record.ParcelID)
	}

	return ids
}
