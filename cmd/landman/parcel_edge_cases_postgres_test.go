//go:build postgres

package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JustinMonty20/landman/internal/connector"
	"github.com/JustinMonty20/landman/internal/jobs"
	"github.com/JustinMonty20/landman/internal/parcelevidence"
)

func TestHashUpgradeComparesWithoutBusinessWrites(t *testing.T) {
	ctx, dsn, pool := parcelFixtureDB(t)
	f := &parcelHTTPFixture{features: []map[string]interface{}{feature(1, "A", "same", true, 1)}}
	source := f.source(t, 1)
	if _, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err != nil {
		t.Fatal(err)
	}
	var stamp, saleStamp string
	if err := pool.QueryRow(ctx, `SELECT p.updated_at::text,s.updated_at::text FROM parcels p JOIN parcel_sales s ON s.parcel_id=p.id`).Scan(&stamp, &saleStamp); err != nil {
		t.Fatal(err)
	}
	_, err := pool.Exec(ctx, `UPDATE parcel_evidence_current SET normalized_hash_version='parcel-storage-v1',normalized_hash=repeat('0',64);
 CREATE SEQUENCE parcel_update_probe;
 CREATE FUNCTION probe_parcel_update() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN PERFORM nextval('parcel_update_probe'); RETURN NEW; END $$;
 CREATE TRIGGER probe_parcel_update BEFORE UPDATE ON parcels FOR EACH ROW EXECUTE FUNCTION probe_parcel_update();
 CREATE SEQUENCE sale_update_probe;
 CREATE FUNCTION probe_sale_update() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN PERFORM nextval('sale_update_probe'); RETURN NEW; END $$;
 CREATE TRIGGER probe_sale_update BEFORE UPDATE ON parcel_sales FOR EACH ROW EXECUTE FUNCTION probe_sale_update()`)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err != nil {
		t.Fatal(err)
	}
	var parcelCalled, saleCalled bool
	var now, saleNow, version string
	if err := pool.QueryRow(ctx, `SELECT (SELECT is_called FROM parcel_update_probe),(SELECT is_called FROM sale_update_probe),p.updated_at::text,s.updated_at::text,c.normalized_hash_version FROM parcels p JOIN parcel_sales s ON s.parcel_id=p.id JOIN parcel_evidence_current c USING(county_id,source_name,source_parcel_id)`).Scan(&parcelCalled, &saleCalled, &now, &saleNow, &version); err != nil {
		t.Fatal(err)
	}
	if parcelCalled || saleCalled || now != stamp || saleNow != saleStamp || version != parcelevidence.NormalizedHashVersion || countRows(t, ctx, pool, "parcel_evidence_history") != 1 {
		t.Fatalf("unchanged upgrade wrote business rows: parcel=%v sale=%v timestamps=%s/%s version=%s", parcelCalled, saleCalled, now, saleNow, version)
	}
	if _, err := pool.Exec(ctx, `UPDATE parcel_evidence_current SET normalized_hash_version='parcel-storage-v1',normalized_hash=repeat('0',64)`); err != nil {
		t.Fatal(err)
	}
	f.features[0] = feature(1, "A", "changed", true, 1)
	if _, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err != nil {
		t.Fatal(err)
	}
	var owner string
	if err := pool.QueryRow(ctx, `SELECT current_name_1 FROM parcels`).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if owner != "changed" || countRows(t, ctx, pool, "parcel_evidence_history") != 2 {
		t.Fatalf("changed legacy source did not publish: owner=%s", owner)
	}
}

func TestNoncompressibleOversizedPIDPreservesEvidenceAndNeighbor(t *testing.T) {
	for _, valid := range []bool{false, true} {
		t.Run(fmt.Sprint(valid), func(t *testing.T) {
			ctx, dsn, pool := parcelFixtureDB(t)
			var pid strings.Builder
			for i := 0; i < 145; i++ {
				fmt.Fprintf(&pid, "%x", sha256.Sum256([]byte(fmt.Sprint(i))))
			}
			long := pid.String()
			f := &parcelHTTPFixture{features: []map[string]interface{}{feature(1, long, "oversized", valid, 0), feature(2, "B", "neighbor", true, 0)}}
			out, result, err := runParcelFixture(ctx, pool, dsn, f.source(t, 2), false, 0)
			if err != nil || result.Processed != 1 || result.Quarantined != 1 || out != jobs.OutcomeSuccessWithQuarantine {
				t.Fatalf("oversized pid stopped neighbor: %s %+v %v", out, result, err)
			}
			var stored, reference, reason string
			if err := pool.QueryRow(ctx, `SELECT source_parcel_id,payload_reference,reason FROM quarantined_records WHERE raw_record_key='1' AND reprocessed_at IS NULL`).Scan(&stored, &reference, &reason); err != nil {
				t.Fatal(err)
			}
			if stored != long || !strings.Contains(reason, "indexed key limit") {
				t.Fatalf("lost exact oversized identity: stored=%d reference=%q reason=%q", len(stored), reference, reason)
			}
			path, digestIndex, ok := strings.Cut(reference, "@")
			if !ok || !strings.HasSuffix(digestIndex, "#0") {
				t.Fatalf("invalid receipt %q", reference)
			}
			body, err := os.ReadFile(filepath.Join(os.Getenv("PARCEL_EVIDENCE_DIR"), path))
			if err != nil || !strings.HasPrefix(digestIndex, fmt.Sprintf("%x", sha256.Sum256(body))) || !strings.Contains(string(body), long) {
				t.Fatalf("receipt does not preserve oversized feature: %q %v", reference, err)
			}
			var owner string
			if err := pool.QueryRow(ctx, `SELECT current_name_1 FROM parcels WHERE source_parcel_id='B'`).Scan(&owner); err != nil || owner != "neighbor" {
				t.Fatal(owner, err)
			}
		})
	}
}

func TestFailedRepairReplayKeepsUnselectedAndReobservedEvidence(t *testing.T) {
	ctx, dsn, pool := parcelFixtureDB(t)
	f := &parcelHTTPFixture{features: []map[string]interface{}{feature(1, "A", "bad", false, 0), feature(2, "A", "bad", false, 0)}}
	source := f.source(t, 2)
	if _, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err != nil {
		t.Fatal(err)
	}
	f.features = []map[string]interface{}{feature(1, "A", "fixed", true, 0), feature(2, "A", "fixed", true, 0)}
	if _, err := pool.Exec(ctx, `ALTER TABLE parcels ADD CONSTRAINT fail_repair CHECK(current_name_1<>'fixed')`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runParcelFixture(ctx, pool, dsn, source, true, 1); err == nil {
		t.Fatal("repair did not fail")
	}
	var id string
	if err := pool.QueryRow(ctx, `SELECT artifact_id FROM parcel_evidence_captures WHERE attempt_kind='reprocess'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE parcels DROP CONSTRAINT fail_repair`); err != nil {
		t.Fatal(err)
	}
	f.features = []map[string]interface{}{feature(1, "A", "bad again", false, 0), feature(2, "A", "bad again", false, 0)}
	if _, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err != nil {
		t.Fatal(err)
	}
	runner, _ := jobs.NewRunner(pool)
	if _, _, err := replayFollowup(t, ctx, dsn, runner, pool, id); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, ctx, pool, "quarantined_records WHERE reprocessed_at IS NULL"); n != 2 {
		t.Fatalf("newer/unselected occurrences resolved: pending=%d", n)
	}
}

func TestFailedFullScanReplayKeepsLaterQuarantineOccurrence(t *testing.T) {
	ctx, dsn, pool := parcelFixtureDB(t)
	f := &parcelHTTPFixture{features: []map[string]interface{}{feature(1, "A", "invalid", false, 0)}}
	source := f.source(t, 1)
	if _, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err != nil {
		t.Fatal(err)
	}
	f.features[0] = feature(1, "A", "fixed", true, 0)
	if _, err := pool.Exec(ctx, `ALTER TABLE parcels ADD CONSTRAINT fail_full_retry CHECK(current_name_1<>'fixed')`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err == nil {
		t.Fatal("full scan did not fail")
	}
	var id string
	if err := pool.QueryRow(ctx, `SELECT c.artifact_id FROM parcel_evidence_captures c JOIN job_attempts a ON a.id=c.attempt_id WHERE c.attempt_kind='full_scan' AND a.outcome='failed'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE parcels DROP CONSTRAINT fail_full_retry`); err != nil {
		t.Fatal(err)
	}
	f.features[0] = feature(1, "A", "invalid again", false, 0)
	if _, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err != nil {
		t.Fatal(err)
	}
	runner, _ := jobs.NewRunner(pool)
	if _, _, err := replayFollowup(t, ctx, dsn, runner, pool, id); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, ctx, pool, "quarantined_records WHERE reprocessed_at IS NULL"); n != 1 {
		t.Fatalf("replay resolved later full-scan occurrence: pending=%d", n)
	}
}

func TestDerivedAcknowledgementRequiresPreworkRequestGeneration(t *testing.T) {
	ctx, dsn, pool := parcelFixtureDB(t)
	f := &parcelHTTPFixture{features: []map[string]interface{}{feature(1, "A", "owner", true, 0)}}
	snapshot, err := parcelevidence.New(os.Getenv("PARCEL_EVIDENCE_DIR"), "full_scan")
	if err != nil {
		t.Fatal(err)
	}
	n, err := f.source(t, 1).CaptureParcelPages(ctx, snapshot.Save, func([]connector.RawRecord) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err = snapshot.Complete(n); err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, fmt.Sprintf(`CREATE TABLE derived_seen(n int);
 CREATE FUNCTION pause_derived() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF NEW.job_id='%s' AND NEW.outcome='success' THEN
 INSERT INTO derived_seen SELECT count(*) FROM parcels;
 PERFORM pg_advisory_xact_lock(77740001);
 END IF; RETURN NEW; END $$;
 CREATE TRIGGER pause_derived BEFORE UPDATE ON job_attempts FOR EACH ROW EXECUTE FUNCTION pause_derived()`, qualificationJobID))
	if err != nil {
		t.Fatal(err)
	}
	pub, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer pub.Rollback(ctx)
	if _, err = pub.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext('union.parcel.evidence.publication'))"); err != nil {
		t.Fatal(err)
	}
	ack, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer ack.Rollback(ctx)
	if _, err = ack.Exec(ctx, "SELECT pg_advisory_xact_lock(77740001)"); err != nil {
		t.Fatal(err)
	}
	runner, _ := jobs.NewRunner(pool)
	replay := make(chan error, 1)
	go func() { _, _, e := replayFollowup(t, ctx, dsn, runner, pool, snapshot.Manifest.ID); replay <- e }()
	wait := func(want int) {
		t.Helper()
		for i := 0; i < 200; i++ {
			var k int
			if e := pool.QueryRow(ctx, "SELECT count(*) FROM pg_locks WHERE locktype='advisory' AND NOT granted").Scan(&k); e != nil {
				t.Fatal(e)
			}
			if k == want {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("missing %d advisory waiters", want)
	}
	wait(1)
	t.Setenv("DATABASE_URL", dsn)
	old := os.Args
	defer func() { os.Args = old }()
	os.Args = []string{"landman", "run", qualificationJobID}
	derived := make(chan error, 1)
	go func() { derived <- run(context.Background()) }()
	wait(2)
	if err = pub.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = <-replay; err != nil {
		t.Fatal(err)
	}
	if err = ack.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = <-derived; err != nil {
		t.Fatal(err)
	}
	var seen int
	var pending bool
	if err = pool.QueryRow(ctx, "SELECT n,qualification_pending FROM derived_seen CROSS JOIN parcel_derived_handoff").Scan(&seen, &pending); err != nil {
		t.Fatal(err)
	}
	if seen != 0 || !pending {
		t.Fatalf("qualification saw %d parcels, later replay published %d; pending=%v", seen, countRows(t, ctx, pool, "parcels"), pending)
	}
	requests, err := observedHandoffs(ctx, pool, "qualification_pending")
	if err != nil || len(requests) != 1 {
		t.Fatal(requests, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE parcel_derived_handoff SET request_generation=request_generation+1,qualification_pending=true`); err != nil {
		t.Fatal(err)
	}
	if err := acknowledgeHandoffs(ctx, pool, "qualification_pending", requests); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT qualification_pending FROM parcel_derived_handoff`).Scan(&pending); err != nil || !pending {
		t.Fatal("a new generation on an already observed artifact was acknowledged", err)
	}
	requests, err = observedHandoffs(ctx, pool, "qualification_pending")
	if err != nil {
		t.Fatal(err)
	}
	if err := acknowledgeHandoffs(ctx, pool, "qualification_pending", requests); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT qualification_pending FROM parcel_derived_handoff`).Scan(&pending); err != nil || pending {
		t.Fatal("the observed generation was not acknowledged", err)
	}
	screening, err := observedHandoffs(ctx, pool, "screening_pending")
	if err != nil || len(screening) != 1 {
		t.Fatal(screening, err)
	}
	if err := acknowledgeHandoffs(ctx, pool, "screening_pending", screening); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT screening_pending FROM parcel_derived_handoff`).Scan(&pending); err != nil || pending {
		t.Fatal("the observed screening generation was not acknowledged", err)
	}
}
