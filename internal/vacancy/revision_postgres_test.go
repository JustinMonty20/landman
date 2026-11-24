//go:build postgres

package vacancy

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Evidence publication locks source revision first. A concurrent shape edit
// must not hold topology while waiting for that same source revision lock.
func TestEvidenceAndShapeRevisionLockOrder(t *testing.T) {
	dsn := os.Getenv("API_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("disposable API_TEST_DATABASE_URL required")
	}
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(config.ConnConfig.Database, "landman_api_test_") {
		t.Fatal("refusing non-disposable database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var name string
	if err := pool.QueryRow(ctx, `SELECT current_database()`).Scan(&name); err != nil || name != config.ConnConfig.Database {
		t.Fatal("database target mismatch", err)
	}
	defer func() {
		cleanup, done := context.WithTimeout(context.Background(), 2*time.Second)
		defer done()
		for _, sql := range []string{`DELETE FROM parcel_evidence_current WHERE source_parcel_id='revision-lock-a'`, `DELETE FROM parcels WHERE source_parcel_id IN ('revision-lock-a','revision-lock-b')`, `DELETE FROM parcel_evidence_observations WHERE artifact_id='revision-lock'`} {
			if _, e := pool.Exec(cleanup, sql); e != nil {
				t.Errorf("cleanup: %v", e)
			}
		}
	}()
	if _, err = pool.Exec(ctx, `INSERT INTO parcel_evidence_observations(artifact_id,observed_at,attempt_kind,manifest) VALUES('revision-lock',now(),'full_scan','{}')`); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO parcels(county_id,county_code,source_name,source_parcel_id,parcel_number,shape) VALUES
 ('union','179','union_county_gis','revision-lock-a','revision-lock-a',ST_MakeEnvelope(0,0,1,1,3857)),
 ('union','179','union_county_gis','revision-lock-b','revision-lock-b',ST_MakeEnvelope(2,0,3,1,3857))`); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO parcel_evidence_current(county_id,source_name,source_parcel_id,observation_id,source_hash,source_hash_version,normalized_hash,normalizer_version,geometry_hash,source_features,normalized_hash_version,geometry_hash_version)
 SELECT 'union','union_county_gis','revision-lock-a',id,repeat('a',64),'test',repeat('b',64),'test',repeat('c',64),'[]'::jsonb,'test','test'
 FROM parcel_evidence_observations WHERE artifact_id='revision-lock'`); err != nil {
		t.Fatal(err)
	}
	evidence, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer evidence.Rollback(context.Background())
	if _, err = evidence.Exec(ctx, `UPDATE parcel_evidence_current SET normalized_hash=repeat('d',64) WHERE source_parcel_id='revision-lock-a'`); err != nil {
		t.Fatal(err)
	}
	shape, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer shape.Rollback(context.Background())
	if _, err = shape.Exec(ctx, `SET LOCAL lock_timeout='2s'`); err != nil {
		t.Fatal(err)
	}
	var pid int
	if err = shape.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		_, e := shape.Exec(ctx, `UPDATE parcels SET shape=ST_Translate(shape,1,0) WHERE source_parcel_id='revision-lock-a'`)
		result <- e
	}()
	// Wait until the shape writer is blocked on source revision, not a timer guess.
	blocked := false
	for !blocked && ctx.Err() == nil {
		var waiting bool
		if err = pool.QueryRow(ctx, `SELECT coalesce(wait_event_type='Lock',false) FROM pg_stat_activity WHERE pid=$1`, pid).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		blocked = waiting
		if !blocked {
			time.Sleep(5 * time.Millisecond)
		}
	}
	if !blocked {
		t.Fatal("shape writer did not wait on source revision")
	}
	if _, err = evidence.Exec(ctx, `SET LOCAL lock_timeout='2s'`); err != nil {
		t.Fatal(err)
	}
	if _, err = evidence.Exec(ctx, `UPDATE parcels SET shape=ST_Translate(shape,0,1) WHERE source_parcel_id='revision-lock-a'`); err != nil {
		t.Fatalf("revision lock order deadlocked: %v", err)
	}
	if err = evidence.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-result:
		if err != nil {
			t.Fatal("shape writer failed after evidence commit: ", err)
		}
	case <-ctx.Done():
		t.Fatal("shape writer remained blocked")
	}
	if err = shape.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}
