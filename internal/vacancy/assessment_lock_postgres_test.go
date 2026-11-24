//go:build postgres

package vacancy

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func checkAssessmentParcelLockOrder(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	defer func() {
		cleanup, done := context.WithTimeout(context.Background(), 3*time.Second)
		defer done()
		if _, err := pool.Exec(cleanup, `DELETE FROM parcels WHERE source_parcel_id IN ('assessment-lock-a','assessment-lock-b')`); err != nil {
			t.Error(err)
		}
	}()
	if _, err := pool.Exec(ctx, `INSERT INTO parcels(county_id,county_code,source_name,source_parcel_id,parcel_number,shape) VALUES
 ('union','179','union_county_gis','assessment-lock-a','assessment-lock-a',ST_MakeEnvelope(0,0,1,1,3857)),
 ('union','179','union_county_gis','assessment-lock-b','assessment-lock-b',ST_MakeEnvelope(2,0,3,1,3857))`); err != nil {
		t.Fatal(err)
	}
	assessment := `INSERT INTO vacancy_assessments(parcel_id,tier,rule_version,parcel_generation,structure_generation,assessment_generation,facts,reason)
 SELECT p.id,'unknown',1,'test',s.generation_id,v.generation_id,'{}', $1
 FROM parcels p
 CROSS JOIN vacancy_live_validation s CROSS JOIN vacancy_live_validation v
 WHERE s.source_name='union_structures_171' AND v.source_name='union_assessed_value_0'
 AND s.capture_id=(SELECT capture_id FROM vacancy_latest_observations WHERE source_name='union_structures_171')
 AND v.capture_id=(SELECT capture_id FROM vacancy_latest_observations WHERE source_name='union_assessed_value_0')
 AND p.source_parcel_id IN ('assessment-lock-a','assessment-lock-b')
 ON CONFLICT(parcel_id) DO UPDATE SET reason=EXCLUDED.reason`
	if _, err := pool.Exec(ctx, assessment, "seed"); err != nil {
		t.Fatal(err)
	}
	anchor, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer anchor.Rollback(context.Background())
	if _, err = anchor.Exec(ctx, `SELECT a.parcel_id FROM vacancy_assessments a JOIN parcels p ON p.id=a.parcel_id WHERE p.source_parcel_id='assessment-lock-b' FOR UPDATE OF a`); err != nil {
		t.Fatal(err)
	}
	upsert, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer upsert.Rollback(context.Background())
	if _, err = upsert.Exec(ctx, `SET LOCAL lock_timeout='4s'`); err != nil {
		t.Fatal(err)
	}
	var upsertPID int
	if err = upsert.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&upsertPID); err != nil {
		t.Fatal(err)
	}
	upsertDone := make(chan error, 1)
	go func() { _, e := upsert.Exec(ctx, assessment, "upsert"); upsertDone <- e }()
	wait := func(pid int) {
		t.Helper()
		waiting := false
		for !waiting && ctx.Err() == nil {
			if e := pool.QueryRow(ctx, `SELECT coalesce(wait_event_type='Lock',false) FROM pg_stat_activity WHERE pid=$1`, pid).Scan(&waiting); e != nil {
				t.Fatal(e)
			}
			if !waiting {
				time.Sleep(5 * time.Millisecond)
			}
		}
		if !waiting {
			t.Fatalf("backend %d did not reach lock wait", pid)
		}
	}
	wait(upsertPID)
	deletion, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer deletion.Rollback(context.Background())
	if _, err = deletion.Exec(ctx, `SET LOCAL lock_timeout='4s'`); err != nil {
		t.Fatal(err)
	}
	var deletePID int
	if err = deletion.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&deletePID); err != nil {
		t.Fatal(err)
	}
	deleteDone := make(chan error, 1)
	go func() {
		_, e := deletion.Exec(ctx, `DELETE FROM parcels WHERE source_parcel_id='assessment-lock-a'`)
		deleteDone <- e
	}()
	wait(deletePID)
	if err = anchor.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-upsertDone:
		if err != nil {
			t.Fatal("upsert deadlocked: ", err)
		}
	case <-ctx.Done():
		t.Fatal("upsert timed out")
	}
	if err = upsert.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-deleteDone:
		if err != nil {
			t.Fatal("delete deadlocked: ", err)
		}
	case <-ctx.Done():
		t.Fatal("delete timed out")
	}
	if err = deletion.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM vacancy_assessments a JOIN parcels p ON p.id=a.parcel_id WHERE p.source_parcel_id='assessment-lock-a'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("cascade failed: %d %v", count, err)
	}
}
