package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/JustinMonty20/landman/internal/connector"
	"github.com/JustinMonty20/landman/internal/connector/union"
	"github.com/JustinMonty20/landman/internal/service"
	"github.com/JustinMonty20/landman/internal/storage"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	gisSource, err := union.NewGISSource(union.DefaultGISSourceConfig())
	if err != nil {
		log.Fatalf("Failed to create GIS source: %v", err)
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		log.Fatalf("Failed to create Postgres pool: %v", err)
	}
	defer pool.Close()

	parcelStore := storage.NewPostgresParcelStore(pool)
	parcelPersister, err := service.NewParcelBatchPersister(union.NewGISTranslator(), parcelStore, log.Default())
	if err != nil {
		log.Fatalf("Failed to create parcel persister: %v", err)
	}

	fmt.Println("Streaming parcel batches from Union County GIS...")
	var totalParcels int
	var totalIDs int
	batchIndex := 0
	err = gisSource.FetchBatches(ctx, func(batch []connector.RawRecord) error {
		batchIndex++
		totalParcels += len(batch)
		_, parcelIDs := eligibleParcelRecords(batch, union.IsLikelyVacantGISRecord)
		totalIDs += len(parcelIDs)

		sampleSize := min(3, len(parcelIDs))

		fmt.Printf("Batch %d: %d parcel IDs", batchIndex, len(parcelIDs))
		if sampleSize > 0 {
			fmt.Printf(" (sample: %v)", parcelIDs[:sampleSize])
		}
		fmt.Println()

		persistResult, err := parcelPersister.PersistRawBatch(ctx, batchIndex, batch)
		if err != nil {
			return fmt.Errorf("persist batch %d: %w", batchIndex, err)
		}
		fmt.Printf("Batch %d: persisted %d/%d normalized GIS parcels (%d errors)\n", batchIndex, persistResult.Inserted, persistResult.Translated, persistResult.Errors)

		return nil
	})
	if err != nil {
		log.Fatalf("Failed to stream parcel batches: %v", err)
	}

	fmt.Printf("Total parcels streamed: %d\n", totalParcels)
	fmt.Printf("Total likely-vacant parcel IDs streamed: %d\n", totalIDs)
}

func eligibleParcelRecords(records []connector.RawRecord, filter service.ParcelRecordFilter) ([]connector.RawRecord, []string) {
	if len(records) == 0 {
		return nil, nil
	}

	eligible := make([]connector.RawRecord, 0, len(records))
	ids := make([]string, 0, len(records))
	seen := make(map[string]struct{}, len(records))
	for _, record := range records {
		if filter != nil && !filter(record) {
			continue
		}
		eligible = append(eligible, record)
		if record.ParcelID == "" {
			continue
		}
		if _, exists := seen[record.ParcelID]; exists {
			continue
		}
		seen[record.ParcelID] = struct{}{}
		ids = append(ids, record.ParcelID)
	}
	return eligible, ids
}
