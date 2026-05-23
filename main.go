package main

import (
	"context"
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/JustinMonty20/landman/internal/connector"
	"github.com/JustinMonty20/landman/internal/connector/union"
	"github.com/JustinMonty20/landman/internal/connector/union/spatialist"
	"github.com/JustinMonty20/landman/internal/service"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	gisSource, err := union.NewGISSource(union.DefaultGISSourceConfig())
	if err != nil {
		log.Fatalf("Failed to create GIS source: %v", err)
	}

	spatialistClient, err := spatialist.NewUnionCountySpatialist(spatialist.DefaultSpatialistConfig())
	if err != nil {
		log.Fatalf("Failed to create Spatialist fetcher: %v", err)
	}

	fetchers := map[string]connector.SingleParcelFetcher{
		spatialistClient.Name(): spatialistClient,
	}

	fetcherConfigs := map[string]service.SourceConfig{}
	for name := range fetchers {
		fetcherConfigs[name] = service.SourceConfig{Concurrency: 5}
	}

	enricher, err := service.NewBatchEnricher(fetchers, fetcherConfigs, log.Default())
	if err != nil {
		log.Fatalf("Failed to create batch enricher: %v", err)
	}

	spatialistEnricher, err := spatialist.NewSpatialistEnricher(
		enricher,
		spatialistClient.Name(),
		spatialist.FlattenSpatialistRecord,
		spatialist.WithRecordFilter(spatialist.KeepWhenYearBuiltIsNil),
	)
	if err != nil {
		log.Fatalf("Failed to create spatialist enricher: %v", err)
	}

	fmt.Println("Streaming parcel IDs from Union County GIS...")
	importer := service.NewParcelIDImportService(
		gisSource,
		50,
		service.WithParcelRecordFilter(union.IsLikelyVacantGISRecord),
	)

	var totalIDs int
	err = importer.Run(ctx, func(ctx context.Context, batchIndex int, parcelIDs []string) error {
		totalIDs += len(parcelIDs)

		sampleSize := min(3, len(parcelIDs))

		fmt.Printf("Batch %d: %d parcel IDs", batchIndex, len(parcelIDs))
		if sampleSize > 0 {
			fmt.Printf(" (sample: %v)", parcelIDs[:sampleSize])
		}
		fmt.Println()

		records, err := spatialistEnricher.EnrichBatch(ctx, parcelIDs)
		if err != nil {
			log.Printf("Batch %d spatialist enrichment error: %v", batchIndex, err)
		}

		nextSourceParcelIDs := sortedParcelIDs(records)
		fmt.Printf("Batch %d: %d parcels survived spatialist YEARBLT filter\n", batchIndex, len(nextSourceParcelIDs))
		fmt.Printf("Batch %d next-source parcelIDs: %v\n", batchIndex, nextSourceParcelIDs)
		return nil
	})
	if err != nil {
		log.Fatalf("Failed to stream parcel IDs: %v", err)
	}

	fmt.Printf("Total likely-vacant parcel IDs streamed: %d\n", totalIDs)
}

func sortedParcelIDs(records map[string]*spatialist.SpatialistFlatEnvelope) []string {
	if len(records) == 0 {
		return nil
	}

	ids := make([]string, 0, len(records))
	for parcelID := range records {
		if parcelID == "" {
			continue
		}
		ids = append(ids, parcelID)
	}

	sort.Strings(ids)
	return ids
}
