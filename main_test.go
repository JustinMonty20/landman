package main

import (
	"context"
	"errors"
	"strings"
	"testing"

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
