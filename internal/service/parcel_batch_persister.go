package service

import (
	"context"
	"fmt"
	"log"

	"github.com/JustinMonty20/landman/internal/connector"
	"github.com/JustinMonty20/landman/internal/models"
	"github.com/JustinMonty20/landman/internal/storage"
)

// NormalizedParcelBatchStore persists a normalized parcel batch.
type NormalizedParcelBatchStore interface {
	InsertBatch(ctx context.Context, parcels []*models.GISParcel) (storage.BatchInsertResult, error)
}

// ParcelBatchPersistResult summarizes translation and storage for one source batch.
type ParcelBatchPersistResult struct {
	Translated int
	Inserted   int
	Errors     int
}

// ParcelBatchPersister translates raw parcel records and persists the normalized batch.
type ParcelBatchPersister struct {
	translator connector.Translator
	store      NormalizedParcelBatchStore
	logger     Logger
}

// NewParcelBatchPersister creates a batch persister for normalized parcel imports.
func NewParcelBatchPersister(translator connector.Translator, store NormalizedParcelBatchStore, logger Logger) (*ParcelBatchPersister, error) {
	if translator == nil {
		return nil, fmt.Errorf("translator cannot be nil")
	}
	if store == nil {
		return nil, fmt.Errorf("store cannot be nil")
	}
	if logger == nil {
		logger = log.Default()
	}
	return &ParcelBatchPersister{translator: translator, store: store, logger: logger}, nil
}

// PersistRawBatch translates valid records, stores them as one batch, and logs record-level failures.
func (p *ParcelBatchPersister) PersistRawBatch(ctx context.Context, batchIndex int, records []connector.RawRecord) (ParcelBatchPersistResult, error) {
	var result ParcelBatchPersistResult
	if len(records) == 0 {
		return result, nil
	}

	parcels := make([]*models.GISParcel, 0, len(records))
	for _, record := range records {
		parcel, err := p.translator.Translate(record)
		if err != nil {
			result.Errors++
			p.logger.Printf("batch %d parcel %s translation error: %v", batchIndex, record.ParcelID, err)
			continue
		}
		parcels = append(parcels, parcel)
		result.Translated++
	}

	storeResult, err := p.store.InsertBatch(ctx, parcels)
	if err != nil {
		return result, err
	}
	result.Inserted = storeResult.Inserted
	for _, insertErr := range storeResult.Errors {
		result.Errors++
		p.logger.Printf("batch %d parcel %s persistence error: %v", batchIndex, insertErr.ParcelID, insertErr.Err)
	}

	return result, nil
}
