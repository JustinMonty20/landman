package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/JustinMonty20/landman/internal/connector"
	"github.com/JustinMonty20/landman/internal/connector/union"
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

	// Step 3: Fetch all data
	// The DataSource handles pagination/batching internally
	fmt.Println("Fetching parcel data from Union County GIS...")
	rawRecords, err := gisSource.Fetch(ctx)
	if err != nil {
		log.Fatalf("Failed to fetch data: %v", err)
	}

	fmt.Printf("Fetched %d raw records\n", len(rawRecords))

	// Step 4: Get the translator for Union County
	translators := connector.GetTranslatorsForCounty("union")
	if len(translators) == 0 {
		log.Fatal("No translator found for Union County")
	}
	translator := translators[0]

	// Step 5: Translate raw records to normalized Parcels
	fmt.Println("Translating raw records to normalized Parcels...")
	var parcels []*connector.RawRecord
	for i := range rawRecords {
		// Translate each record
		_, err := translator.Translate(rawRecords[i])
		if err != nil {
			log.Printf("Warning: failed to translate record %s: %v", rawRecords[i].ParcelID, err)
			continue
		}
		parcels = append(parcels, &rawRecords[i])
	}

	fmt.Printf("Successfully translated %d parcels\n", len(parcels))

	// Step 6: Show sample of first few parcels
	sampleSize := 5
	if len(parcels) < sampleSize {
		sampleSize = len(parcels)
	}

	fmt.Printf("\nSample of first %d parcels:\n", sampleSize)
	for i := 0; i < sampleSize; i++ {
		fmt.Printf("  Parcel %d: ID=%s, Source=%s, FetchedAt=%s\n",
			i+1,
			parcels[i].ParcelID,
			parcels[i].SourceName,
			parcels[i].FetchedAt.Format(time.RFC3339),
		)
	}

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
