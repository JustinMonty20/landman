package jobs

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/robfig/cron/v3"
)

// Executor runs a definition, optionally including explicitly ordered downstream work.
type Executor interface {
	Run(context.Context, Definition) (Outcome, Result, error)
}

// Scheduler dispatches code-defined jobs at their explicit calendar schedules.
type Scheduler struct {
	runner      Executor
	definitions map[string]Definition
	cron        *cron.Cron
	ctx         context.Context
	cancel      context.CancelFunc
}

// NewScheduler validates all definitions before registering any schedule. Schedules
// must include CRON_TZ so a job's calendar is unambiguous across DST changes.
func NewScheduler(runner Executor, definitions []Definition) (*Scheduler, error) {
	if runner == nil {
		return nil, fmt.Errorf("jobs: runner is required")
	}
	byID := make(map[string]Definition, len(definitions))
	for _, definition := range definitions {
		if err := validateDefinition(definition); err != nil {
			return nil, err
		}
		if _, exists := byID[definition.ID]; exists {
			return nil, fmt.Errorf("jobs: duplicate definition ID %q", definition.ID)
		}
		if !strings.HasPrefix(definition.Schedule, "CRON_TZ=") {
			return nil, fmt.Errorf("jobs: definition %q schedule must declare CRON_TZ", definition.ID)
		}
		byID[definition.ID] = definition
	}

	ctx, cancel := context.WithCancel(context.Background())
	scheduler := &Scheduler{
		runner:      runner,
		definitions: byID,
		cron:        cron.New(),
		ctx:         ctx,
		cancel:      cancel,
	}
	for _, definition := range definitions {
		definition := definition
		if _, err := scheduler.cron.AddFunc(definition.Schedule, func() {
			outcome, _, err := scheduler.Run(scheduler.ctx, definition.ID)
			if err != nil || outcome != OutcomeSuccess {
				log.Printf("scheduled %s outcome=%s error=%v", definition.ID, outcome, err)
			}
		}); err != nil {
			cancel()
			return nil, fmt.Errorf("jobs: definition %q schedule: %w", definition.ID, err)
		}
	}
	return scheduler, nil
}

// Start begins dispatching at each schedule's next due time. It never backfills.
func (s *Scheduler) Start() {
	s.cron.Start()
}

// Stop prevents new dispatches. The returned context completes after cron workers stop.
func (s *Scheduler) Stop() context.Context {
	s.cancel()
	return s.cron.Stop()
}

// Run manually invokes the same runner path used by scheduled work.
func (s *Scheduler) Run(ctx context.Context, jobID string) (Outcome, Result, error) {
	definition, exists := s.definitions[jobID]
	if !exists {
		return "", Result{}, fmt.Errorf("jobs: unknown job %q", jobID)
	}
	return s.runner.Run(ctx, definition)
}
