package jobs

import (
	"context"
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
