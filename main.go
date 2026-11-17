package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/JustinMonty20/landman/internal/connector/union"
	"github.com/JustinMonty20/landman/internal/connector/union/taxjob"
	"github.com/JustinMonty20/landman/internal/county"
	"github.com/JustinMonty20/landman/internal/jobs"
	"github.com/JustinMonty20/landman/internal/soil"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/robfig/cron/v3"
)

type requiredRunner interface {
	Run(context.Context, jobs.Definition) (jobs.Outcome, jobs.Result, error)
}

func runRequired(ctx context.Context, runner requiredRunner, definition jobs.Definition) error {
	outcome, _, err := runner.Run(ctx, definition)
	if err != nil {
		return fmt.Errorf("required %s: %w", definition.ID, err)
	}
	if outcome != jobs.OutcomeSuccess {
		return fmt.Errorf("required %s did not complete cleanly: %s", definition.ID, outcome)
	}
	return nil
}

const (
	parcelJobID    = "union.gis.parcels"
	soilJobID      = "union.gis.soil"
	screeningJobID = "union.soil.screening"
	taxJobID       = "union.tax"
	jobTimeout     = 10 * time.Minute
)

// unionExecutor preserves source outcomes when a separate downstream attempt fails.
type unionExecutor struct {
	runner  requiredRunner
	refresh func(context.Context) error
}

func (e unionExecutor) Run(ctx context.Context, definition jobs.Definition) (jobs.Outcome, jobs.Result, error) {
	ctx, cancel := context.WithTimeout(ctx, jobTimeout)
	defer cancel()
	outcome, result, err := e.runner.Run(ctx, definition)
	if err == nil && outcome == jobs.OutcomeSuccess {
		err = e.refresh(ctx)
	}
	return outcome, result, err
}

func command(args []string) (string, error) {
	if len(args) == 0 {
		return "all", nil
	}
	if len(args) == 1 && args[0] == "schedule" {
		return "schedule", nil
	}
	if len(args) == 2 && args[0] == "run" {
		switch args[1] {
		case parcelJobID, soilJobID, screeningJobID, taxJobID:
			return args[1], nil
		}
	}
	if len(args) == 4 && args[0] == "reprocess" && args[1] == parcelJobID && args[2] == "--limit" {
		if _, err := reprocessLimit(args[3]); err != nil {
			return "", err
		}
		return "reprocess", nil
	}
	return "", fmt.Errorf("usage: landman [schedule | run <%s|%s|%s|%s> | reprocess %s --limit N]", parcelJobID, soilJobID, screeningJobID, taxJobID, parcelJobID)
}

func schedules(getenv func(string) string) (map[string]string, error) {
	values := make(map[string]string, 2)
	for _, entry := range []struct{ id, env string }{
		{parcelJobID, "UNION_PARCELS_CRON"}, {soilJobID, "UNION_SOIL_CRON"},
	} {
		text := getenv(entry.env)
		fields := strings.Fields(text)
		if len(fields) != 6 || !strings.HasPrefix(fields[0], "CRON_TZ=") || fields[0] == "CRON_TZ=" {
			return nil, fmt.Errorf("%s requires CRON_TZ and five calendar fields", entry.env)
		}
		text = strings.Join(fields, " ")
		if _, err := cron.ParseStandard(text); err != nil {
			return nil, fmt.Errorf("%s: %w", entry.env, err)
		}
		values[entry.id] = text
	}
	return values, nil
}

// Tax files are supplied by the operator. Select private paths before enabling the manual job.
type taxRunConfig struct {
	Enabled bool
	taxjob.Config
}

var taxConfig taxRunConfig

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context) error {
	mode, err := command(os.Args[1:])
	if err != nil {
		return err
	}
	var calendar map[string]string
	if mode == "schedule" {
		calendar, err = schedules(os.Getenv)
		if err != nil {
			return err
		}
	} else {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, jobTimeout)
		defer cancel()
	}

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
	definitions := []jobs.Definition{
		{ID: parcelJobID, Schedule: calendar[parcelJobID], Run: func(ctx context.Context) (jobs.Result, error) {
			return importParcels(ctx, dsn, gisSource, jobs.NewQuarantineStore(pool))
		}},
		{ID: soilJobID, Schedule: calendar[soilJobID], Run: func(ctx context.Context) (jobs.Result, error) {
			polygons, units, err := gisSource.FetchSoilPolygons(ctx)
			if err != nil {
				return jobs.Result{}, fmt.Errorf("fetch soil polygons: %w", err)
			}
			_, err = soil.StoreWithSet(ctx, dsn, county.Union, gisSource.Name(), polygons, units)
			return jobs.Result{Processed: len(polygons)}, err
		}},
	}
	if taxConfig.Enabled && mode != "schedule" {
		definitions = append(definitions, jobs.Definition{ID: taxJobID, Run: func(ctx context.Context) (jobs.Result, error) {
			return taxjob.Run(ctx, pool, taxConfig.Config)
		}})
	}
	if mode == taxJobID && !taxConfig.Enabled {
		return errors.New("union.tax is disabled")
	}
	refresh := func(ctx context.Context) error {
		return withRefreshLock(ctx, dsn, func(ctx context.Context) error {
			return runDerived(ctx, runner, jobs.Definition{ID: screeningJobID, Run: func(ctx context.Context) (jobs.Result, error) {
				count, err := soil.ScreenStored(ctx, dsn, county.Union, gisSource.Name(), interval)
				if errors.Is(err, soil.ErrNoStoredSet) {
					err = fmt.Errorf("%w: %w", jobs.ErrPending, err)
				}
				return jobs.Result{Processed: count}, err
			}})
		})
	}
	executor := unionExecutor{runner: runner, refresh: refresh}
	if mode == "reprocess" {
		limit, _ := reprocessLimit(os.Args[4])
		return runRequired(ctx, executor, jobs.Definition{ID: parcelJobID, Kind: jobs.AttemptReprocess, Run: func(ctx context.Context) (jobs.Result, error) {
			return reprocessParcels(ctx, dsn, gisSource, jobs.NewQuarantineStore(pool), limit)
		}})
	}
	if mode == "schedule" {
		scheduler, err := jobs.NewScheduler(executor, definitions)
		if err != nil {
			return err
		}
		scheduler.Start()
		<-ctx.Done()
		<-scheduler.Stop().Done()
		return nil
	}
	if mode == taxJobID {
		return runRequired(ctx, runner, definitions[len(definitions)-1])
	}
	return runManual(ctx, mode, runner, executor, definitions, refresh)
}

func runManual(ctx context.Context, mode string, runner requiredRunner, executor unionExecutor, definitions []jobs.Definition, refresh func(context.Context) error) error {
	if mode == screeningJobID {
		return refresh(ctx)
	}
	for _, definition := range definitions {
		if mode == "all" {
			if err := runRequired(ctx, runner, definition); err != nil {
				return err
			}
		} else if mode == definition.ID {
			return runRequired(ctx, executor, definition)
		}
	}
	return refresh(ctx)
}

// Retry a derived overlap while the caller's deadline remains. A lost advisory-lock session must not discard a source success.
func runDerived(ctx context.Context, runner requiredRunner, definition jobs.Definition) error {
	for {
		outcome, _, err := runner.Run(ctx, definition)
		if err != nil {
			return fmt.Errorf("required %s: %w", definition.ID, err)
		}
		if outcome == jobs.OutcomeSuccess {
			return nil
		}
		if outcome != jobs.OutcomeSkippedOverlap {
			return fmt.Errorf("required %s did not complete cleanly: %s", definition.ID, outcome)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("required %s: %w", definition.ID, ctx.Err())
		case <-time.After(time.Second):
		}
	}
}

// Wait before claiming the derived job so a concurrent successful source cannot lose its refresh to an overlap skip.
func withRefreshLock(ctx context.Context, dsn string, refresh func(context.Context) error) error {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return err
	}
	conn, err := pgx.ConnectConfig(ctx, config.ConnConfig)
	if err != nil {
		return fmt.Errorf("connect for derived refresh exclusion: %w", err)
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = conn.Close(closeCtx)
	}()
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock(hashtextextended($1,0))`, "soil-refresh:union:union_county_gis"); err != nil {
		return fmt.Errorf("wait for derived refresh exclusion: %w", err)
	}
	return refresh(ctx)
}
