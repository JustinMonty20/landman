//go:build postgres

package parceljob

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JustinMonty20/landman/internal/jobs"
	"github.com/JustinMonty20/landman/internal/parcelstore"
	"github.com/JustinMonty20/landman/internal/taxartifact"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestOwnerLabelsCanonicalPublicationReplayAndConcurrentBackfill(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	dsn := os.Getenv("DATABASE_URL")
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var db string
	if err := pool.QueryRow(ctx, `SELECT current_database()`).Scan(&db); err != nil || !(strings.HasPrefix(db, "landman_parceljob_test_") || strings.HasPrefix(db, "landman_owner_test_")) {
		t.Fatal("refusing non-disposable parcel job database")
	}
	runner, err := jobs.NewRunner(pool)
	if err != nil {
		t.Fatal(err)
	}
	store := t.TempDir()
	if err := os.Chmod(store, 0700); err != nil {
		t.Fatal(err)
	}
	job := Job{Root: store, DSN: dsn, Quarantine: jobs.NewQuarantineStore(pool)}
	fixture, err := os.ReadFile(filepath.Join("..", "testdata", "parcel-multiple-shells.json"))
	if err != nil {
		t.Fatal(err)
	}
	var feature map[string]any
	if err = json.Unmarshal(fixture, &feature); err != nil {
		t.Fatal(err)
	}
	pid := fmt.Sprintf("owner-canonical-%d", time.Now().UnixNano())
	attrs := feature["attributes"].(map[string]any)
	attrs["PID"] = pid
	attrs["OBJECTID"] = float64(time.Now().UnixNano()%1000000000 + 1)
	rawKey := fmt.Sprint(int64(attrs["OBJECTID"].(float64)))
	var captures []string
	// Restore the disposable source serving state for subsequent package tests.
	defer func() {
		cleanupCtx, done := context.WithTimeout(context.Background(), 20*time.Second)
		defer done()
		tx, e := pool.Begin(cleanupCtx)
		if e != nil {
			t.Error(e)
			return
		}
		defer tx.Rollback(context.Background())
		for _, statement := range []string{
			`DELETE FROM parcel_evidence_current WHERE source_parcel_id=$1`,
			`DELETE FROM parcel_evidence_history WHERE source_parcel_id=$1`,
			`DELETE FROM parcels WHERE source_parcel_id=$1`,
		} {
			if _, e = tx.Exec(cleanupCtx, statement, pid); e != nil {
				t.Error(e)
				return
			}
		}
		if _, e = tx.Exec(cleanupCtx, `DELETE FROM parcel_source_object_identity WHERE raw_key=$1 AND source_parcel_id=$2`, rawKey, pid); e != nil {
			t.Error(e)
			return
		}
		for _, statement := range []string{
			`DELETE FROM parcel_derived_handoff WHERE artifact_id=ANY($1)`,
			`DELETE FROM parcel_geometry_decisions WHERE artifact_id=ANY($1)`,
			`DELETE FROM parcel_evidence_observations WHERE artifact_id=ANY($1)`,
		} {
			if _, e = tx.Exec(cleanupCtx, statement, captures); e != nil {
				t.Error(e)
				return
			}
		}
		if e = tx.Commit(cleanupCtx); e != nil {
			t.Error(e)
		}
	}()
	attrs["CURR_NAME1"] = "ACME LLC"
	attrs["CURR_STATE"] = "SC"
	publish := func(name, state string) string {
		t.Helper()
		attrs["CURR_NAME1"] = name
		attrs["CURR_STATE"] = state
		body, err := json.Marshal(map[string]any{"spatialReference": map[string]any{"wkid": 3857}, "features": []any{feature}})
		if err != nil {
			t.Fatal(err)
		}
		var captureID string
		outcome, _, err := runner.Run(ctx, jobs.Definition{ID: JobID, Kind: jobs.AttemptFullScan, Run: func(runCtx context.Context) (jobs.Result, error) {
			artifactStore, err := taxartifact.NewLocal(store)
			if err != nil {
				return jobs.Result{}, err
			}
			recorder, err := job.newRecorder(artifactStore)
			if err != nil {
				return jobs.Result{}, err
			}
			for _, page := range []struct {
				request string
				crs     int
				body    []byte
			}{
				{"https://example.invalid/query?returnCountOnly=true", 0, []byte(`{"count":1}`)},
				{"https://example.invalid/query?resultOffset=0&resultRecordCount=1", 3857, body},
			} {
				if err := recorder.Record(runCtx, page.request, "", page.crs, bytes.NewReader(page.body)); err != nil {
					return jobs.Result{}, err
				}
			}
			result, err := job.publishShared(runCtx, artifactStore, recorder, nil, 1)
			if err == nil {
				attemptID, _ := jobs.AttemptID(runCtx)
				err = pool.QueryRow(runCtx, `SELECT id::text FROM ingest_captures WHERE attempt_id=$1`, attemptID).Scan(&captureID)
			}
			return result, err
		}})
		if err != nil || outcome != jobs.OutcomeSuccess {
			t.Fatalf("canonical publication %q %q: %v", name, outcome, err)
		}
		captures = append(captures, captureID)
		return captureID
	}
	captureID := publish("ACME LLC", "SC")
	var parcelID, name, state, mailing, entity string
	read := func() {
		t.Helper()
		if err := pool.QueryRow(ctx, `SELECT p.id::text,p.current_name_1,p.mailing_state,l.out_of_state_mailing,l.entity_name_indicator FROM parcels p JOIN parcel_owner_labels l ON l.parcel_id=p.id WHERE p.source_parcel_id=$1`, pid).Scan(&parcelID, &name, &state, &mailing, &entity); err != nil {
			t.Fatal(err)
		}
	}
	read()
	if name != "ACME LLC" || state != "SC" || mailing != "yes" || entity != "matched" {
		t.Fatalf("canonical labels: %s %s %s %s", name, state, mailing, entity)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM parcel_owner_labels WHERE parcel_id=$1`, parcelID); err != nil {
		t.Fatal(err)
	}
	if outcome, _, err := runner.Run(ctx, jobs.Definition{ID: JobID, Kind: jobs.AttemptReplay, Run: func(runCtx context.Context) (jobs.Result, error) {
		return job.Replay(runCtx, captureID)
	}}); err != nil || outcome != jobs.OutcomeSuccess {
		t.Fatalf("replay: %q %v", outcome, err)
	}
	read()
	if mailing != "yes" || entity != "matched" {
		t.Fatalf("replay did not repair missing labels: %s %s", mailing, entity)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM parcel_owner_labels WHERE parcel_id=$1`, parcelID); err != nil {
		t.Fatal(err)
	}
	anchor, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer anchor.Rollback(context.Background())
	if _, err := anchor.Exec(ctx, `SELECT id FROM parcels WHERE id=$1 FOR UPDATE`, parcelID); err != nil {
		t.Fatal(err)
	}
	waitForLock := func(fragment string) {
		t.Helper()
		deadline := time.Now().Add(12 * time.Second)
		for time.Now().Before(deadline) {
			var waiting bool
			err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE '%'||$1||'%' AND ($1<>'source_parcel_id' OR query NOT LIKE '%FOR UPDATE OF p%') AND pid<>pg_backend_pid())`, fragment).Scan(&waiting)
			if err != nil {
				t.Fatal(err)
			}
			if waiting {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("did not observe lock wait for %q", fragment)
	}
	backfillErr := make(chan error, 1)
	go func() {
		_, _, err := parcelstore.BackfillOwnerLabels(ctx, pool)
		backfillErr <- err
	}()
	waitForLock("FOR UPDATE OF p")
	publicationDone := make(chan struct{})
	go func() {
		defer close(publicationDone)
		_ = publish("PERSON", "NC")
	}()
	waitForLock("source_parcel_id")
	if err := anchor.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-publicationDone:
	case <-ctx.Done():
		t.Fatal("publication timed out waiting for parcel lock", ctx.Err())
	}
	select {
	case err := <-backfillErr:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("backfill timed out waiting for parcel lock", ctx.Err())
	}
	read()
	if name != "PERSON" || state != "NC" || mailing != "no" || entity != "not_detected" {
		t.Fatalf("backfill and refresh diverged: %s %s %s %s", name, state, mailing, entity)
	}
}
