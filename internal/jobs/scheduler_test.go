package jobs

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewSchedulerValidatesDefinitions(t *testing.T) {
	runner := newRunner(&fakeStore{claimed: true})
	run := func(context.Context) (Result, error) { return Result{}, nil }
	tests := []struct {
		name        string
		definitions []Definition
		want        string
	}{
		{name: "duplicate ID", definitions: []Definition{{ID: "a", Schedule: "CRON_TZ=UTC 0 0 * * *", Run: run}, {ID: "a", Schedule: "CRON_TZ=UTC 1 0 * * *", Run: run}}, want: "duplicate"},
		{name: "timezone omitted", definitions: []Definition{{ID: "a", Schedule: "0 0 * * *", Run: run}}, want: "CRON_TZ"},
		{name: "malformed", definitions: []Definition{{ID: "a", Schedule: "CRON_TZ=UTC nonsense", Run: run}}, want: "schedule"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewScheduler(runner, tt.definitions)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("NewScheduler() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestSchedulerStopCancelsScheduledJob(t *testing.T) {
	started := make(chan struct{})
	cancelled := make(chan struct{})
	release := make(chan struct{})
	var startedOnce, cancelledOnce sync.Once
	store := &fakeStore{claimed: true}
	scheduler, err := NewScheduler(newRunner(store), []Definition{{
		ID:       "scheduled.test",
		Schedule: "CRON_TZ=UTC @every 10ms",
		Run: func(ctx context.Context) (Result, error) {
			startedOnce.Do(func() { close(started) })
			select {
			case <-ctx.Done():
				cancelledOnce.Do(func() { close(cancelled) })
				return Result{}, ctx.Err()
			case <-release:
				return Result{}, nil
			}
		},
	}})
	if err != nil {
		t.Fatalf("NewScheduler() error = %v", err)
	}
	var stopped context.Context
	defer func() {
		close(release)
		if stopped != nil {
			select {
			case <-stopped.Done():
			case <-time.After(2 * time.Second):
				t.Error("scheduled job did not exit after release")
			}
		}
	}()

	scheduler.Start()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("scheduled job did not start")
	}

	stopped = scheduler.Stop()
	select {
	case <-stopped.Done():
	case <-time.After(250 * time.Millisecond):
		t.Fatal("Stop() did not finish after cancelling the scheduled job")
	}
	select {
	case <-cancelled:
	default:
		t.Fatal("scheduled job did not receive cancellation")
	}
}

func TestSchedulerRunUsesRunner(t *testing.T) {
	store := &fakeStore{claimed: true}
	scheduler, err := NewScheduler(newRunner(store), []Definition{{
		ID:       "union.gis.parcels",
		Schedule: "CRON_TZ=America/New_York 0 2 * * *",
		Run: func(context.Context) (Result, error) {
			return Result{Processed: 3}, nil
		},
	}})
	if err != nil {
		t.Fatalf("NewScheduler() error = %v", err)
	}
	outcome, result, err := scheduler.Run(context.Background(), "union.gis.parcels")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if outcome != OutcomeSuccess || result.Processed != 3 || store.finished != OutcomeSuccess {
		t.Fatalf("Run() = (%q, %#v), persisted %q", outcome, result, store.finished)
	}
}

func TestScheduledDispatchReportsIncompleteWork(t *testing.T) {
	for _, tc := range []struct {
		name    string
		claimed bool
		result  Result
		err     error
		outcome Outcome
	}{
		{name: "success", claimed: true, outcome: OutcomeSuccess},
		{name: "failed", claimed: true, err: errors.New("fixture failure"), outcome: OutcomeFailed},
		{name: "pending", claimed: true, err: ErrPending, outcome: OutcomePending},
		{name: "quarantine", claimed: true, result: Result{Quarantined: 1}, outcome: OutcomeSuccessWithQuarantine},
		{name: "overlap", outcome: OutcomeSkippedOverlap},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var output bytes.Buffer
			previous := log.Writer()
			log.SetOutput(&output)
			defer log.SetOutput(previous)
			scheduler, err := NewScheduler(newRunner(&fakeStore{claimed: tc.claimed}), []Definition{{
				ID: "scheduled.fixture", Schedule: "CRON_TZ=UTC 0 2 * * *",
				Run: func(context.Context) (Result, error) { return tc.result, tc.err },
			}})
			if err != nil {
				t.Fatal(err)
			}
			defer scheduler.Stop()
			scheduler.cron.Entries()[0].Job.Run()
			if tc.outcome == OutcomeSuccess {
				if output.Len() != 0 {
					t.Fatalf("clean success logged as incomplete: %s", output.String())
				}
				return
			}
			if !strings.Contains(output.String(), "scheduled.fixture") || !strings.Contains(output.String(), string(tc.outcome)) {
				t.Fatalf("incomplete work missing from logs: %s", output.String())
			}
			if tc.err != nil && !strings.Contains(output.String(), tc.err.Error()) {
				t.Fatalf("failure cause missing from logs: %s", output.String())
			}
		})
	}
}

func TestSchedulerIndependentCalendars(t *testing.T) {
	runner := newRunner(&fakeStore{claimed: true})
	run := func(context.Context) (Result, error) { return Result{}, nil }
	scheduler, err := NewScheduler(runner, []Definition{
		{ID: "daily", Schedule: "CRON_TZ=America/New_York 0 2 * * *", Run: run},
		{ID: "quarterly", Schedule: "CRON_TZ=UTC 0 3 1 */3 *", Run: run},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer scheduler.Stop()
	entries := scheduler.cron.Entries()
	// New York skips 02:00 on the spring DST transition day.
	start := time.Date(2026, time.March, 8, 5, 0, 0, 0, time.UTC)
	want := time.Date(2026, time.March, 9, 6, 0, 0, 0, time.UTC)
	if got := entries[0].Schedule.Next(start); !got.Equal(want) {
		t.Fatalf("daily next=%v want=%v", got, want)
	}
	boundary := time.Date(2026, time.December, 31, 23, 59, 0, 0, time.UTC)
	wantQuarter := time.Date(2027, time.January, 1, 3, 0, 0, 0, time.UTC)
	if got := entries[1].Schedule.Next(boundary); !got.Equal(wantQuarter) {
		t.Fatalf("quarter next=%v want=%v", got, wantQuarter)
	}
	if got := entries[1].Schedule.Next(wantQuarter); got.Month() != time.April || got.Day() != 1 {
		t.Fatalf("next quarter=%v", got)
	}
}
