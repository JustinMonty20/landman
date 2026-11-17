package main

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/JustinMonty20/landman/internal/connector/union/taxjob"
	"github.com/JustinMonty20/landman/internal/jobs"
)

type fakeRequiredRunner struct {
	outcome jobs.Outcome
	err     error
	calls   int
	id      string
}

func (f *fakeRequiredRunner) Run(_ context.Context, definition jobs.Definition) (jobs.Outcome, jobs.Result, error) {
	f.calls++
	f.id = definition.ID
	return f.outcome, jobs.Result{}, f.err
}

func TestRunRequired(t *testing.T) {
	definition := jobs.Definition{ID: "union.gis.soil", Run: func(context.Context) (jobs.Result, error) {
		return jobs.Result{}, nil
	}}
	for _, tc := range []struct {
		name    string
		outcome jobs.Outcome
		wantErr bool
	}{
		{"success", jobs.OutcomeSuccess, false},
		{"quarantined", jobs.OutcomeSuccessWithQuarantine, true},
		{"failed", jobs.OutcomeFailed, true},
		{"interrupted", jobs.OutcomeInterrupted, true},
		{"overlap", jobs.OutcomeSkippedOverlap, true},
		{"pending", jobs.OutcomePending, true},
		{"unknown", jobs.Outcome("unknown"), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runner := &fakeRequiredRunner{outcome: tc.outcome}
			err := runRequired(context.Background(), runner, definition)
			if (err != nil) != tc.wantErr {
				t.Fatalf("runRequired() error = %v, want error %t", err, tc.wantErr)
			}
			if tc.wantErr && (!strings.Contains(err.Error(), definition.ID) || !strings.Contains(err.Error(), string(tc.outcome))) {
				t.Fatalf("error %q lacks job and outcome context", err)
			}
			if runner.calls != 1 || runner.id != definition.ID {
				t.Fatalf("runner called %d times with ID %q", runner.calls, runner.id)
			}
		})
	}
}

func TestRunRequiredWrapsRunnerError(t *testing.T) {
	cause := errors.New("storage unavailable")
	runner := &fakeRequiredRunner{outcome: jobs.OutcomeFailed, err: cause}
	err := runRequired(context.Background(), runner, jobs.Definition{ID: "union.gis.parcels"})
	if !errors.Is(err, cause) || !strings.Contains(err.Error(), "union.gis.parcels") {
		t.Fatalf("runRequired() error = %v, want wrapped cause and job ID", err)
	}
}

func TestCommands(t *testing.T) {
	for _, tc := range []struct {
		args []string
		mode string
	}{
		{nil, "all"}, {[]string{"schedule"}, "schedule"},
		{[]string{"run", parcelJobID}, parcelJobID}, {[]string{"run", soilJobID}, soilJobID},
		{[]string{"run", screeningJobID}, screeningJobID}, {[]string{"run", taxJobID}, taxJobID},
	} {
		mode, err := command(tc.args)
		if err != nil || mode != tc.mode {
			t.Fatalf("command(%v) = %q, %v", tc.args, mode, err)
		}
	}
	for _, args := range [][]string{{"run"}, {"run", "unknown"}, {"schedule", "extra"}, {"extra"}} {
		if _, err := command(args); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}

func TestIndependentSchedules(t *testing.T) {
	env := map[string]string{
		"UNION_PARCELS_CRON": "CRON_TZ=America/New_York 0 2 * * 0,3",
		"UNION_SOIL_CRON":    "CRON_TZ=UTC 0 3 1 */3 *",
	}
	getenv := func(key string) string { return env[key] }
	before, err := schedules(getenv)
	if err != nil {
		t.Fatal(err)
	}
	env["UNION_PARCELS_CRON"] = "CRON_TZ=UTC 15 1 * * *"
	after, err := schedules(getenv)
	if err != nil {
		t.Fatal(err)
	}
	if before[soilJobID] != after[soilJobID] || before[parcelJobID] == after[parcelJobID] || len(after) != 2 {
		t.Fatalf("schedules not independent: %v -> %v", before, after)
	}
	for _, invalid := range []string{"", "0 2 * * *", "CRON_TZ=UTC @every 1h", "CRON_TZ= 0 2 * * *", "CRON_TZ=Invalid/Zone 0 2 * * *", "CRON_TZ=UTC 99 2 * * *"} {
		env["UNION_SOIL_CRON"] = invalid
		if _, err := schedules(getenv); err == nil {
			t.Fatalf("accepted %q", invalid)
		}
	}
}

func TestSchedulesNormalizeWhitespace(t *testing.T) {
	for _, expression := range []string{
		"CRON_TZ=UTC\t0\t2\t*\t*\t*",
		"  CRON_TZ=UTC  0 2 * * *  ",
	} {
		calendar, err := schedules(func(string) string { return expression })
		if err != nil {
			t.Fatalf("schedules(%q): %v", expression, err)
		}
		for _, id := range []string{parcelJobID, soilJobID} {
			if calendar[id] != "CRON_TZ=UTC 0 2 * * *" {
				t.Fatalf("schedule %s = %q", id, calendar[id])
			}
		}
	}
}

type executingRunner struct {
	ids      []string
	outcomes map[string]jobs.Outcome
}

func (r *executingRunner) Run(ctx context.Context, d jobs.Definition) (jobs.Outcome, jobs.Result, error) {
	r.ids = append(r.ids, d.ID)
	if outcome := r.outcomes[d.ID]; outcome != "" && outcome != jobs.OutcomeSuccess {
		return outcome, jobs.Result{}, nil
	}
	result, err := d.Run(ctx)
	if err != nil {
		return jobs.OutcomeFailed, result, err
	}
	return jobs.OutcomeSuccess, result, nil
}

func TestManualSourcePaths(t *testing.T) {
	for _, mode := range []string{"all", parcelJobID, soilJobID, screeningJobID} {
		t.Run(mode, func(t *testing.T) {
			runner := &executingRunner{}
			parcelFetches, soilFetches, refreshes := 0, 0, 0
			definitions := []jobs.Definition{
				{ID: parcelJobID, Run: func(context.Context) (jobs.Result, error) { parcelFetches++; return jobs.Result{}, nil }},
				{ID: soilJobID, Run: func(context.Context) (jobs.Result, error) { soilFetches++; return jobs.Result{}, nil }},
			}
			refresh := func(ctx context.Context) error {
				return runRequired(ctx, runner, jobs.Definition{ID: screeningJobID, Run: func(context.Context) (jobs.Result, error) {
					refreshes++
					return jobs.Result{}, nil
				}})
			}
			executor := unionExecutor{runner: runner, refresh: refresh}
			if err := runManual(context.Background(), mode, runner, executor, definitions, refresh); err != nil {
				t.Fatal(err)
			}
			wantParcels, wantSoil := 0, 0
			if mode == "all" || mode == parcelJobID {
				wantParcels = 1
			}
			if mode == "all" || mode == soilJobID {
				wantSoil = 1
			}
			if parcelFetches != wantParcels || soilFetches != wantSoil || refreshes != 1 {
				t.Fatalf("fetch parcels=%d soil=%d refresh=%d", parcelFetches, soilFetches, refreshes)
			}
			if mode == "all" && strings.Join(runner.ids, ",") != parcelJobID+","+soilJobID+","+screeningJobID {
				t.Fatalf("manual order %v", runner.ids)
			}
		})
	}
}

func TestSourceOutcomesDoNotConcealIncompleteWork(t *testing.T) {
	for _, outcome := range []jobs.Outcome{jobs.OutcomeFailed, jobs.OutcomeInterrupted, jobs.OutcomeSkippedOverlap, jobs.OutcomeSuccessWithQuarantine} {
		runner := &executingRunner{outcomes: map[string]jobs.Outcome{parcelJobID: outcome}}
		refreshes := 0
		executor := unionExecutor{runner: runner, refresh: func(context.Context) error { refreshes++; return nil }}
		definition := jobs.Definition{ID: parcelJobID, Run: func(context.Context) (jobs.Result, error) {
			t.Fatal("non-clean source invoked")
			return jobs.Result{}, nil
		}}
		if err := runRequired(context.Background(), executor, definition); err == nil || refreshes != 0 {
			t.Fatalf("outcome %s: error=%v refreshes=%d", outcome, err, refreshes)
		}
	}
	for _, cause := range []error{errors.New("USDA failed"), jobs.ErrPending, context.Canceled} {
		runner := &executingRunner{}
		executor := unionExecutor{runner: runner, refresh: func(context.Context) error { return cause }}
		outcome, result, err := executor.Run(context.Background(), jobs.Definition{ID: soilJobID, Run: func(context.Context) (jobs.Result, error) { return jobs.Result{Processed: 3}, nil }})
		if outcome != jobs.OutcomeSuccess || result.Processed != 3 || !errors.Is(err, cause) {
			t.Fatalf("lost separate source success or derived error: %s %v %v", outcome, result, err)
		}
	}
}

func TestSourceAndRefreshShareDeadlineAndCancellation(t *testing.T) {
	const limit = 10 * time.Minute
	for _, parentLimit := range []time.Duration{time.Hour, time.Minute} {
		t.Run(parentLimit.String(), func(t *testing.T) {
			parent, cancel := context.WithTimeout(context.Background(), parentLimit)
			defer cancel()
			started := time.Now()
			var sourceDeadline time.Time
			executor := unionExecutor{runner: &executingRunner{}, refresh: func(ctx context.Context) error {
				deadline, ok := ctx.Deadline()
				if !ok || !deadline.Equal(sourceDeadline) {
					t.Fatalf("source deadline %v differs from refresh deadline %v", sourceDeadline, deadline)
				}
				return ctx.Err()
			}}
			outcome, _, err := executor.Run(parent, jobs.Definition{ID: parcelJobID, Run: func(ctx context.Context) (jobs.Result, error) {
				var ok bool
				sourceDeadline, ok = ctx.Deadline()
				if !ok {
					t.Fatal("source has no deadline")
				}
				if parentLimit > limit {
					if sourceDeadline.Before(started.Add(limit)) || sourceDeadline.After(time.Now().Add(limit)) {
						t.Fatalf("source deadline %v is not bounded by %v", sourceDeadline, limit)
					}
				} else if deadline, _ := parent.Deadline(); !sourceDeadline.Equal(deadline) {
					t.Fatalf("extended parent deadline %v to %v", deadline, sourceDeadline)
				}
				cancel()
				return jobs.Result{}, nil
			}})
			if outcome != jobs.OutcomeSuccess || !errors.Is(err, context.Canceled) {
				t.Fatalf("outcome=%s error=%v", outcome, err)
			}
		})
	}
}

func TestScheduleValidationPrecedesDatabaseAccess(t *testing.T) {
	args := os.Args
	os.Args = []string{"landman", "schedule"}
	t.Cleanup(func() { os.Args = args })
	t.Setenv("DATABASE_URL", "://invalid")
	for _, missing := range []string{"UNION_PARCELS_CRON", "UNION_SOIL_CRON"} {
		t.Run(missing, func(t *testing.T) {
			t.Setenv("UNION_PARCELS_CRON", "CRON_TZ=UTC 0 2 * * *")
			t.Setenv("UNION_SOIL_CRON", "CRON_TZ=UTC 0 3 1 */3 *")
			t.Setenv(missing, "")
			if err := run(context.Background()); err == nil || !strings.Contains(err.Error(), missing) {
				t.Fatalf("startup error=%v, want schedule validation for %s", err, missing)
			}
		})
	}
}

type overlapOnceRunner struct {
	calls  int
	cancel context.CancelFunc
}

func (r *overlapOnceRunner) Run(_ context.Context, _ jobs.Definition) (jobs.Outcome, jobs.Result, error) {
	r.calls++
	if r.calls == 1 {
		if r.cancel != nil {
			r.cancel()
		}
		return jobs.OutcomeSkippedOverlap, jobs.Result{}, nil
	}
	return jobs.OutcomeSuccess, jobs.Result{}, nil
}

func TestDerivedRefreshRetriesOverlap(t *testing.T) {
	runner := &overlapOnceRunner{}
	if err := runDerived(context.Background(), runner, jobs.Definition{ID: screeningJobID}); err != nil {
		t.Fatal(err)
	}
	if runner.calls != 2 {
		t.Fatalf("derived runner calls = %d, want 2", runner.calls)
	}
}

func TestDerivedRefreshOverlapHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	runner := &overlapOnceRunner{cancel: cancel}
	if err := runDerived(ctx, runner, jobs.Definition{ID: screeningJobID}); !errors.Is(err, context.Canceled) {
		t.Fatalf("derived refresh error = %v, want cancellation", err)
	}
	if runner.calls != 1 {
		t.Fatalf("derived runner calls = %d, want 1", runner.calls)
	}
}

func setManualTaxEnv(t *testing.T, config taxjob.Config) {
	t.Helper()
	for _, entry := range []struct{ name, value string }{
		{"UNION_TAX_ARTIFACT_DIR", config.ArtifactDir},
		{"UNION_TAX_DELINQUENT_PATH", config.DelinquentPath},
		{"UNION_TAX_BILLING_PATH", config.BillingPath},
		{"UNION_TAX_FORECLOSURE_PATH", config.ForeclosurePath},
		{"UNION_TAX_PIPELINE_PATH", config.PipelinePath},
		{"UNION_TAX_BILLING_SOURCE_URL", config.BillingSourceURL},
		{"UNION_TAX_PIPELINE_SOURCE_URL", config.PipelineSourceURL},
	} {
		t.Setenv(entry.name, entry.value)
	}
}

func TestManualTaxRequiresAllInputsBeforeDatabase(t *testing.T) {
	previousArgs := os.Args
	defer func() { os.Args = previousArgs }()
	os.Args = []string{"landman", "run", taxJobID}
	t.Setenv("DATABASE_URL", "invalid://database")
	missing := t.TempDir() + "/absent.csv"
	config := taxjob.Config{
		ArtifactDir: missing, DelinquentPath: missing, BillingPath: missing,
		ForeclosurePath: missing, PipelinePath: missing,
		BillingSourceURL: "https://example.org/billing", PipelineSourceURL: "https://example.org/pipeline",
	}
	setManualTaxEnv(t, config)
	for _, name := range []string{
		"UNION_TAX_ARTIFACT_DIR", "UNION_TAX_DELINQUENT_PATH", "UNION_TAX_BILLING_PATH",
		"UNION_TAX_FORECLOSURE_PATH", "UNION_TAX_PIPELINE_PATH",
		"UNION_TAX_BILLING_SOURCE_URL", "UNION_TAX_PIPELINE_SOURCE_URL",
	} {
		t.Setenv(name, "")
		if err := run(context.Background()); err == nil || !strings.Contains(err.Error(), name) {
			t.Fatalf("missing %s: %v", name, err)
		}
		setManualTaxEnv(t, config)
	}
	got, err := manualTaxConfig(os.Getenv)
	if err != nil || got != config {
		t.Fatalf("configured tax job: %+v %v", got, err)
	}
	if err := run(context.Background()); err == nil || !strings.Contains(err.Error(), "database") {
		t.Fatalf("configured tax job did not reach database: %v", err)
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatalf("manual tax accessed missing file: %v", err)
	}
}

func TestDefaultManualStopsBeforeDependencies(t *testing.T) {
	runner := &executingRunner{outcomes: map[string]jobs.Outcome{parcelJobID: jobs.OutcomeSkippedOverlap}}
	definitions := []jobs.Definition{{ID: parcelJobID}, {ID: soilJobID}}
	refresh := func(context.Context) error { t.Fatal("refresh after skipped source"); return nil }
	err := runManual(context.Background(), "all", runner, unionExecutor{}, definitions, refresh)
	if err == nil || len(runner.ids) != 1 {
		t.Fatalf("error=%v jobs=%v", err, runner.ids)
	}
}

func TestSchedulerDispatchesSourceAndDerivedThroughRunner(t *testing.T) {
	runner := &executingRunner{}
	refreshes := 0
	executor := unionExecutor{runner: runner, refresh: func(ctx context.Context) error {
		return runRequired(ctx, runner, jobs.Definition{ID: screeningJobID, Run: func(context.Context) (jobs.Result, error) { refreshes++; return jobs.Result{}, nil }})
	}}
	parcels, soilFetches := 0, 0
	scheduler, err := jobs.NewScheduler(executor, []jobs.Definition{
		{ID: parcelJobID, Schedule: "CRON_TZ=UTC 0 2 * * *", Run: func(context.Context) (jobs.Result, error) { parcels++; return jobs.Result{}, nil }},
		{ID: soilJobID, Schedule: "CRON_TZ=UTC 0 3 1 */3 *", Run: func(context.Context) (jobs.Result, error) { soilFetches++; return jobs.Result{}, nil }},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer scheduler.Stop()
	if parcels != 0 || soilFetches != 0 || refreshes != 0 {
		t.Fatal("scheduler ran on construction")
	}
	if outcome, _, err := scheduler.Run(context.Background(), parcelJobID); err != nil || outcome != jobs.OutcomeSuccess {
		t.Fatalf("dispatch=%s %v", outcome, err)
	}
	if parcels != 1 || soilFetches != 0 || refreshes != 1 {
		t.Fatalf("parcels=%d soil=%d refresh=%d", parcels, soilFetches, refreshes)
	}
	if _, _, err := scheduler.Run(context.Background(), screeningJobID); err == nil {
		t.Fatal("derived job was independently registered")
	}
}

func TestReprocessCommandValidationBeforeDatabase(t *testing.T) {
	for _, limit := range []string{"1", "100"} {
		mode, err := command([]string{"reprocess", parcelJobID, "--limit", limit})
		if err != nil || mode != "reprocess" {
			t.Fatal(mode, err)
		}
	}
	for _, args := range [][]string{
		{"reprocess", parcelJobID, "--limit", "0"}, {"reprocess", parcelJobID, "--limit", "101"},
		{"reprocess", parcelJobID, "--limit", "invalid"}, {"reprocess", soilJobID, "--limit", "1"},
		{"reprocess", parcelJobID}, {"reprocess", parcelJobID, "--limit", "1", "extra"},
	} {
		old := os.Args
		os.Args = append([]string{"landman"}, args...)
		err := run(context.Background())
		os.Args = old
		if err == nil || strings.Contains(err.Error(), "connect") {
			t.Fatalf("args=%v error=%v", args, err)
		}
	}
}

func TestCleanRepairUsesExistingDerivedExecutor(t *testing.T) {
	runner := &executingRunner{}
	refreshes := 0
	executor := unionExecutor{runner: runner, refresh: func(context.Context) error { refreshes++; return errors.New("derived failed") }}
	outcome, result, err := executor.Run(context.Background(), jobs.Definition{ID: parcelJobID, Kind: jobs.AttemptReprocess, Run: func(context.Context) (jobs.Result, error) { return jobs.Result{Processed: 2}, nil }})
	if outcome != jobs.OutcomeSuccess || result.Processed != 2 || refreshes != 1 || err == nil {
		t.Fatalf("outcome=%s result=%v refreshes=%d error=%v", outcome, result, refreshes, err)
	}
}
