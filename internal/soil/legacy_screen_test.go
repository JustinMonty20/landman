//go:build postgres

package soil

import (
	"context"
	"errors"
	"github.com/JustinMonty20/landman/internal/county"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"time"
)

type ScreeningOptions struct {
	FreshnessInterval time.Duration
	SoilSetID         string
}

func screen(ctx context.Context, pool *pgxpool.Pool, client *http.Client, endpoint string, countyID county.ID, source string, polygons []Polygon, units []MapUnit, options ...ScreeningOptions) (runErr error) {
	var previousHead string
	defer func() {
		if runErr != nil {
			failureCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			failureErr := RecordScreeningFailure(failureCtx, pool, countyID, source, previousHead, runErr)
			runErr = errors.Join(runErr, failureErr)
		}
	}()
	if err := pool.QueryRow(ctx, `SELECT COALESCE((SELECT run_id::text FROM soil_screening_heads WHERE county_id=$1 AND source_name=$2),'')`, countyID.String(), source).Scan(&previousHead); err != nil {
		return err
	}
	if err := validateMembership(countyID, source, polygons, units); err != nil {
		return err
	}
	interval := DefaultFreshnessInterval
	if len(options) != 1 || options[0].SoilSetID == "" {
		return errors.New("soil: screening requires the committed soil set ID")
	}
	if options[0].FreshnessInterval != 0 {
		interval = options[0].FreshnessInterval
	}
	if interval <= 0 {
		return errors.New("soil: freshness interval must be positive")
	}
	keys := make([]string, 0, len(polygons))
	for _, polygon := range polygons {
		keys = append(keys, polygon.MapUnitKey)
	}
	values, err := FetchInterpretations(ctx, client, endpoint, keys)
	if err != nil {
		return err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if err := PublishScreeningTx(ctx, tx, countyID, source, options[0].SoilSetID, polygons, values, endpoint, interval, time.Now().UTC()); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func screenStored(ctx context.Context, pool *pgxpool.Pool, client *http.Client, endpoint string, countyID county.ID, source string, interval time.Duration) (int, error) {
	setID, polygons, units, err := StoredSet(ctx, pool, countyID, source)
	if err != nil {
		return 0, err
	}
	err = screen(ctx, pool, client, endpoint, countyID, source, polygons, units, ScreeningOptions{SoilSetID: setID, FreshnessInterval: interval})
	return len(polygons), err
}
