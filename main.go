package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/JustinMonty20/landman/internal/connector/union"
	"github.com/JustinMonty20/landman/internal/service"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	gisSource, err := union.NewGISSource(union.DefaultGISSourceConfig())
	if err != nil {
		log.Fatalf("Failed to create GIS source: %v", err)
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
		return nil
	})
	if err != nil {
		log.Fatalf("Failed to stream parcel IDs: %v", err)
	}

	fmt.Printf("Total likely-vacant parcel IDs streamed: %d\n", totalIDs)
}
