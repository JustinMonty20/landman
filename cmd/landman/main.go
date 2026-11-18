package main

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/JustinMonty20/landman/internal/connector/union"
	"github.com/JustinMonty20/landman/internal/connector/union/parceljob"
	"github.com/JustinMonty20/landman/internal/connector/union/soiljob"
	"github.com/JustinMonty20/landman/internal/connector/union/taxjob"
	"github.com/JustinMonty20/landman/internal/county"
	"github.com/JustinMonty20/landman/internal/jobs"
	"github.com/JustinMonty20/landman/internal/qualification"
	"github.com/JustinMonty20/landman/internal/soil"
	"github.com/JustinMonty20/landman/internal/taxartifact"
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
	parcelJobID        = parceljob.JobID
	soilJobID          = "union.gis.soil"
	screeningJobID     = "union.soil.screening"
	taxJobID           = "union.tax"
	zoningJobID        = "union.gis.zoning"
	roadsJobID         = "union.gis.roads"
	qualificationJobID = "union.qualification.assess"
	jobTimeout         = 10 * time.Minute
)

// unionExecutor preserves source outcomes when a separate downstream attempt fails.
type unionExecutor struct {
	runner      requiredRunner
	refresh     func(context.Context) error
	afterSource func(context.Context, string) error
	qualify     func(context.Context) error
}

func (e unionExecutor) Run(ctx context.Context, definition jobs.Definition) (jobs.Outcome, jobs.Result, error) {
	ctx, cancel := context.WithTimeout(ctx, jobTimeout)
	defer cancel()
	outcome, result, err := e.runner.Run(ctx, definition)
	if err == nil && outcome == jobs.OutcomeSuccess {
		if definition.ID == qualificationJobID {
			return outcome, result, nil
		}
		if e.afterSource != nil {
			err = e.afterSource(ctx, definition.ID)
		} else {
			err = e.refresh(ctx)
		}
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
		case parcelJobID, soilJobID, screeningJobID, taxJobID, zoningJobID, roadsJobID, qualificationJobID:
			return args[1], nil
		}
	}
	if len(args) == 4 && args[0] == "replay" && (args[1] == parcelJobID || args[1] == taxJobID || args[1] == soilJobID || args[1] == screeningJobID) && args[2] == "--snapshot" {
		if !validCaptureID(args[3]) {
			return "", fmt.Errorf("snapshot ID must be a capture UUID")
		}
		if args[1] == taxJobID {
			return "tax-replay", nil
		}
		if args[1] == soilJobID {
			return "soil-replay", nil
		}
		if args[1] == screeningJobID {
			return "screening-replay", nil
		}
		return "replay", nil
	}
	if len(args) == 4 && args[0] == "reprocess" && args[1] == parcelJobID && args[2] == "--limit" {
		if _, err := parceljob.ReprocessLimit(args[3]); err != nil {
			return "", err
		}
		return "reprocess", nil
	}
	return "", fmt.Errorf("usage: landman [schedule | run <job ID> | replay <%s|%s|%s|%s> --snapshot UUID | reprocess %s --limit N]", parcelJobID, taxJobID, soilJobID, screeningJobID, parcelJobID)
}

func validCaptureID(id string) bool {
	if len(id) != 36 || id[8] != '-' || id[13] != '-' || id[18] != '-' || id[23] != '-' {
		return false
	}
	_, err := hex.DecodeString(strings.ReplaceAll(id, "-", ""))
	return err == nil
}

func schedules(getenv func(string) string) (map[string]string, error) {
	values := make(map[string]string, 5)
	for _, entry := range []struct{ id, env string }{
		{parcelJobID, "UNION_PARCELS_CRON"}, {soilJobID, "UNION_SOIL_CRON"}, {zoningJobID, "UNION_ZONING_CRON"}, {roadsJobID, "UNION_ROADS_CRON"}, {qualificationJobID, "UNION_QUALIFICATION_CRON"},
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

// manualTaxConfig reads only the explicit manual job inputs.
func manualTaxConfig(getenv func(string) string) (taxjob.Config, error) {
	config := taxjob.Config{BillingSourceURL: union.BillingURL, PipelineSourceURL: union.PipelineURL}
	for _, entry := range []struct {
		name  string
		value *string
	}{
		{"UNION_TAX_ARTIFACT_DIR", &config.ArtifactDir},
		{"UNION_TAX_DELINQUENT_PATH", &config.DelinquentPath},
		{"UNION_TAX_BILLING_PATH", &config.BillingPath},
		{"UNION_TAX_FORECLOSURE_PATH", &config.ForeclosurePath},
		{"UNION_TAX_PIPELINE_PATH", &config.PipelinePath},
	} {
		*entry.value = getenv(entry.name)
		if *entry.value == "" {
			return taxjob.Config{}, fmt.Errorf("%s is required for %s", entry.name, taxJobID)
		}
	}
	return config, nil
}

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
	var taxInput taxjob.Config
	if mode == taxJobID || mode == "all" {
		taxInput, err = manualTaxConfig(os.Getenv)
		if err != nil {
			return err
		}
	}
	if mode == "tax-replay" {
		taxInput.ArtifactDir = os.Getenv("UNION_TAX_ARTIFACT_DIR")
		if taxInput.ArtifactDir == "" {
			return fmt.Errorf("UNION_TAX_ARTIFACT_DIR is required for %s replay", taxJobID)
		}
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
	qstore := qualification.Store{Pool: pool}
	countyRequest := func(ctx context.Context, fn func(context.Context) (jobs.Result, error)) (jobs.Result, error) {
		var result jobs.Result
		err := withCountyHostLock(ctx, dsn, func(ctx context.Context) error { var e error; result, e = fn(ctx); return e })
		return result, err
	}
	artifactRoot := os.Getenv("PARCEL_EVIDENCE_DIR")
	if artifactRoot == "" && mode != taxJobID && mode != "tax-replay" && mode != qualificationJobID {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		artifactRoot = home + "/.local/share/landman/parcel-evidence"
	}
	var soilArtifacts taxartifact.Store
	if mode != taxJobID && mode != "tax-replay" && mode != qualificationJobID && mode != "replay" && mode != "reprocess" {
		soilArtifacts, err = taxartifact.NewLocal(artifactRoot)
		if err != nil {
			return err
		}
	}
	geometryJob := soiljob.Job{Source: gisSource, Pool: pool, Artifacts: soilArtifacts}
	screeningJob := soiljob.ScreeningJob{Pool: pool, Artifacts: soilArtifacts, Client: http.DefaultClient, Interval: interval}
	parcelJob := parceljob.Job{Root: artifactRoot, DSN: dsn, Source: gisSource, Quarantine: jobs.NewQuarantineStore(pool)}
	definitions := []jobs.Definition{
		{ID: parcelJobID, Schedule: calendar[parcelJobID], Run: func(ctx context.Context) (jobs.Result, error) {
			return countyRequest(ctx, func(ctx context.Context) (jobs.Result, error) {
				return parcelJob.Import(ctx)
			})
		}},
		{ID: soilJobID, Schedule: calendar[soilJobID], Run: func(ctx context.Context) (jobs.Result, error) {
			return countyRequest(ctx, func(ctx context.Context) (jobs.Result, error) {
				return geometryJob.Import(ctx)
			})
		}},
	}
	if mode == "schedule" || mode == zoningJobID || mode == roadsJobID {
		for _, entry := range []struct{ id, group string }{{zoningJobID, union.QualificationZoning}, {roadsJobID, union.QualificationRoads}} {
			group := entry.group
			definitions = append(definitions, jobs.Definition{ID: entry.id, Schedule: calendar[entry.id], Run: func(ctx context.Context) (jobs.Result, error) {
				n, e := qstore.RefreshWithGate(ctx, gisSource, group, func(ctx context.Context, work func(context.Context) error) error {
					return withCountyHostLock(ctx, dsn, work)
				})
				return jobs.Result{Processed: n}, e
			}})
		}
	}

	if mode == taxJobID || mode == "all" {
		definitions = append(definitions, jobs.Definition{ID: taxJobID, Run: func(ctx context.Context) (jobs.Result, error) {
			return taxjob.Run(ctx, pool, taxInput)
		}})
	}
	refresh := func(ctx context.Context) error {
		return withRefreshLock(ctx, dsn, func(ctx context.Context) error {
			requests, err := observedHandoffs(ctx, pool, "screening_pending")
			if err != nil {
				return err
			}
			err = runDerived(ctx, runner, jobs.Definition{ID: screeningJobID, Run: func(ctx context.Context) (jobs.Result, error) {
				result, err := screeningJob.Import(ctx)
				if errors.Is(err, soil.ErrNoStoredSet) {
					err = fmt.Errorf("%w: %w", jobs.ErrPending, err)
				}
				return result, err
			}})
			if err != nil {
				return err
			}
			return acknowledgeHandoffs(ctx, pool, "screening_pending", requests)
		})
	}
	qualificationRefresh := func(ctx context.Context) error {
		return withQualificationLock(ctx, dsn, func(ctx context.Context) error {
			requests, err := observedHandoffs(ctx, pool, "qualification_pending")
			if err != nil {
				return err
			}
			err = runDerived(ctx, runner, jobs.Definition{ID: qualificationJobID, Run: func(ctx context.Context) (jobs.Result, error) {
				n, e := qstore.Reconcile(ctx)
				return jobs.Result{Processed: n}, e
			}})
			if err != nil {
				return err
			}
			return acknowledgeHandoffs(ctx, pool, "qualification_pending", requests)
		})
	}
	executor := unionExecutor{runner: runner, refresh: refresh, qualify: qualificationRefresh, afterSource: func(ctx context.Context, id string) error {
		if id == parcelJobID {
			due, err := soil.NeedsScreenStored(ctx, pool, county.Union, gisSource.Name())
			if err != nil {
				return err
			}
			var screenErr error
			if due {
				screenErr = refresh(ctx)
			}
			return errors.Join(screenErr, qualificationRefresh(ctx))
		}
		if id == soilJobID {
			screeningError := refresh(ctx)
			qualificationError := qualificationRefresh(ctx)
			return errors.Join(screeningError, qualificationError)
		}
		return qualificationRefresh(ctx)
	}}
	if mode == "soil-replay" || mode == "screening-replay" {
		id := soilJobID
		replay := geometryJob.Replay
		if mode == "screening-replay" {
			id = screeningJobID
			replay = screeningJob.Replay
		}
		outcome, _, err := runner.Run(ctx, jobs.Definition{ID: id, Kind: jobs.AttemptReplay, Run: func(ctx context.Context) (jobs.Result, error) { return jobs.Result{}, replay(ctx, os.Args[4]) }})
		if err != nil {
			return err
		}
		if outcome != jobs.OutcomeSuccess {
			return fmt.Errorf("soil replay did not validate: %s", outcome)
		}
		return nil
	}
	if mode == "tax-replay" {
		outcome, _, err := runner.Run(ctx, jobs.Definition{ID: taxJobID, Kind: jobs.AttemptReplay, Run: func(ctx context.Context) (jobs.Result, error) {
			return taxjob.Replay(ctx, pool, taxInput.ArtifactDir, os.Args[4])
		}})
		if err != nil {
			return err
		}
		if outcome != jobs.OutcomeSuccess && outcome != jobs.OutcomeSuccessWithQuarantine {
			return fmt.Errorf("tax replay did not validate: %s", outcome)
		}
		return nil
	}
	if mode == "replay" {
		outcome, _, err := runner.Run(ctx, jobs.Definition{ID: parcelJobID, Kind: jobs.AttemptReplay, Run: func(ctx context.Context) (jobs.Result, error) { return parcelJob.Replay(ctx, os.Args[4]) }})
		if err != nil {
			return err
		}
		if outcome != jobs.OutcomeSuccess && outcome != jobs.OutcomeSuccessWithQuarantine {
			return fmt.Errorf("replay did not validate: %s", outcome)
		}
		// Independent stored-input jobs discover stale dependencies later.
		return nil
	}
	if mode == "reprocess" {
		limit, _ := parceljob.ReprocessLimit(os.Args[4])
		return runRequired(ctx, executor, jobs.Definition{ID: parcelJobID, Kind: jobs.AttemptReprocess, Run: func(ctx context.Context) (jobs.Result, error) {
			return countyRequest(ctx, func(ctx context.Context) (jobs.Result, error) {
				return parcelJob.Reprocess(ctx, limit)
			})
		}})
	}
	if mode == "schedule" {
		definitions = append(definitions, jobs.Definition{ID: qualificationJobID, Schedule: calendar[qualificationJobID], Run: func(ctx context.Context) (jobs.Result, error) {
			requests, e := observedHandoffs(ctx, pool, "qualification_pending")
			if e != nil {
				return jobs.Result{}, e
			}
			n, e := qstore.Reconcile(ctx)
			if e == nil {
				e = acknowledgeHandoffs(ctx, pool, "qualification_pending", requests)
			}
			return jobs.Result{Processed: n}, e
		}})
		scheduler, err := jobs.NewScheduler(executor, definitions)
		if err != nil {
			return err
		}
		startupCtx, startupCancel := context.WithTimeout(ctx, jobTimeout)
		if err := qualificationRefresh(startupCtx); err != nil {
			log.Printf("qualification startup reconciliation: %v", err)
		}
		startupCancel()
		scheduler.Start()
		<-ctx.Done()
		<-scheduler.Stop().Done()
		return nil
	}
	if mode == qualificationJobID {
		return qualificationRefresh(ctx)
	}
	if mode == taxJobID {
		return runRequired(ctx, runner, definitions[len(definitions)-1])
	}
	return runManual(ctx, mode, runner, executor, definitions, refresh)
}

type handoffGeneration struct {
	artifact   string
	generation int64
}

// Snapshot only requests visible before the derived work. A publication
// arriving during that work must remain pending even if its clock is older.
func observedHandoffs(ctx context.Context, pool *pgxpool.Pool, column string) ([]handoffGeneration, error) {
	if column != "screening_pending" && column != "qualification_pending" {
		return nil, fmt.Errorf("invalid handoff column")
	}
	rows, err := pool.Query(ctx, `SELECT artifact_id,request_generation FROM parcel_derived_handoff WHERE `+column)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var requests []handoffGeneration
	for rows.Next() {
		var request handoffGeneration
		if err := rows.Scan(&request.artifact, &request.generation); err != nil {
			return nil, err
		}
		requests = append(requests, request)
	}
	return requests, rows.Err()
}

func acknowledgeHandoffs(ctx context.Context, pool *pgxpool.Pool, column string, requests []handoffGeneration) error {
	if column != "screening_pending" && column != "qualification_pending" {
		return fmt.Errorf("invalid handoff column")
	}
	if len(requests) == 0 {
		return nil
	}
	ids := make([]string, 0, len(requests))
	generations := make([]int64, 0, len(requests))
	for _, request := range requests {
		ids = append(ids, request.artifact)
		generations = append(generations, request.generation)
	}
	_, err := pool.Exec(ctx, `UPDATE parcel_derived_handoff SET `+column+`=false WHERE `+column+` AND (artifact_id,request_generation) IN (SELECT * FROM unnest($1::text[],$2::bigint[]))`, ids, generations)
	return err
}

func runManual(ctx context.Context, mode string, runner requiredRunner, executor unionExecutor, definitions []jobs.Definition, refresh func(context.Context) error) error {
	if mode == screeningJobID {
		return refresh(ctx)
	}
	for _, definition := range definitions {
		if mode == "all" {
			if definition.ID == taxJobID {
				if err := refresh(ctx); err != nil {
					return err
				}
				return runRequired(ctx, runner, definition)
			}
			if err := runRequired(ctx, runner, definition); err != nil {
				return err
			}
			if executor.qualify != nil {
				if err := executor.qualify(ctx); err != nil {
					return err
				}
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

// Cross-process host exclusion keeps distinct manual and scheduled county jobs
// under the same request budget without changing the scheduler.
func withCountyHostLock(ctx context.Context, dsn string, fn func(context.Context) error) error {
	return withAdvisoryLock(ctx, dsn, "county-host:atlas.unioncountync.gov", func(ctx context.Context) error {
		timer := time.NewTimer(time.Second)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return fn(ctx)
		}
	})
}
func withQualificationLock(ctx context.Context, dsn string, fn func(context.Context) error) error {
	return withAdvisoryLock(ctx, dsn, "qualification-refresh:union", fn)
}
func withAdvisoryLock(ctx context.Context, dsn, key string, fn func(context.Context) error) error {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return err
	}
	conn, err := pgx.ConnectConfig(ctx, config.ConnConfig)
	if err != nil {
		return err
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = conn.Close(closeCtx)
	}()
	if _, err = conn.Exec(ctx, `SELECT pg_advisory_lock(hashtextextended($1,0))`, key); err != nil {
		return fmt.Errorf("wait for %s: %w", key, err)
	}
	workCtx, cancel := context.WithCancelCause(ctx)
	monitorDone := make(chan struct{})
	go func() {
		defer close(monitorDone)
		if err := conn.PgConn().WaitForNotification(workCtx); err != nil && workCtx.Err() == nil {
			cancel(fmt.Errorf("%s lock session lost: %w", key, err))
		} else if err == nil && workCtx.Err() == nil {
			cancel(fmt.Errorf("%s lock session received unexpected notification", key))
		}
	}()
	result := fn(workCtx)
	cause := context.Cause(workCtx)
	cancel(nil)
	<-monitorDone
	if cause != nil {
		return cause
	}
	return result
}
