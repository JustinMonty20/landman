package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/JustinMonty20/landman/internal/connector"
	"github.com/JustinMonty20/landman/internal/connector/union"
	"github.com/JustinMonty20/landman/internal/service"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Step 1: Register Union County connectors
	// This registers the GIS DataSource, Translator, and Merger with the global registry
	// Each data source manages its own configuration internally
	if err := union.Register(); err != nil {
		log.Fatalf("Failed to register Union County: %v", err)
	}

	fmt.Println("Union County connectors registered successfully")

	// Step 2: Get the Union County GIS data source from the registry
	gisSource, err := connector.GetSource("union_county_gis")
	if err != nil {
		log.Fatalf("Failed to get GIS source: %v", err)
	}

	spatialistConfig := union.DefaultSpatialistConfig()
	spatialist, err := union.NewUnionCountySpatialist(spatialistConfig)
	if err != nil {
		log.Fatalf("Failed to create Spatialist fetcher: %v", err)
	}

	fetchers := map[string]connector.SingleParcelFetcher{
		spatialist.Name(): spatialist,
	}

	fetcherConfigs := map[string]service.SourceConfig{}
	for name := range fetchers {
		fetcherConfigs[name] = service.SourceConfig{
			Concurrency: 5,
		}
	}

	enricher, err := service.NewBatchEnricher(fetchers, fetcherConfigs, log.Default())
	if err != nil {
		log.Fatalf("Failed to create batch enricher: %v", err)
	}

	spatialistEnricher, err := union.NewSpatialistEnricher(enricher, spatialist.Name(), union.FlattenSpatialistRecord)
	if err != nil {
		log.Fatalf("Failed to create spatialist enricher: %v", err)
	}

	// Step 3: Stream parcel IDs in batches
	fmt.Println("Streaming parcel IDs from Union County GIS...")
	importer := service.NewParcelIDImportService(
		gisSource,
		50,
		service.WithParcelRecordFilter(union.IsLikelyVacantGISRecord),
	)

	var totalIDs int
	err = importer.Run(ctx, func(ctx context.Context, batchIndex int, parcelIDs []string) error {
		totalIDs += len(parcelIDs)

		sampleSize := 3
		if len(parcelIDs) < sampleSize {
			sampleSize = len(parcelIDs)
		}

		fmt.Printf("Batch %d: %d parcel IDs", batchIndex, len(parcelIDs))
		if sampleSize > 0 {
			fmt.Printf(" (sample: %v)", parcelIDs[:sampleSize])
		}
		fmt.Println()

		records, err := spatialistEnricher.EnrichBatch(ctx, parcelIDs)
		if err != nil {
			return err
		}
		sampleID := ""
		if len(parcelIDs) > 0 {
			sampleID = parcelIDs[0]
		}
		if sampleID != "" {
			if record, ok := records[sampleID]; ok && record != nil {
				pretty, err := json.MarshalIndent(record, "", "  ")
				if err != nil {
					return fmt.Errorf("marshal sample record: %w", err)
				}
				fmt.Printf("Batch %d sample enriched record for parcel %s:\n%s\n", batchIndex, sampleID, string(pretty))
			}
		}
		fmt.Printf("Batch %d: enriched %d parcels\n", batchIndex, len(records))
		return nil
	})
	if err != nil {
		log.Fatalf("Failed to stream parcel IDs: %v", err)
	}

	fmt.Printf("Total likely-vacant parcel IDs streamed: %d\n", totalIDs)

	// Future: When we add the Merger and have multiple sources:
	// Step 6: Group parcels by ParcelID
	// Step 7: Merge parcels from different sources using the Merger
	// Step 8: Save to database using repository layer
}

// NOTES FOR FUTURE EXPANSION:
//
// When adding Mecklenburg County:
// 1. Create internal/connector/mecklenburg/ package
// 2. Implement MecklenburgCountyGISSource (DataSource interface)
// 3. Implement MecklenburgCountyTranslator (Translator interface)
// 4. Implement MecklenburgCountyMerger (Merger interface)
// 5. Create mecklenburg.Register() function
// 6. Call mecklenburg.Register() here in main()
//
// When adding Union County Tax source:
// 1. Create internal/connector/union/tax_source.go
// 2. Implement UnionCountyTaxSource (DataSource interface)
// 3. Update union.Register() to register the tax source
// 4. Update UnionCountyMerger to merge GIS + Tax data
//
// The core connector interfaces don't change - only add new implementations!
