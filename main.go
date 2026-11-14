package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/JustinMonty20/landman/internal/connector"
	"github.com/JustinMonty20/landman/internal/connector/union"
	"github.com/JustinMonty20/landman/internal/county"
	"github.com/JustinMonty20/landman/internal/jobs"
	"github.com/JustinMonty20/landman/internal/parcelstore"
	"github.com/JustinMonty20/landman/internal/service"
	"github.com/JustinMonty20/landman/internal/soil"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if err := run(ctx); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context) error {
	dsn := os.Getenv("DATABASE_URL")
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()
	runner, err := jobs.NewRunner(pool)
	if err != nil {
		return err
	}
	if err = runner.RecoverInterrupted(ctx); err != nil {
		return err
	}
	interval := soil.DefaultFreshnessInterval
	if text := os.Getenv("SOIL_SCREENING_MAX_AGE"); text != "" {
		interval, err = time.ParseDuration(text)
		if err != nil || interval <= 0 {
			return fmt.Errorf("invalid SOIL_SCREENING_MAX_AGE %q", text)
		}
	}
	config := union.DefaultGISSourceConfig()
	config.BatchSize = 2000
	gisSource, err := union.NewGISSource(config)
	if err != nil {
		return fmt.Errorf("create GIS source: %w", err)
	}
	var polygons []soil.Polygon
	var units []soil.MapUnit
	var soilSetID string
	outcome, _, err := runner.Run(ctx, jobs.Definition{ID: "union.gis.parcels", Run: func(ctx context.Context) (jobs.Result, error) {
		var err error
		polygons, units, err = gisSource.FetchSoilPolygons(ctx)
		if err != nil {
			return jobs.Result{}, fmt.Errorf("fetch soil polygons: %w", err)
		}
		count, err := importParcels(ctx, dsn, gisSource)
		if err != nil {
			return jobs.Result{}, err
		}
		soilSetID, err = soil.StoreWithSet(ctx, dsn, county.Union, gisSource.Name(), polygons, units)
		if err != nil {
			return jobs.Result{Processed: count}, fmt.Errorf("store soil geometry: %w", err)
		}
		return jobs.Result{Processed: count}, nil
	}})
	if err != nil {
		return err
	}
	if outcome != jobs.OutcomeSuccess {
		return fmt.Errorf("source import did not complete cleanly: %s", outcome)
	}
	outcome, _, err = runner.Run(ctx, jobs.Definition{ID: "union.soil.screening", Run: func(ctx context.Context) (jobs.Result, error) {
		err := soil.Screening(ctx, dsn, county.Union, gisSource.Name(), polygons, units, soil.ScreeningOptions{FreshnessInterval: interval, SoilSetID: soilSetID})
		return jobs.Result{Processed: len(polygons)}, err
	}})
	if err != nil {
		return fmt.Errorf("required soil screening: %w", err)
	}
	if outcome != jobs.OutcomeSuccess {
		return fmt.Errorf("required soil screening did not complete cleanly: %s", outcome)
	}
	return nil
}

func importParcels(ctx context.Context, dsn string, gisSource *union.GISSource) (int, error) {
	writer, err := parcelstore.NewShapeWriter(ctx, dsn)
	if err != nil {
		return 0, fmt.Errorf("create parcel writer: %w", err)
	}
	defer writer.Close()
	translator := union.NewGISTranslator()
	var processed int
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
				processed++
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
		return 0, fmt.Errorf("import parcels: %w", err)
	}
	if err := writer.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit parcels: %w", err)
	}
	fmt.Printf("Imported parcels, %d likely-vacant feature IDs\n", totalIDs)
	return processed, nil
}
