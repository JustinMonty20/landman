//go:build postgres

package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JustinMonty20/landman/internal/connector/union"
	"github.com/JustinMonty20/landman/internal/connector/union/parceljob"
	"github.com/JustinMonty20/landman/internal/ingest"
	"github.com/JustinMonty20/landman/internal/jobs"
	"github.com/JustinMonty20/landman/internal/parcelevidence"
	"github.com/jackc/pgx/v5/pgxpool"
)

func replayFollowup(t *testing.T, ctx context.Context, dsn string, poolRunner *jobs.Runner, pool *pgxpool.Pool, id string) (jobs.Outcome, jobs.Result, error) {
	t.Helper()
	return poolRunner.Run(ctx, jobs.Definition{ID: parcelJobID, Kind: jobs.AttemptReplay, Run: func(ctx context.Context) (jobs.Result, error) {
		return (parceljob.Job{Root: os.Getenv("PARCEL_EVIDENCE_DIR"), DSN: dsn, Quarantine: jobs.NewQuarantineStore(pool)}).Replay(ctx, id)
	}})
}

func TestRegisteredCaptureMismatchRejectedOnEmptyStore(t *testing.T) {
	ctx, dsn, pool := parcelFixtureDB(t)
	f := &parcelHTTPFixture{features: []map[string]interface{}{feature(1, "A", "owner", true, 0)}}
	if _, err := pool.Exec(ctx, `ALTER TABLE parcels ADD CHECK(source_parcel_id <> 'A')`); err != nil {
		t.Fatal(err)
	}
	if out, _, err := runParcelFixture(ctx, pool, dsn, f.source(t, 1), false, 0); err == nil || out != jobs.OutcomeFailed {
		t.Fatalf("failed initial capture: %s %v", out, err)
	}
	var id string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM ingest_captures`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	var data []byte
	if err := pool.QueryRow(ctx, `SELECT manifest FROM ingest_captures WHERE id=$1::uuid`, id).Scan(&data); err != nil {
		t.Fatal(err)
	}
	var manifest ingest.Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest.ObservedAt = manifest.ObservedAt.Add(time.Second)
	altered, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE ingest_captures SET manifest=$2 WHERE id=$1::uuid`, id, altered); err != nil {
		t.Fatal(err)
	}
	runner, _ := jobs.NewRunner(pool)
	if out, _, err := replayFollowup(t, ctx, dsn, runner, pool, id); err == nil || out != jobs.OutcomeFailed {
		t.Fatalf("mismatched registered capture accepted: %s %v", out, err)
	}
	if countRows(t, ctx, pool, "parcels") != 0 || countRows(t, ctx, pool, "parcel_evidence_observations") != 0 {
		t.Fatal("mismatched registered capture published business rows")
	}
}

func TestRegisteredMixedRetryPreservesNewerPID(t *testing.T) {
	ctx, dsn, pool := parcelFixtureDB(t)
	f := &parcelHTTPFixture{features: []map[string]interface{}{feature(1, "A", "old A", true, 0), feature(2, "B", "old B", true, 0)}}
	source := f.source(t, 2)
	if _, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err != nil {
		t.Fatal(err)
	}
	f.features = []map[string]interface{}{feature(1, "A", "retry A", true, 0), feature(2, "B", "retry B", true, 0)}
	if _, err := pool.Exec(ctx, `ALTER TABLE parcels ADD CONSTRAINT fail_followup CHECK(current_name_1 <> 'retry A')`); err != nil {
		t.Fatal(err)
	}
	if out, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err == nil || out != jobs.OutcomeFailed {
		t.Fatalf("expected registered failure: %s %v", out, err)
	}
	var id string
	if err := pool.QueryRow(ctx, `SELECT c.id::text FROM ingest_captures c JOIN job_attempts a ON a.id=c.attempt_id WHERE a.outcome='failed'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE parcels DROP CONSTRAINT fail_followup`); err != nil {
		t.Fatal(err)
	}
	f.features = []map[string]interface{}{feature(2, "B", "latest B", true, 0)}
	if _, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err != nil {
		t.Fatal(err)
	}
	runner, _ := jobs.NewRunner(pool)
	out, result, err := replayFollowup(t, ctx, dsn, runner, pool, id)
	if err != nil || out != jobs.OutcomeSuccess || result.Processed != 1 {
		t.Fatalf("mixed retry failed to publish eligible A: %s %+v %v", out, result, err)
	}
	var a, b string
	if err := pool.QueryRow(ctx, `SELECT (SELECT current_name_1 FROM parcels WHERE source_parcel_id='A'), (SELECT current_name_1 FROM parcels WHERE source_parcel_id='B')`).Scan(&a, &b); err != nil {
		t.Fatal(err)
	}
	if a != "retry A" || b != "latest B" {
		t.Fatalf("mixed retry overwrote newer PID: A=%q B=%q", a, b)
	}
	var originalID string
	if err := pool.QueryRow(ctx, `SELECT artifact_id FROM parcel_evidence_observations WHERE publication_kind='observed' ORDER BY observed_at LIMIT 1`).Scan(&originalID); err != nil {
		t.Fatal(err)
	}
	before := countRows(t, ctx, pool, "parcel_derived_handoff")
	out, result, err = replayFollowup(t, ctx, dsn, runner, pool, originalID)
	if err != nil || out != jobs.OutcomeSuccess || result.Processed != 0 || countRows(t, ctx, pool, "parcel_derived_handoff") != before {
		t.Fatalf("historical validation requested derived work: %s %+v %v", out, result, err)
	}
}

func TestFeatureLatestWKIDIsServedSourceSRID(t *testing.T) {
	ctx, dsn, pool := parcelFixtureDB(t)
	f := &parcelHTTPFixture{features: []map[string]interface{}{feature(1, "A", "owner", true, 0)}, crs: 102100}
	f.features[0]["geometry"].(map[string]interface{})["spatialReference"] = map[string]interface{}{"latestWkid": 3857}
	if _, _, err := runParcelFixture(ctx, pool, dsn, f.source(t, 1), false, 0); err != nil {
		t.Fatal(err)
	}
	var declared int
	if err := pool.QueryRow(ctx, `SELECT source_srid FROM parcels WHERE source_parcel_id='A'`).Scan(&declared); err != nil || declared != 3857 {
		t.Fatalf("feature declared latestWkid was not stored: %d %v", declared, err)
	}
}

func TestExactAcceptedNumericStringAndWideSourceArea(t *testing.T) {
	ctx, dsn, pool := parcelFixtureDB(t)
	f := &parcelHTTPFixture{features: []map[string]interface{}{feature(1, "A", "owner", true, 1)}}
	attrs := f.features[0]["attributes"].(map[string]interface{})
	attrs["TOTVAL"] = "12.3449999999999999999"
	attrs["s1_SALESAMT"] = "12.3449999999999999999"
	attrs["SHAPE.STArea()"] = json.Number("1000000000000.000001")
	attrs["SHAPE.STLength()"] = json.Number("1e-6")
	source := f.source(t, 1)
	if _, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err != nil {
		t.Fatal(err)
	}
	var value, area, length, sale, history, hash string
	if err := pool.QueryRow(ctx, `SELECT p.valuation_assessed::text,p.source_area::text,p.source_length::text,(SELECT amount::text FROM parcel_sales WHERE parcel_id=p.id),h.new_fields->>'valuation_assessed',c.normalized_hash FROM parcels p JOIN parcel_evidence_current c USING(county_id,source_name,source_parcel_id) JOIN parcel_evidence_history h USING(county_id,source_name,source_parcel_id)`).Scan(&value, &area, &length, &sale, &history, &hash); err != nil {
		t.Fatal(err)
	}
	if value != "12.34" || history != value || sale != value || area != "1000000000000.000001" || length != "0.000001" {
		t.Fatalf("numeric precision lost: value=%s history=%s sale=%s area=%s length=%s", value, history, sale, area, length)
	}
	attrs["SHAPE.STArea()"] = json.Number("1000000000000.000002")
	if _, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err != nil {
		t.Fatal(err)
	}
	var nextHash, nextArea string
	if err := pool.QueryRow(ctx, `SELECT c.normalized_hash,p.source_area::text FROM parcel_evidence_current c JOIN parcels p USING(county_id,source_name,source_parcel_id)`).Scan(&nextHash, &nextArea); err != nil {
		t.Fatal(err)
	}
	if nextArea != "1000000000000.000002" || nextHash == hash {
		t.Fatalf("wide decimal source update lost: %s %s -> %s", area, hash, nextArea)
	}
}

func TestFailedRegisteredRepairReplayKeepsUnverifiableIdentityPending(t *testing.T) {
	ctx, dsn, pool := parcelFixtureDB(t)
	f := &parcelHTTPFixture{features: []map[string]interface{}{feature(1, "A", "bad", false, 0)}}
	source := f.source(t, 1)
	if out, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err != nil || out != jobs.OutcomeSuccessWithQuarantine {
		t.Fatalf("initial quarantine: %s %v", out, err)
	}
	f.features[0] = feature(1, "A", "fixed", true, 0)
	if _, err := pool.Exec(ctx, `ALTER TABLE parcels ADD CONSTRAINT reject_followup_repair CHECK(current_name_1 <> 'fixed')`); err != nil {
		t.Fatal(err)
	}
	if out, _, err := runParcelFixture(ctx, pool, dsn, source, true, 1); err == nil || out != jobs.OutcomeFailed {
		t.Fatalf("expected registered repair publication failure: %s %v", out, err)
	}
	var id string
	if err := pool.QueryRow(ctx, `SELECT c.id::text FROM ingest_captures c JOIN job_attempts a ON a.id=c.attempt_id WHERE c.attempt_kind='reprocess' AND a.outcome='failed'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE parcels DROP CONSTRAINT reject_followup_repair`); err != nil {
		t.Fatal(err)
	}
	runner, _ := jobs.NewRunner(pool)
	if out, result, err := replayFollowup(t, ctx, dsn, runner, pool, id); err != nil || out != jobs.OutcomeSuccess || result.Processed != 1 {
		t.Fatalf("offline repair retry failed: %s %+v %v", out, result, err)
	}
	if countRows(t, ctx, pool, "quarantined_records WHERE reprocessed_at IS NULL") != 1 {
		t.Fatal("offline retry cannot verify original repair selection; evidence must stay pending")
	}
}

func TestFailedLaterRepairPageRetainsNewInvalidSibling(t *testing.T) {
	ctx, dsn, pool := parcelFixtureDB(t)
	f := &parcelHTTPFixture{features: []map[string]interface{}{feature(1, "A", "bad", false, 0), feature(2, "B", "good", true, 0)}}
	source := f.source(t, 1)
	if out, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err != nil || out != jobs.OutcomeSuccessWithQuarantine {
		t.Fatalf("initial invalid selection: %s %v", out, err)
	}
	f.features = []map[string]interface{}{feature(1, "A", "fixed", true, 0), feature(3, "A", "new bad", false, 0), feature(4, "A", "last", true, 0)}
	f.failOffset, f.failStatus = 2, 500
	if out, _, err := runParcelFixture(ctx, pool, dsn, source, true, 100); err == nil || out != jobs.OutcomeFailed {
		t.Fatalf("later repair failure not observed: %s %v", out, err)
	}
	var reference string
	if err := pool.QueryRow(ctx, `SELECT payload_reference FROM quarantined_records WHERE raw_record_key='3' AND reprocessed_at IS NULL`).Scan(&reference); err != nil {
		t.Fatalf("earlier repair page invalid sibling was lost: %v", err)
	}
	path, digestIndex, ok := strings.Cut(reference, "@")
	if !ok || !strings.HasSuffix(digestIndex, "#0") {
		t.Fatalf("invalid repair receipt: %q", reference)
	}
	body, err := os.ReadFile(filepath.Join(os.Getenv("PARCEL_EVIDENCE_DIR"), path))
	if err != nil || !strings.HasPrefix(digestIndex, fmt.Sprintf("%x", sha256.Sum256(body))) {
		t.Fatalf("repair receipt not backed by exact bytes: %q %v", reference, err)
	}
	if countRows(t, ctx, pool, "parcels") != 1 {
		t.Fatal("failed repair partially published new PID")
	}
}

func TestLargeUnsupportedIdentityAndExponentKeepNeighborAvailable(t *testing.T) {
	ctx, dsn, pool := parcelFixtureDB(t)
	pid := strings.Repeat("p", 9000)
	bad := feature(1, pid, "bad", false, 0)
	bad["attributes"].(map[string]interface{})["UNUSED_EXPONENT"] = json.Number("1e-65536")
	f := &parcelHTTPFixture{features: []map[string]interface{}{bad, feature(2, "B", "good", true, 0)}}
	source := f.source(t, 2)
	out, result, err := runParcelFixture(ctx, pool, dsn, source, false, 0)
	if err != nil || out != jobs.OutcomeSuccessWithQuarantine || result.Processed != 1 {
		t.Fatalf("large invalid feature blocked valid PID: %s %+v %v", out, result, err)
	}
	var savedPID, reference string
	var payloadBytes int
	if err := pool.QueryRow(ctx, `SELECT source_parcel_id,payload_reference,coalesce(octet_length(payload::text),0) FROM quarantined_records`).Scan(&savedPID, &reference, &payloadBytes); err != nil {
		t.Fatal(err)
	}
	if savedPID != pid || payloadBytes > 8192 || !strings.Contains(reference, "@") {
		t.Fatalf("unrepairable bounded quarantine: identity=%d payload=%d receipt=%q", len(savedPID), payloadBytes, reference)
	}
	f.features[0] = feature(1, pid, "repaired", true, 0)
	out, result, err = runParcelFixture(ctx, pool, dsn, source, true, 100)
	if err != nil || out != jobs.OutcomeSuccessWithQuarantine || result.Processed != 0 || countRows(t, ctx, pool, "parcels") != 1 || countRows(t, ctx, pool, "quarantined_records WHERE reprocessed_at IS NULL") != 1 {
		t.Fatalf("unsupported identity was served or lost: %s %+v %v", out, result, err)
	}
}

func TestHashVersionTransitionDoesNotWriteBusinessHistory(t *testing.T) {
	ctx, dsn, pool := parcelFixtureDB(t)
	f := &parcelHTTPFixture{features: []map[string]interface{}{feature(1, "A", "same", true, 0)}}
	source := f.source(t, 1)
	if _, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err != nil {
		t.Fatal(err)
	}
	var sourceFeatures []byte
	var geometryHash, timestamp string
	if err := pool.QueryRow(ctx, `SELECT c.source_features,c.geometry_hash,p.updated_at::text FROM parcel_evidence_current c JOIN parcels p USING(county_id,source_name,source_parcel_id)`).Scan(&sourceFeatures, &geometryHash, &timestamp); err != nil {
		t.Fatal(err)
	}
	var features []json.RawMessage
	if err := json.Unmarshal(sourceFeatures, &features); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"spatialReference": map[string]any{"wkid": 3857}, "features": features})
	records, err := union.DecodeParcelPage(body, time.Now(), "union_county_gis")
	if err != nil {
		t.Fatal(err)
	}
	projection, err := union.NormalizeParcel(records[0])
	if err != nil {
		t.Fatal(err)
	}
	projection.ID = ""
	projection.Geometry.Shape = nil
	legacyPayload, err := json.Marshal(struct {
		Parcel   any    `json:"parcel"`
		Geometry string `json:"geometry_hash"`
	}{projection, geometryHash})
	if err != nil {
		t.Fatal(err)
	}
	legacy := fmt.Sprintf("%x", sha256.Sum256(append([]byte("parcel-storage-v1\n"), legacyPayload...)))
	if _, err := pool.Exec(ctx, `UPDATE parcel_evidence_current SET normalized_hash=$1,normalized_hash_version='parcel-storage-v1'`, legacy); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err != nil {
		t.Fatal(err)
	}
	var now, version string
	if err := pool.QueryRow(ctx, `SELECT p.updated_at::text,c.normalized_hash_version FROM parcels p JOIN parcel_evidence_current c USING(county_id,source_name,source_parcel_id)`).Scan(&now, &version); err != nil {
		t.Fatal(err)
	}
	if now != timestamp || version != parcelevidence.NormalizedHashVersion || countRows(t, ctx, pool, "parcel_evidence_history") != 1 {
		t.Fatalf("version-only change rewrote parcel/history: %s -> %s version=%s", timestamp, now, version)
	}
	if _, err := pool.Exec(ctx, `UPDATE parcel_evidence_current SET normalized_hash=repeat('0',64),normalized_hash_version='parcel-storage-v1'`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT p.updated_at::text,c.normalized_hash_version FROM parcels p JOIN parcel_evidence_current c USING(county_id,source_name,source_parcel_id)`).Scan(&now, &version); err != nil {
		t.Fatal(err)
	}
	if now != timestamp || version != parcelevidence.NormalizedHashVersion || countRows(t, ctx, pool, "parcel_evidence_history") != 1 {
		t.Fatalf("incomparable version with same stored projection rewrote parcel/history: %s -> %s version=%s", timestamp, now, version)
	}
}

func TestLiveParcelPublicationRequestsDerivedHandoff(t *testing.T) {
	ctx, dsn, pool := parcelFixtureDB(t)
	fixture := &parcelHTTPFixture{features: []map[string]interface{}{feature(1, "A", "owner", true, 0)}}
	source := fixture.source(t, 1)
	if out, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err != nil || out != jobs.OutcomeSuccess {
		t.Fatalf("initial parcel import: %s %v", out, err)
	}
	var generation int64
	var screen, qualification bool
	if err := pool.QueryRow(ctx, `SELECT request_generation,screening_pending,qualification_pending FROM parcel_derived_handoff`).Scan(&generation, &screen, &qualification); err != nil || !screen || !qualification {
		t.Fatalf("live geometry did not request both derived jobs: %d %t %t %v", generation, screen, qualification, err)
	}
	if out, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err != nil || out != jobs.OutcomeSuccess {
		t.Fatalf("repeat parcel import: %s %v", out, err)
	}
	var after int64
	if err := pool.QueryRow(ctx, `SELECT request_generation FROM parcel_derived_handoff`).Scan(&after); err != nil || after != generation {
		t.Fatalf("unchanged parcel advanced derived generation: %d -> %d (%v)", generation, after, err)
	}
	fixture.features = []map[string]interface{}{feature(1, "A", "new owner", true, 0)}
	if out, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err != nil || out != jobs.OutcomeSuccess {
		t.Fatalf("ownership-only import: %s %v", out, err)
	}
	if countRows(t, ctx, pool, "parcel_evidence_history") != 2 || countRows(t, ctx, pool, "parcel_derived_handoff") != 1 {
		t.Fatal("ownership-only import lost history or enqueued derived work")
	}
	if err := pool.QueryRow(ctx, `SELECT request_generation FROM parcel_derived_handoff`).Scan(&after); err != nil || after != generation {
		t.Fatalf("ownership-only import advanced derived generation: %d -> %d (%v)", generation, after, err)
	}
}

func TestReplayDerivedHandoffPersistsAcrossPendingScreening(t *testing.T) {
	ctx, dsn, pool := parcelFixtureDB(t)
	f := &parcelHTTPFixture{features: []map[string]interface{}{feature(1, "A", "owner", true, 0)}}
	id, _ := recordOfflineParcelCapture(t, ctx, pool, f.source(t, 1))
	runner, _ := jobs.NewRunner(pool)
	out, result, err := replayFollowup(t, ctx, dsn, runner, pool, id)
	if err != nil || out != jobs.OutcomeSuccess || result.Processed != 1 {
		t.Fatalf("offline rebuild: %s %+v %v", out, result, err)
	}
	var screen, qualification bool
	var generation int64
	if err := pool.QueryRow(ctx, `SELECT screening_pending,qualification_pending,request_generation FROM parcel_derived_handoff WHERE artifact_id=$1`, id).Scan(&screen, &qualification, &generation); err != nil || !screen || !qualification {
		t.Fatalf("missing durable derived request: %t %t %v", screen, qualification, err)
	}
	out, result, err = replayFollowup(t, ctx, dsn, runner, pool, id)
	if err != nil || out != jobs.OutcomeSuccess || result.Processed != 1 {
		t.Fatalf("repeat current replay: %s %+v %v", out, result, err)
	}
	var requests int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM parcel_derived_handoff`).Scan(&requests); err != nil || requests != 1 {
		t.Fatalf("duplicate handoff %d %v", requests, err)
	}
	var after int64
	if err := pool.QueryRow(ctx, `SELECT request_generation FROM parcel_derived_handoff WHERE artifact_id=$1`, id).Scan(&after); err != nil || after != generation {
		t.Fatalf("unchanged current replay advanced derived generation: %d -> %d (%v)", generation, after, err)
	}
	t.Setenv("DATABASE_URL", dsn)
	blockedRoot := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blockedRoot, []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PARCEL_EVIDENCE_DIR", filepath.Join(blockedRoot, "soil"))
	args := os.Args
	defer func() { os.Args = args }()
	os.Args = []string{"landman", "run", qualificationJobID}
	if err := run(ctx); err != nil {
		t.Fatalf("stored-input qualification recovery: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT screening_pending,qualification_pending FROM parcel_derived_handoff WHERE artifact_id=$1`, id).Scan(&screen, &qualification); err != nil || !screen || qualification {
		t.Fatalf("qualification recovery cleared wrong handoff: screening=%t qualification=%t err=%v", screen, qualification, err)
	}
}
