// Package jobs runs source imports with durable attempt history.
package jobs

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type Outcome string

const (
	OutcomeSuccess               Outcome = "success"
	OutcomeSuccessWithQuarantine Outcome = "success_with_quarantine"
	OutcomeFailed                Outcome = "failed"
	OutcomeInterrupted           Outcome = "interrupted"
	OutcomeSkippedOverlap        Outcome = "skipped_overlap"
)

// Result describes work completed by a source job.
type Result struct {
	Processed   int
	Quarantined int
}

// Definition is a code-defined source import.
type Definition struct {
	ID       string
	Schedule string
	Run      func(context.Context) (Result, error)
}

type attemptStore interface {
	claim(context.Context, string) (attempt, bool, error)
	recordOverlap(context.Context, string) error
	recoverInterrupted(context.Context) error
}

type attempt interface {
	finish(context.Context, Outcome, Result, error) error
}

// Runner serializes executions of a job ID across processes and persists outcomes.
type Runner struct {
	store attemptStore
}

func newRunner(store attemptStore) *Runner {
	return &Runner{store: store}
}

// RecoverInterrupted changes abandoned running attempts to interrupted.
func (r *Runner) RecoverInterrupted(ctx context.Context) error {
	if r == nil || r.store == nil {
		return errors.New("jobs: runner has no attempt store")
	}
	return r.store.recoverInterrupted(ctx)
}

// Run executes a definition once. Overlapping executions are recorded and skipped.
func (r *Runner) Run(ctx context.Context, definition Definition) (Outcome, Result, error) {
	if r == nil || r.store == nil {
		return "", Result{}, errors.New("jobs: runner has no attempt store")
	}
	if err := validateDefinition(definition); err != nil {
		return "", Result{}, err
	}

	attempt, claimed, err := r.store.claim(ctx, definition.ID)
	if err != nil {
		return "", Result{}, fmt.Errorf("jobs: claim %q: %w", definition.ID, err)
	}
	if !claimed {
		if err := r.store.recordOverlap(ctx, definition.ID); err != nil {
			return "", Result{}, fmt.Errorf("jobs: record overlap %q: %w", definition.ID, err)
		}
		return OutcomeSkippedOverlap, Result{}, nil
	}

	result, runErr := definition.Run(ctx)
	if resultErr := validResult(result); resultErr != nil && runErr == nil {
		runErr = resultErr
	}
	outcome := outcomeFor(result, runErr)
	if err := attempt.finish(context.WithoutCancel(ctx), outcome, result, runErr); err != nil {
		return "", Result{}, fmt.Errorf("jobs: finish %q: %w", definition.ID, err)
	}
	return outcome, result, runErr
}

func outcomeFor(result Result, err error) Outcome {
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return OutcomeInterrupted
		}
		return OutcomeFailed
	}
	if result.Quarantined > 0 {
		return OutcomeSuccessWithQuarantine
	}
	return OutcomeSuccess
}

func validateDefinition(definition Definition) error {
	if definition.ID == "" {
		return errors.New("jobs: definition ID is required")
	}
	if definition.Run == nil {
		return fmt.Errorf("jobs: definition %q has no run function", definition.ID)
	}
	return nil
}

func validResult(result Result) error {
	if result.Processed < 0 || result.Quarantined < 0 {
		return errors.New("jobs: result counts cannot be negative")
	}
	return nil
}

// LastCleanSuccess is intentionally represented by the store query rather than
// inferring freshness from any successful-with-quarantine attempt.
type LastCleanSuccess struct {
	JobID      string
	FinishedAt time.Time
}
