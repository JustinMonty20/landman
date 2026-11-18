//go:build postgres

package main

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/JustinMonty20/landman/internal/soil"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestReprocessCommandDispatchesDueScreeningWithStore(t *testing.T) {
	dsn := os.Getenv("REPAIR_DISPATCH_TEST_DSN")
	if os.Getenv("REPAIR_DISPATCH_TEST_DISPOSABLE") != "1" || !strings.Contains(dsn, "/taxworker_upgrade_") {
		t.Skip("requires a fresh migrated disposable database with no source records")
	}
	t.Setenv("DATABASE_URL", dsn)
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PARCEL_EVIDENCE_DIR", root)
	previous := os.Args
	os.Args = []string{"landman", "reprocess", parcelJobID, "--limit", "1"}
	defer func() { os.Args = previous }()
	// No stored soil set is due; the screening attempt must reach its prerequisite
	// rather than fail the artifact-store precondition after repair has committed.
	err := run(context.Background())
	if !errors.Is(err, soil.ErrNoStoredSet) {
		t.Fatalf("repair did not dispatch screening with artifact store: %v", err)
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var repairs, pendingScreens int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FILTER (WHERE job_id=$1 AND outcome='success'), count(*) FILTER (WHERE job_id=$2 AND outcome='pending') FROM job_attempts WHERE job_id IN ($1,$2)`, parcelJobID, screeningJobID).Scan(&repairs, &pendingScreens); err != nil || repairs != 1 || pendingScreens != 1 {
		t.Fatalf("repair committed/screening pending: %d/%d %v", repairs, pendingScreens, err)
	}
}
