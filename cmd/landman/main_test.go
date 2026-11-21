package main

import (
	"bytes"
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JustinMonty20/landman/internal/connector/fema"
	"github.com/JustinMonty20/landman/internal/connector/union"
	"github.com/JustinMonty20/landman/internal/connector/union/taxjob"
	"github.com/JustinMonty20/landman/internal/connector/union/vacancyjob"
	"github.com/JustinMonty20/landman/internal/jobs"
)

func repositoryPath(parts ...string) string {
	workingDirectory, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	return filepath.Join(append([]string{workingDirectory, "..", ".."}, parts...)...)
}

func TestMain(m *testing.M) {
	if err := os.Chdir(repositoryPath()); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

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

func TestStandaloneJobLogsSourceAndDerivedAttempts(t *testing.T) {
	var output bytes.Buffer
	logger := log.Default()
	previous := logger.Writer()
	logger.SetOutput(&output)
	t.Cleanup(func() { logger.SetOutput(previous) })

	runner := loggingRunner{&executingRunner{}}
	source := jobs.Definition{ID: zoningJobID, Run: func(context.Context) (jobs.Result, error) {
		if !strings.Contains(output.String(), "[union.gis.zoning] Started:") {
			t.Fatal("source started before its start log")
		}
		return jobs.Result{Processed: 7}, nil
	}}
	executor := unionExecutor{runner: runner, afterSource: func(ctx context.Context, _ string) error {
		return runDerived(ctx, runner, jobs.Definition{ID: qualificationJobID, Run: func(context.Context) (jobs.Result, error) {
			return jobs.Result{Processed: 3}, nil
		}})
	}}
	if err := runManual(context.Background(), zoningJobID, runner, executor, []jobs.Definition{source}, nil); err != nil {
		t.Fatal(err)
	}
	remaining := output.String()
	for _, line := range []string{
		"[union.gis.zoning] Started: kind=full_scan",
		"[union.gis.zoning] Finished: outcome=success processed=7",
		"[union.qualification.assess] Started: kind=full_scan",
		"[union.qualification.assess] Finished: outcome=success processed=3",
	} {
		_, after, ok := strings.Cut(remaining, line)
		if !ok {
			t.Fatalf("missing or out-of-order log %q in %s", line, output.String())
		}
		remaining = after
	}
}

func TestManualRoadAssessmentGetsIndependentHour(t *testing.T) {
	runner := &executingRunner{}
	var sourceLimit, assessmentLimit time.Duration
	executor := unionExecutor{runner: runner, roadAssessmentTimeout: standaloneQualificationTimeout, afterSource: func(ctx context.Context, id string) error {
		if id != roadsJobID {
			t.Fatalf("downstream job %s", id)
		}
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Fatal("assessment has no deadline")
		}
		assessmentLimit = time.Until(deadline)
		return nil
	}}
	road := jobs.Definition{ID: roadsJobID, Run: func(ctx context.Context) (jobs.Result, error) {
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Fatal("roads have no deadline")
		}
		sourceLimit = time.Until(deadline)
		return jobs.Result{}, nil
	}}
	if err := runRequired(context.Background(), executor, road); err != nil {
		t.Fatal(err)
	}
	if sourceLimit < jobTimeout-time.Second || sourceLimit > jobTimeout || assessmentLimit < standaloneQualificationTimeout-time.Second || assessmentLimit > standaloneQualificationTimeout {
		t.Fatalf("roads limit %s, assessment limit %s", sourceLimit, assessmentLimit)
	}
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
		{[]string{"run", vacancyjob.StructuresID}, vacancyjob.StructuresID},
		{[]string{"run", vacancyjob.AssessedValueID}, vacancyjob.AssessedValueID},
		{[]string{"replay", vacancyjob.StructuresID, "--snapshot", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"}, "vacancy-live-replay"},
		{[]string{"replay", vacancyjob.AssessedValueID, "--snapshot", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"}, "vacancy-live-replay"},
		{[]string{"replay", taxJobID, "--snapshot", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"}, "tax-replay"},
		{[]string{"replay", soilJobID, "--snapshot", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"}, "soil-replay"},
		{[]string{"replay", screeningJobID, "--snapshot", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"}, "screening-replay"},
		{[]string{"run", zoningJobID}, zoningJobID}, {[]string{"run", roadsJobID}, roadsJobID}, {[]string{"run", qualificationJobID}, qualificationJobID},
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
		"UNION_PARCELS_CRON":       "CRON_TZ=America/New_York 0 2 * * 0,3",
		"UNION_SOIL_CRON":          "CRON_TZ=UTC 0 3 1 */3 *",
		"UNION_ZONING_CRON":        "CRON_TZ=UTC 0 4 * * 1",
		"UNION_ROADS_CRON":         "CRON_TZ=UTC 0 5 * * 2",
		"UNION_QUALIFICATION_CRON": "CRON_TZ=UTC 0 6 * * *",
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
	if before[soilJobID] != after[soilJobID] || before[parcelJobID] == after[parcelJobID] || len(after) != 5 {
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
		for _, id := range []string{parcelJobID, soilJobID, zoningJobID, roadsJobID, qualificationJobID} {
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

func TestSourceAndRefreshHaveSeparateDeadlines(t *testing.T) {
	parent, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()
	var sourceDeadline time.Time
	executor := unionExecutor{runner: &executingRunner{}, refresh: func(ctx context.Context) error {
		refreshDeadline, ok := ctx.Deadline()
		if !ok {
			t.Fatal("refresh has no deadline")
		}
		if !refreshDeadline.After(sourceDeadline) {
			t.Fatalf("refresh deadline %v must follow source deadline %v", refreshDeadline, sourceDeadline)
		}
		return nil
	}}
	outcome, _, err := executor.Run(parent, jobs.Definition{ID: parcelJobID, Run: func(ctx context.Context) (jobs.Result, error) {
		var ok bool
		sourceDeadline, ok = ctx.Deadline()
		if !ok {
			t.Fatal("source has no deadline")
		}
		return jobs.Result{}, nil
	}})
	if outcome != jobs.OutcomeSuccess || err != nil {
		t.Fatalf("outcome=%s error=%v", outcome, err)
	}
}

func TestSourceAndRefreshDoNotExtendParentDeadline(t *testing.T) {
	parent, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	parentDeadline, _ := parent.Deadline()
	executor := unionExecutor{runner: &executingRunner{}, refresh: func(ctx context.Context) error {
		deadline, ok := ctx.Deadline()
		if !ok || !deadline.Equal(parentDeadline) {
			t.Fatalf("refresh deadline %v, want parent deadline %v", deadline, parentDeadline)
		}
		return nil
	}}
	outcome, _, err := executor.Run(parent, jobs.Definition{ID: parcelJobID, Run: func(ctx context.Context) (jobs.Result, error) {
		deadline, ok := ctx.Deadline()
		if !ok || !deadline.Equal(parentDeadline) {
			t.Fatalf("source deadline %v, want parent deadline %v", deadline, parentDeadline)
		}
		return jobs.Result{}, nil
	}})
	if outcome != jobs.OutcomeSuccess || err != nil {
		t.Fatalf("outcome=%s error=%v", outcome, err)
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
		BillingSourceURL: union.BillingURL, PipelineSourceURL: union.PipelineURL,
	}
	setManualTaxEnv(t, config)
	for _, name := range []string{
		"UNION_TAX_ARTIFACT_DIR", "UNION_TAX_DELINQUENT_PATH", "UNION_TAX_BILLING_PATH",
		"UNION_TAX_FORECLOSURE_PATH", "UNION_TAX_PIPELINE_PATH",
	} {
		t.Setenv(name, "")
		for _, args := range [][]string{{"landman", "run", taxJobID}, {"landman"}} {
			os.Args = args
			if err := run(context.Background()); err == nil || !strings.Contains(err.Error(), name) {
				t.Fatalf("missing %s for %v: %v", name, args, err)
			}
		}
		setManualTaxEnv(t, config)
	}
	t.Setenv("UNION_FEMA_ARTIFACT_DIR", missing)
	t.Setenv("UNION_FEMA_CATALOG_PATH", missing)
	t.Setenv("UNION_FEMA_ARCHIVE_PATH", missing)
	got, err := manualTaxConfig(os.Getenv)
	if err != nil || got != config {
		t.Fatalf("configured tax job: %+v %v", got, err)
	}
	for _, args := range [][]string{{"landman", "run", taxJobID}, {"landman"}} {
		os.Args = args
		if err := run(context.Background()); err == nil || !strings.Contains(err.Error(), "database") {
			t.Fatalf("configured job %v did not reach database: %v", args, err)
		}
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatalf("manual tax accessed missing file: %v", err)
	}
}

func TestDefaultManualRunsAllJobsOnce(t *testing.T) {
	runner := &executingRunner{}
	definitions := []jobs.Definition{
		{ID: parcelJobID, Run: func(context.Context) (jobs.Result, error) { return jobs.Result{}, nil }},
		{ID: soilJobID, Run: func(context.Context) (jobs.Result, error) { return jobs.Result{}, nil }},
		{ID: zoningJobID, Run: func(context.Context) (jobs.Result, error) { return jobs.Result{}, nil }},
		{ID: roadsJobID, Run: func(context.Context) (jobs.Result, error) { return jobs.Result{}, nil }},
		{ID: fema.JobID, Run: func(context.Context) (jobs.Result, error) { return jobs.Result{}, nil }},
		{ID: vacancyjob.StructuresID, Run: func(context.Context) (jobs.Result, error) { return jobs.Result{}, nil }},
		{ID: vacancyjob.AssessedValueID, Run: func(context.Context) (jobs.Result, error) { return jobs.Result{}, nil }},
		{ID: taxJobID, Run: func(context.Context) (jobs.Result, error) { return jobs.Result{}, nil }},
	}
	refresh := func(ctx context.Context) error {
		return runRequired(ctx, runner, jobs.Definition{ID: screeningJobID, Run: func(context.Context) (jobs.Result, error) { return jobs.Result{}, nil }})
	}
	qualify := func(ctx context.Context) error {
		return runRequired(ctx, runner, jobs.Definition{ID: qualificationJobID, Run: func(context.Context) (jobs.Result, error) { return jobs.Result{}, nil }})
	}
	if err := runManual(context.Background(), "all", runner, unionExecutor{qualify: qualify}, definitions, refresh); err != nil {
		t.Fatal(err)
	}
	want := strings.Join([]string{parcelJobID, soilJobID, zoningJobID, roadsJobID, fema.JobID, vacancyjob.StructuresID, vacancyjob.AssessedValueID, screeningJobID, qualificationJobID, taxJobID}, ",")
	if got := strings.Join(runner.ids, ","); got != want {
		t.Fatalf("default job order %q, want %q", got, want)
	}
}

func TestDefaultTaxFailureKeepsCompletedScreening(t *testing.T) {
	runner := &executingRunner{outcomes: map[string]jobs.Outcome{taxJobID: jobs.OutcomeFailed}}
	definitions := []jobs.Definition{
		{ID: parcelJobID, Run: func(context.Context) (jobs.Result, error) { return jobs.Result{}, nil }},
		{ID: soilJobID, Run: func(context.Context) (jobs.Result, error) { return jobs.Result{}, nil }},
		{ID: taxJobID},
	}
	refreshes := 0
	refresh := func(context.Context) error { refreshes++; return nil }
	if err := runManual(context.Background(), "all", runner, unionExecutor{}, definitions, refresh); err == nil || !strings.Contains(err.Error(), taxJobID) {
		t.Fatalf("tax failure: %v", err)
	}
	if got, want := strings.Join(runner.ids, ","), parcelJobID+","+soilJobID+","+taxJobID; got != want || refreshes != 1 {
		t.Fatalf("source order=%q screening runs=%d", got, refreshes)
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

func TestScheduledQualificationDoesNotRefreshSoil(t *testing.T) {
	runner := &executingRunner{}
	called := 0
	executor := unionExecutor{runner: runner, refresh: func(context.Context) error { called++; return errors.New("soil refresh should not run") }}
	scheduler, err := jobs.NewScheduler(executor, []jobs.Definition{{ID: qualificationJobID, Schedule: "CRON_TZ=UTC 0 4 * * *", Run: func(context.Context) (jobs.Result, error) { return jobs.Result{}, nil }}})
	if err != nil {
		t.Fatal(err)
	}
	defer scheduler.Stop()
	outcome, _, err := scheduler.Run(context.Background(), qualificationJobID)
	if err != nil || outcome != jobs.OutcomeSuccess || called != 0 {
		t.Fatalf("scheduled qualification: outcome %s error %v soil refreshes %d", outcome, err, called)
	}
}

func TestRepairDispatchOpensScreeningStoreOnlyWhenNeeded(t *testing.T) {
	for _, mode := range []string{"reprocess", parcelJobID, soilJobID, screeningJobID, "screening-replay", "soil-replay", "schedule", "all"} {
		if !needsSoilArtifacts(mode) {
			t.Errorf("%s needs a screening artifact store", mode)
		}
	}
	for _, mode := range []string{zoningJobID, roadsJobID, taxJobID, "tax-replay", qualificationJobID, "replay"} {
		if needsSoilArtifacts(mode) {
			t.Errorf("%s must not open a screening artifact store", mode)
		}
	}
}
