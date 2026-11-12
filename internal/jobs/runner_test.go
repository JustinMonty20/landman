package jobs

import (
	"context"
	"errors"
	"testing"
)

type fakeStore struct {
	claimed         bool
	recovered       bool
	overlapRecorded bool
	finished        Outcome
	result          Result
	runErr          error
}

func (s *fakeStore) claim(context.Context, string) (attempt, bool, error) {
	return fakeAttempt{s: s}, s.claimed, nil
}

func (s *fakeStore) recordOverlap(context.Context, string) error {
	s.overlapRecorded = true
	return nil
}

func (s *fakeStore) recoverInterrupted(context.Context) error {
	s.recovered = true
	return nil
}

type fakeAttempt struct{ s *fakeStore }

func (a fakeAttempt) id() string { return "test-attempt" }

func (a fakeAttempt) finish(_ context.Context, outcome Outcome, result Result, err error) error {
	a.s.finished = outcome
	a.s.result = result
	a.s.runErr = err
	return nil
}

func TestRunnerRun(t *testing.T) {
	tests := []struct {
		name    string
		claimed bool
		result  Result
		runErr  error
		outcome Outcome
	}{
		{name: "success", claimed: true, result: Result{Processed: 2}, outcome: OutcomeSuccess},
		{name: "quarantine", claimed: true, result: Result{Processed: 2, Quarantined: 1}, outcome: OutcomeSuccessWithQuarantine},
		{name: "failure", claimed: true, runErr: errors.New("source failed"), outcome: OutcomeFailed},
		{name: "cancellation", claimed: true, runErr: context.Canceled, outcome: OutcomeInterrupted},
		{name: "invalid counts", claimed: true, result: Result{Processed: -1}, outcome: OutcomeFailed},
		{name: "overlap", claimed: false, outcome: OutcomeSkippedOverlap},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeStore{claimed: tt.claimed}
			runner := newRunner(store)
			outcome, result, err := runner.Run(context.Background(), Definition{
				ID:  "union.gis.parcels",
				Run: func(context.Context) (Result, error) { return tt.result, tt.runErr },
			})
			if tt.name == "invalid counts" {
				if err == nil {
					t.Fatal("error = nil, want invalid count error")
				}
			} else if !errors.Is(err, tt.runErr) {
				t.Fatalf("error = %v, want %v", err, tt.runErr)
			}
			if outcome != tt.outcome {
				t.Errorf("outcome = %q, want %q", outcome, tt.outcome)
			}
			if result != tt.result && tt.claimed {
				t.Errorf("result = %#v, want %#v", result, tt.result)
			}
			if tt.claimed && store.finished != tt.outcome {
				t.Errorf("persisted outcome = %q, want %q", store.finished, tt.outcome)
			}
			if !tt.claimed && !store.overlapRecorded {
				t.Error("overlap was not recorded")
			}
		})
	}
}

func TestRunnerRejectsInvalidDefinition(t *testing.T) {
	runner := newRunner(&fakeStore{claimed: true})
	_, _, err := runner.Run(context.Background(), Definition{})
	if err == nil {
		t.Fatal("Run() error = nil, want error")
	}
}

func TestRunnerRecoverInterrupted(t *testing.T) {
	store := &fakeStore{}
	if err := newRunner(store).RecoverInterrupted(context.Background()); err != nil {
		t.Fatalf("RecoverInterrupted() error = %v", err)
	}
	if !store.recovered {
		t.Error("recovery was not requested")
	}
}
