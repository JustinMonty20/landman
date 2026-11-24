//go:build postgres

package parceljob

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JustinMonty20/landman/internal/jobs"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestHistoricalSchema56CaptureAfterUpgrade(t *testing.T) {
	root := os.Getenv("HISTORICAL_ROOT")
	if root == "" {
		t.Skip("historical schema56 capture root required")
	}
	cfg, err := pgxpool.ParseConfig(os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(cfg.ConnConfig.Database, "landman_parceljob_test_historical_") {
		t.Fatal("refusing non-disposable database")
	}
	ctx, done := context.WithTimeout(context.Background(), 25*time.Second)
	defer done()
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var db string
	if err = pool.QueryRow(ctx, `SELECT current_database()`).Scan(&db); err != nil || db != cfg.ConnConfig.Database {
		t.Fatal("database mismatch", err)
	}
	b, err := os.ReadFile(filepath.Join(root, "historical-b-id"))
	if err != nil {
		t.Fatal(err)
	}
	var before time.Time
	if err = pool.QueryRow(ctx, `SELECT observed_at FROM parcel_evidence_rejection_cutoff WHERE source_parcel_id='historical-live-a'`).Scan(&before); err != nil {
		t.Fatal("migration did not seed C refusal cutoff: ", err)
	}
	runner, err := jobs.NewRunner(pool)
	if err != nil {
		t.Fatal(err)
	}
	job := Job{Root: root, DSN: os.Getenv("DATABASE_URL"), Quarantine: jobs.NewQuarantineStore(pool)}
	outcome, _, err := runner.Run(ctx, jobs.Definition{ID: JobID, Kind: jobs.AttemptReplay, Run: func(runCtx context.Context) (jobs.Result, error) { return job.Replay(runCtx, string(b)) }})
	if err != nil || outcome != jobs.OutcomeSuccess {
		t.Fatalf("historical B replay %s %v", outcome, err)
	}
	var after time.Time
	if err = pool.QueryRow(ctx, `SELECT observed_at FROM parcel_evidence_rejection_cutoff WHERE source_parcel_id='historical-live-a'`).Scan(&after); err != nil || !after.Equal(before) {
		t.Fatalf("B rewrote newer C cutoff: %v -> %v %v", before, after, err)
	}
	var current bool
	if err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM parcel_evidence_current WHERE source_parcel_id='historical-live-a' AND parcel_row_version IS NOT NULL)`).Scan(&current); err != nil || current {
		t.Fatalf("historical B recertified C rejection: %v %v", current, err)
	}
	t.Logf("schema56 retained B replayed after upgrade; newer C cutoff=%s unchanged", after)
}
