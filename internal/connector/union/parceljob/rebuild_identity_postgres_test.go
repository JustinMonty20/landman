//go:build postgres

package parceljob

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JustinMonty20/landman/internal/jobs"
	"github.com/JustinMonty20/landman/internal/taxartifact"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestEmptyRebuildReplayRetainsOBJECTIDTransition(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(cfg.ConnConfig.Database, "landman_parceljob_test_") {
		t.Fatal("refusing non-disposable database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var name string
	if err = pool.QueryRow(ctx, `SELECT current_database()`).Scan(&name); err != nil || name != cfg.ConnConfig.Database {
		t.Fatal("database mismatch", err)
	}
	root := t.TempDir()
	if err = os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	job := Job{Root: root, DSN: dsn, Quarantine: jobs.NewQuarantineStore(pool)}
	runner, err := jobs.NewRunner(pool)
	if err != nil {
		t.Fatal(err)
	}
	feature, err := os.ReadFile(filepath.Join("..", "testdata", "parcel-multiple-shells.json"))
	if err != nil {
		t.Fatal(err)
	}
	var source map[string]any
	if err = json.Unmarshal(feature, &source); err != nil {
		t.Fatal(err)
	}
	attrs := source["attributes"].(map[string]any)
	const oid = 7070707
	attrs["OBJECTID"] = oid
	attrs["PID"] = "rebuild-live-a"
	publish := func(expectFailure bool) string {
		t.Helper()
		body, e := json.Marshal(map[string]any{"spatialReference": map[string]any{"wkid": 3857}, "features": []any{source}})
		if e != nil {
			t.Fatal(e)
		}
		var capture string
		outcome, _, e := runner.Run(ctx, jobs.Definition{ID: JobID, Kind: jobs.AttemptFullScan, Run: func(runCtx context.Context) (jobs.Result, error) {
			store, e := taxartifact.NewLocal(root)
			if e != nil {
				return jobs.Result{}, e
			}
			recorder, e := job.newRecorder(store)
			if e != nil {
				return jobs.Result{}, e
			}
			for _, page := range []struct {
				request string
				crs     int
				body    []byte
			}{
				{"https://example.invalid/query?returnCountOnly=true", 0, []byte(`{"count":1}`)},
				{"https://example.invalid/query?resultOffset=0&resultRecordCount=1", 3857, body},
			} {
				if e = recorder.Record(runCtx, page.request, "", page.crs, bytes.NewReader(page.body)); e != nil {
					return jobs.Result{}, e
				}
			}
			result, e := job.publishShared(runCtx, store, recorder, nil, 1)
			if e == nil {
				attempt, _ := jobs.AttemptID(runCtx)
				e = pool.QueryRow(runCtx, `SELECT id::text FROM ingest_captures WHERE attempt_id=$1`, attempt).Scan(&capture)
			}
			return result, e
		}})
		if expectFailure {
			if e == nil || outcome != jobs.OutcomeFailed {
				t.Fatalf("expected failed retained capture: %s %v", outcome, e)
			}
			if e = pool.QueryRow(ctx, `SELECT c.id::text FROM ingest_captures c JOIN job_attempts a ON a.id=c.attempt_id WHERE a.job_id=$1 AND a.outcome='failed' ORDER BY a.started_at DESC LIMIT 1`, JobID).Scan(&capture); e != nil {
				t.Fatal(e)
			}
			return capture
		}
		if e != nil || (outcome != jobs.OutcomeSuccess && outcome != jobs.OutcomeSuccessWithQuarantine) {
			t.Fatalf("publication %s: %v", outcome, e)
		}
		return capture
	}
	capture := publish(false)
	// Only the source serving snapshot is wiped; retained capture and receipt bytes remain.
	for _, sql := range []string{
		`DELETE FROM parcel_evidence_current WHERE source_parcel_id='rebuild-live-a'`,
		`DELETE FROM parcel_evidence_history WHERE source_parcel_id='rebuild-live-a'`,
		`DELETE FROM parcels WHERE source_parcel_id='rebuild-live-a'`,
		`DELETE FROM parcel_source_object_identity WHERE raw_key='7070707'`,
		`DELETE FROM parcel_derived_handoff WHERE artifact_id=$1`,
		`DELETE FROM parcel_evidence_observations WHERE artifact_id=$1`,
	} {
		var args []any
		if strings.Contains(sql, "$1") {
			args = append(args, capture)
		}
		if _, err = pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal("prepare empty-source rebuild: ", err)
		}
	}
	outcome, _, err := runner.Run(ctx, jobs.Definition{ID: JobID, Kind: jobs.AttemptReplay, Run: func(runCtx context.Context) (jobs.Result, error) { return job.Replay(runCtx, capture) }})
	if err != nil || outcome != jobs.OutcomeSuccess {
		t.Fatalf("rebuild replay %s: %v", outcome, err)
	}
	var pid string
	if err = pool.QueryRow(ctx, `SELECT source_parcel_id FROM parcel_source_object_identity WHERE raw_key='7070707'`).Scan(&pid); err != nil || pid != "rebuild-live-a" {
		t.Fatalf("rebuild lost OBJECTID identity: %q %v", pid, err)
	}
	attrs["PID"] = nil
	_ = publish(false)
	var marker *string
	if err = pool.QueryRow(ctx, `SELECT parcel_row_version FROM parcel_evidence_current WHERE source_parcel_id='rebuild-live-a'`).Scan(&marker); err != nil || marker != nil {
		t.Fatalf("former PID still certified: %v %v", marker, err)
	}
	if err = pool.QueryRow(ctx, `SELECT source_parcel_id FROM parcel_evidence_rejection_cutoff WHERE source_parcel_id='rebuild-live-a'`).Scan(&pid); err != nil || pid != "rebuild-live-a" {
		t.Fatalf("missing transition fence: %q %v", pid, err)
	}
	var oversized strings.Builder
	for i := 0; i < 80; i++ {
		digest := sha256.Sum256([]byte(fmt.Sprint(i)))
		oversized.WriteString(hex.EncodeToString(digest[:]))
	}
	attrs["PID"] = oversized.String()
	_ = publish(false)
	var retained bool
	if err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM parcel_source_object_identity WHERE raw_key='7070707' AND source_parcel_id=$1)`, oversized.String()).Scan(&retained); err != nil || retained {
		t.Fatalf("quarantined oversized PID entered identity map: %v %v", retained, err)
	}
	attrs["PID"] = "rebuild-live-new"
	_ = publish(false)
	if err = pool.QueryRow(ctx, `SELECT source_parcel_id FROM parcel_source_object_identity WHERE raw_key='7070707'`).Scan(&pid); err != nil || pid != "rebuild-live-new" {
		t.Fatalf("post-quarantine transition failed: %q %v", pid, err)
	}
	// Retain failed B, then publish newer C rejecting the same PID. The older
	// valid receipt remains replayable, but must not re-certify C's withdrawal.
	if _, err = pool.Exec(ctx, `ALTER TABLE parcels ADD CONSTRAINT revision_reject_b CHECK (current_name_1 IS DISTINCT FROM 'FAILED-B')`); err != nil {
		t.Fatal(err)
	}
	attrs["CURR_NAME1"] = "FAILED-B"
	failedB := publish(true)
	if _, err = pool.Exec(ctx, `ALTER TABLE parcels DROP CONSTRAINT revision_reject_b`); err != nil {
		t.Fatal(err)
	}
	attrs["CURR_NAME1"] = "ACME LLC"
	geometry := source["geometry"]
	source["geometry"] = nil
	_ = publish(false)
	source["geometry"] = geometry
	var cutoff time.Time
	if err = pool.QueryRow(ctx, `SELECT observed_at FROM parcel_evidence_rejection_cutoff WHERE source_parcel_id='rebuild-live-new'`).Scan(&cutoff); err != nil {
		t.Fatal("newer rejection not retained: ", err)
	}
	outcome, _, err = runner.Run(ctx, jobs.Definition{ID: JobID, Kind: jobs.AttemptReplay, Run: func(runCtx context.Context) (jobs.Result, error) { return job.Replay(runCtx, failedB) }})
	if err != nil || outcome != jobs.OutcomeSuccess {
		t.Fatalf("older failed replay %s: %v", outcome, err)
	}
	if err = pool.QueryRow(ctx, `SELECT parcel_row_version FROM parcel_evidence_current WHERE source_parcel_id='rebuild-live-new'`).Scan(&marker); err != nil || marker != nil {
		t.Fatalf("older B recertified after newer refusal at %s: %v %v", cutoff, marker, err)
	}
}
