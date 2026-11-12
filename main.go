package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/JustinMonty20/landman/internal/connector"
	"github.com/JustinMonty20/landman/internal/connector/union"
	"github.com/JustinMonty20/landman/internal/parcelstore"
	"github.com/JustinMonty20/landman/internal/service"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	config := union.DefaultGISSourceConfig()
	config.BatchSize = 2000
	gisSource, err := union.NewGISSource(config)
	if err != nil {
		log.Fatalf("Create GIS source: %v", err)
	}
	writer, err := parcelstore.NewShapeWriter(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatalf("Create parcel writer: %v", err)
	}
	defer writer.Close()
	translator := union.NewGISTranslator()
	importer := service.NewParcelIDImportService(gisSource, 2000,
		service.WithParcelRecordHook(func(ctx context.Context, records []connector.RawRecord) error {
			for _, record := range records {
				vacant := union.IsLikelyVacantGISRecord(record)
				attrs, _ := record.RawData["attributes"].(map[string]interface{})
				pid, _ := attrs["PID"].(string)
				if pid == "" {
					vacant = false
				}
				parcel, err := translator.Translate(record)
				if err != nil {
					return err
				}
				if err := writer.SavePart(ctx, *parcel, vacant); err != nil {
					return err
				}
			}
			return nil
		}),
		service.WithParcelRecordFilter(union.IsLikelyVacantGISRecord),
	)
	var totalIDs int
	err = importer.Run(ctx, func(_ context.Context, _ int, ids []string) error {
		totalIDs += len(ids)
		return nil
	})
	if err != nil {
		log.Fatalf("Import parcels: %v", err)
	}
	if err := writer.Commit(ctx); err != nil {
		log.Fatalf("Commit parcels: %v", err)
	}
	fmt.Printf("Imported parcels, %d likely-vacant feature IDs\n", totalIDs)
}
