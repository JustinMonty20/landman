//go:build postgres

package vacancy

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/JustinMonty20/landman/internal/parcelapi"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPublicationAndGenerationInvalidation(t *testing.T) {
	ctx := context.Background()
	dsn := os.Getenv("API_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("API_TEST_DATABASE_URL not set")
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var db string
	if err = pool.QueryRow(ctx, `SELECT current_database()`).Scan(&db); err != nil || !strings.HasPrefix(db, "landman_api_test_") {
		t.Fatal("requires disposable database", err)
	}
	if err = Publish(ctx, pool); err == nil {
		t.Fatal("published without reviewed live generations")
	}
	raw, err := os.ReadFile("testdata/publication.sql")
	if err != nil {
		t.Fatal(err)
	}
	threeReceipts := regexp.MustCompile(`"receipts":\[[^]]+\]`)
	spans := threeReceipts.FindAllStringIndex(string(raw), -1)
	if len(spans) != 4 {
		t.Fatal("expected both fixture receipt tapes and their digests")
	}
	impossible := string(raw[:spans[2][0]]) + `"receipts":[{"http_status":200,"response_complete":true,"revision":"assessment-count-first-v1","request":"https://example.test/0/query?f=json&returnCountOnly=true&where=1%3D1"},{"http_status":200,"response_complete":true,"revision":"assessment-count-first-v1","request":"https://example.test/0/query?f=json&returnCountOnly=true&where=OBJECTID"},{"http_status":200,"response_complete":true,"revision":"assessment-count-first-v1","request":"https://example.test/0/query?f=json&outFields=%2A&where=OBJECTID"}]` + string(raw[spans[2][1]:])
	denied, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer denied.Rollback(ctx)
	if _, err := denied.Exec(ctx, impossible); err == nil || !strings.Contains(err.Error(), "current capture protocol") {
		t.Fatalf("impossible count-first tape admitted: %v", err)
	}
	if err := denied.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	wrongSource := strings.Replace(string(raw), "'https://example.test/171/query','synthetic terms'", "'https://example.test/wrong/query','synthetic terms'", 1)
	if wrongSource == string(raw) {
		t.Fatal("source URL rejection fixture was not changed")
	}
	refusal, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer refusal.Rollback(ctx)
	if _, err = refusal.Exec(ctx, wrongSource); err == nil || !strings.Contains(err.Error(), "current capture protocol and complete live HTTP receipts") {
		t.Fatal("wrong live source URL admitted: ", err)
	}
	if err = refusal.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	future := strings.Replace(string(raw), "current_date,2026 FROM vacancy_test_g", "current_date,9999 FROM vacancy_test_g", 1)
	if future == string(raw) {
		t.Fatal("future year fixture not changed")
	}
	refusal, err = pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer refusal.Rollback(ctx)
	if _, err = refusal.Exec(ctx, future); err == nil || !strings.Contains(err.Error(), "future assessment year cannot be admitted") {
		t.Fatalf("future year admitted: %v", err)
	}
	if err = refusal.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	legacy := string(raw)
	receipts := regexp.MustCompile(`"receipts":\[[^]]+\]`)
	location := receipts.FindStringIndex(legacy)
	if location == nil {
		t.Fatal("no retained receipt fixture")
	}
	legacy = legacy[:location[0]] + `"receipts":[{"http_status":200,"response_complete":true,"request":"https://example.test/171/query?where=1%3D1"},{"http_status":200,"response_complete":true,"request":"https://example.test/171/query?where=1%3D1"}]` + legacy[location[1]:]
	refusal, err = pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer refusal.Rollback(ctx)
	if _, err = refusal.Exec(ctx, legacy); err == nil || !strings.Contains(err.Error(), "current capture protocol") {
		t.Fatalf("legacy bounded capture admitted: %v", err)
	}
	if err = refusal.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, string(raw)); err != nil {
		tx.Rollback(ctx)
		t.Fatal("seed public synthetic source: ", err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal("commit synthetic source: ", err)
	}
	if err = Publish(ctx, pool); err != nil {
		t.Fatal("publish synthetic source: ", err)
	}
	var first, second time.Time
	if err = pool.QueryRow(ctx, `SELECT assessed_at FROM vacancy_assessments a JOIN parcels p ON p.id=a.parcel_id WHERE p.source_parcel_id='vacancy-test-a'`).Scan(&first); err != nil {
		t.Fatal(err)
	}
	if err = Publish(ctx, pool); err != nil {
		t.Fatal("repeat identical publication: ", err)
	}
	if err = pool.QueryRow(ctx, `SELECT assessed_at FROM vacancy_assessments a JOIN parcels p ON p.id=a.parcel_id WHERE p.source_parcel_id='vacancy-test-a'`).Scan(&second); err != nil || !second.Equal(first) {
		t.Fatalf("repeat rewrote unchanged assessment: %v / %v: %v", first, second, err)
	}
	if _, err = pool.Exec(ctx, `UPDATE vacancy_live_validation SET effective_av_year=2027 WHERE source_name='union_assessed_value_0'`); err == nil {
		t.Fatal("allowed in-place change to approved effective AV year")
	}
	for _, want := range []struct{ pid, tier string }{{"vacancy-test-a", "strong_candidate"}, {"vacancy-test-b", "review"}, {"vacancy-test-c", "conflict"}} {
		var got string
		err = pool.QueryRow(ctx, `SELECT a.tier FROM vacancy_assessments a JOIN parcels p ON p.id=a.parcel_id WHERE p.source_parcel_id=$1`, want.pid).Scan(&got)
		if err != nil || got != want.tier {
			t.Fatalf("%s tier=%q, want %q: %v", want.pid, got, want.tier, err)
		}
	}
	handler := parcelapi.New(pool)
	request := func(path string) map[string]any {
		t.Helper()
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 {
			t.Fatalf("GET %s status %d: %s", path, w.Code, w.Body.String())
		}
		var result map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	var id string
	if err = pool.QueryRow(ctx, `SELECT id::text FROM parcels WHERE source_parcel_id='vacancy-test-a'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	box := "bbox=-0.00001,-0.00001,0.0003,0.0002"
	if got := request("/api/v1/parcels?" + box + "&vacancy_tier=strong_candidate"); got["count"] != float64(1) {
		t.Fatalf("strong list: %v", got)
	}
	if got := request("/api/v1/parcels/map?" + box + "&zoom=14&vacancy_tier=strong_candidate"); got["match_count"] != float64(1) {
		t.Fatalf("strong map: %v", got)
	}
	for _, tier := range []string{"review", "conflict"} {
		if got := request("/api/v1/parcels?" + box + "&vacancy_tier=" + tier); got["count"] != float64(1) {
			t.Fatalf("%s list: %v", tier, got)
		}
		if got := request("/api/v1/parcels/map?" + box + "&zoom=14&vacancy_tier=" + tier); got["match_count"] != float64(1) {
			t.Fatalf("%s map: %v", tier, got)
		}
	}
	if got := request("/api/v1/parcels/" + id + "/vacancy"); got["tier"] != "strong_candidate" || got["available"] != true || got["assessment_source_as_of"] == nil || got["structure_source_as_of"] == nil || got["facts"].(map[string]any)["effective_av_year"] != float64(2026) {
		t.Fatalf("strong detail or provenance: %v", got)
	}
	// Neither a fallback OBJECTID nor an earlier multipart building report
	// may be interpreted as an unambiguous empty parcel.
	for _, sample := range []struct{ raw, tier string }{
		{`[{"attributes":{"PID":"vacancy-test-a","YEARBLT":" 0 ","SQFT":"0"}}]`, "strong_candidate"},
		{`[{"attributes":{"PID":"vacancy-test-a","YEARBLT":" ","SQFT":0}}]`, "unknown"},
		{`[{"attributes":{"PID":"vacancy-test-a","YEARBLT":"\t\n\r\u000b\u000c\u00a0\u2003","SQFT":0}}]`, "unknown"},
		{`[{"attributes":{"PID":"vacancy-test-a","YEARBLT":"0x0p0","SQFT":0}}]`, "unknown"},
		{`[{"attributes":{"PID":"vacancy-test-a","YEARBLT":"1e9999999","SQFT":0}}]`, "unknown"},
		{`[{"attributes":{"PID":"vacancy-test-a","YEARBLT":"0e9999999","SQFT":0}}]`, "strong_candidate"},
		{`[{"attributes":{"PID":"vacancy-test-a","YEARBLT":null,"SQFT":0}}]`, "unknown"},
	} {
		if _, err := pool.Exec(ctx, `UPDATE parcel_evidence_current SET source_features=$1::jsonb WHERE source_parcel_id='vacancy-test-a'`, sample.raw); err != nil {
			t.Fatal(err)
		}
		if err := Publish(ctx, pool); err != nil {
			t.Fatal("accepted numeric spelling aborted publication: ", err)
		}
		if got := request("/api/v1/parcels/" + id + "/vacancy"); got["tier"] != sample.tier {
			t.Fatalf("source %s tier=%v want %s", sample.raw, got["tier"], sample.tier)
		}
	}
	for _, features := range []string{
		`[{"attributes":{"OBJECTID":1,"PID":"","YEARBLT":0,"SQFT":0}}]`,
		`[{"attributes":{"OBJECTID":1,"PID":"vacancy-test-a","YEARBLT":2001,"SQFT":100}},
		  {"attributes":{"OBJECTID":2,"PID":"vacancy-test-a","YEARBLT":0,"SQFT":0}}]`,
	} {
		if _, err = pool.Exec(ctx, `UPDATE parcel_evidence_current SET source_features=$1::jsonb WHERE source_parcel_id='vacancy-test-a'`, features); err != nil {
			t.Fatal(err)
		}
		if err = Publish(ctx, pool); err != nil {
			t.Fatal(err)
		}
		if got := request("/api/v1/parcels/" + id + "/vacancy"); got["tier"] != "unknown" {
			t.Fatalf("ambiguous source reported favorable tier: %v", got)
		}
	}
	if _, err = pool.Exec(ctx, `UPDATE parcel_evidence_current SET source_features=jsonb_build_array(jsonb_build_object('attributes',jsonb_build_object('PID','vacancy-test-a','YEARBLT',0,'SQFT',0))) WHERE source_parcel_id='vacancy-test-a'`); err != nil {
		t.Fatal(err)
	}
	if err = Publish(ctx, pool); err != nil {
		t.Fatal(err)
	}
	// A footprint shared by a newly inserted neighbor withdraws the unchanged
	// assessed parcel. Rollbacks must not advance the transactional revision.
	var neighborID, assessedID string
	var assessedRowBefore, assessedRowAfter string
	if err = pool.QueryRow(ctx, `SELECT id::text,xmin::text FROM parcels WHERE source_parcel_id='vacancy-test-b'`).Scan(&assessedID, &assessedRowBefore); err != nil {
		t.Fatal(err)
	}
	checkB := func(tier string, available bool) {
		t.Helper()
		if got := request("/api/v1/parcels/" + assessedID + "/vacancy"); got["tier"] != tier || got["available"] != available {
			t.Fatalf("neighbor detail want %s/%v: %v", tier, available, got)
		}
		for _, path := range []string{"/api/v1/parcels?" + box + "&vacancy_tier=review", "/api/v1/parcels/map?" + box + "&zoom=14&vacancy_tier=review"} {
			got := request(path)
			key := "count"
			if strings.Contains(path, "/map?") {
				key = "match_count"
			}
			expected := float64(0)
			if tier == "review" {
				expected = 1
			}
			if got[key] != expected {
				t.Fatalf("neighbor %s want %v: %v", path, expected, got)
			}
		}
	}
	insertNeighbor := `INSERT INTO parcels(county_id,county_code,source_name,source_parcel_id,parcel_number,shape) VALUES ('union','179','union_county_gis','vacancy-topology-neighbor','vacancy-topology-neighbor',ST_MakeEnvelope(11,2,14,4,3857)) RETURNING id::text`
	rollback, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rollback.Rollback(ctx)
	if err = rollback.QueryRow(ctx, insertNeighbor).Scan(&neighborID); err != nil {
		t.Fatal(err)
	}
	if err = rollback.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	checkB("review", true)
	if err = pool.QueryRow(ctx, insertNeighbor).Scan(&neighborID); err != nil {
		t.Fatal(err)
	}
	checkB("unknown", false)
	if err = Publish(ctx, pool); err != nil {
		t.Fatal(err)
	}
	checkB("unknown", true)
	rollback, err = pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rollback.Rollback(ctx)
	if _, err = rollback.Exec(ctx, `UPDATE parcels SET shape=ST_MakeEnvelope(40,0,41,1,3857) WHERE id=$1::uuid`, neighborID); err != nil {
		t.Fatal(err)
	}
	if err = rollback.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	checkB("unknown", true)
	if _, err = pool.Exec(ctx, `UPDATE parcels SET shape=ST_MakeEnvelope(40,0,41,1,3857) WHERE id=$1::uuid`, neighborID); err != nil {
		t.Fatal(err)
	}
	checkB("unknown", false)
	if err = Publish(ctx, pool); err != nil {
		t.Fatal(err)
	}
	checkB("review", true)
	// An idempotent neighbor geometry update does not withdraw the assessment.
	if _, err = pool.Exec(ctx, `UPDATE parcels SET shape=ST_MakeEnvelope(40,0,41,1,3857) WHERE id=$1::uuid`, neighborID); err != nil {
		t.Fatal(err)
	}
	checkB("review", true)
	rollback, err = pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rollback.Rollback(ctx)
	if _, err = rollback.Exec(ctx, `DELETE FROM parcels WHERE id=$1::uuid`, neighborID); err != nil {
		t.Fatal(err)
	}
	if err = rollback.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	checkB("review", true)
	if _, err = pool.Exec(ctx, `UPDATE parcels SET shape=ST_MakeEnvelope(11,2,14,4,3857) WHERE id=$1::uuid`, neighborID); err != nil {
		t.Fatal(err)
	}
	if err = Publish(ctx, pool); err != nil {
		t.Fatal(err)
	}
	checkB("unknown", true)
	if _, err = pool.Exec(ctx, `DELETE FROM parcels WHERE id=$1::uuid`, neighborID); err != nil {
		t.Fatal(err)
	}
	checkB("unknown", false)
	if err = Publish(ctx, pool); err != nil {
		t.Fatal(err)
	}
	checkB("review", true)
	if err = pool.QueryRow(ctx, `SELECT xmin::text FROM parcels WHERE id=$1::uuid`, assessedID).Scan(&assessedRowAfter); err != nil || assessedRowAfter != assessedRowBefore {
		t.Fatalf("assessed parcel changed while neighbor changed: %s / %s: %v", assessedRowBefore, assessedRowAfter, err)
	}
	var priorCapture, newerCapture string
	if err = pool.QueryRow(ctx, `SELECT capture_id::text FROM vacancy_latest_observations WHERE source_name='union_structures_171'`).Scan(&priorCapture); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `INSERT INTO ingest_captures(source_name,observed_at,manifest,manifest_sha256,storage_mode,published_at)
 VALUES('union_structures_171',now(),convert_to('{}','UTF8'),decode(repeat('a',64),'hex'),'local',now()) RETURNING id::text`).Scan(&newerCapture); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE vacancy_latest_observations SET capture_id=$1::uuid,observed_at=now() WHERE source_name='union_structures_171'`, newerCapture); err != nil {
		t.Fatal(err)
	}
	if got := request("/api/v1/parcels?" + box + "&vacancy_tier=strong_candidate"); got["count"] != float64(0) {
		t.Fatalf("unvalidated source refresh still serves strong: %v", got)
	}
	if got := request("/api/v1/parcels/" + id + "/vacancy"); got["tier"] != "unknown" || got["available"] != false {
		t.Fatalf("unvalidated source detail: %v", got)
	}
	if _, err = pool.Exec(ctx, `UPDATE vacancy_latest_observations SET capture_id=$1::uuid WHERE source_name='union_structures_171'`, priorCapture); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE parcels SET shape=NULL WHERE id=$1::uuid`, id); err != nil {
		t.Fatal(err)
	}
	if got := request("/api/v1/parcels/" + id + "/vacancy"); got["tier"] != "unknown" || got["available"] != false {
		t.Fatalf("refused geometry remained strong: %v", got)
	}
	if _, err = pool.Exec(ctx, `UPDATE parcels SET shape=ST_MakeEnvelope(0,0,10,10,3857) WHERE id=$1::uuid`, id); err != nil {
		t.Fatal(err)
	}
	if err = Publish(ctx, pool); err != nil {
		t.Fatal("reassess changed parcel without a new parcel observation: ", err)
	}
	if got := request("/api/v1/parcels/" + id + "/vacancy"); got["tier"] != "unknown" {
		t.Fatalf("changed parcel was promoted without new source evidence: %v", got)
	}
	_, err = pool.Exec(ctx, `UPDATE parcel_evidence_current SET normalized_hash=repeat('f',64) WHERE source_parcel_id='vacancy-test-a'`)
	if err != nil {
		t.Fatal(err)
	}
	if got := request("/api/v1/parcels?" + box + "&vacancy_tier=unknown"); got["count"] != float64(1) {
		t.Fatalf("stale list unknown: %v", got)
	}
	if got := request("/api/v1/parcels/map?" + box + "&zoom=14&vacancy_tier=strong_candidate"); got["match_count"] != float64(0) {
		t.Fatalf("stale map still strong: %v", got)
	}
	if got := request("/api/v1/parcels/" + id + "/vacancy"); got["tier"] != "unknown" || got["available"] != false {
		t.Fatalf("stale detail: %v", got)
	}
	// A corrected replay under the identical generation ID must withdraw both
	// the human validation and all assessments before typed rows can be rewritten.
	_, err = pool.Exec(ctx, `UPDATE vacancy_live_generations SET normalized_sha256=repeat('f',64) WHERE capture_id=(SELECT capture_id FROM vacancy_latest_observations WHERE source_name='union_structures_171')`)
	if err != nil {
		t.Fatal("corrected replay invalidation: ", err)
	}
	var n int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM vacancy_assessments a JOIN parcels p ON p.id=a.parcel_id WHERE p.source_parcel_id IN ('vacancy-test-a','vacancy-test-b')`).Scan(&n)
	if err != nil || n != 0 {
		t.Fatalf("stale assessments remain: %d %v", n, err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM vacancy_live_shapes WHERE generation_id=(SELECT id FROM vacancy_live_generations WHERE capture_id=(SELECT capture_id FROM vacancy_latest_observations WHERE source_name='union_structures_171'))`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("stale shapes remain: %d %v", n, err)
	}
	if err = Publish(ctx, pool); err == nil {
		t.Fatal("re-admitted invalidated generation")
	}
	crossing := strings.ReplaceAll(string(raw), "vacancy-test-a", "vacancy-cross-a")
	crossing = strings.ReplaceAll(crossing, "vacancy-test-b", "vacancy-cross-b")
	crossing = strings.ReplaceAll(crossing, "vacancy-test-c", "vacancy-cross-c")
	crossing = strings.ReplaceAll(crossing, "vacancy-publication-test", "vacancy-cross-publication-test")
	crossing = strings.Replace(crossing, `[[[10,1],[15,1],[15,5],[10,5],[10,1]]]`, `[[[5,1],[15,1],[15,5],[5,5],[5,1]]]`, 1)
	if crossing == string(raw) {
		t.Fatal("cross-boundary fixture unchanged")
	}
	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, crossing); err != nil {
		tx.Rollback(ctx)
		t.Fatal("seed crossing fixture: ", err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = Publish(ctx, pool); err != nil {
		t.Fatal("publish crossing fixture: ", err)
	}
	for _, pid := range []string{"vacancy-cross-a", "vacancy-cross-b"} {
		var tier string
		if err = pool.QueryRow(ctx, `SELECT a.tier FROM vacancy_assessments a JOIN parcels p ON p.id=a.parcel_id WHERE p.source_parcel_id=$1`, pid).Scan(&tier); err != nil || tier != "unknown" {
			t.Fatalf("crossing %s tier=%s want unknown: %v", pid, tier, err)
		}
	}
	statusCase := strings.ReplaceAll(string(raw), "vacancy-test-", "vacancy-status-")
	statusCase = strings.ReplaceAll(statusCase, "vacancy-publication-test", "vacancy-status-publication-test")
	statusCase = strings.Replace(statusCase, `'existing',3857`, `'retired',3857`, 1)
	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, statusCase); err != nil {
		tx.Rollback(ctx)
		t.Fatal("seed retired-status fixture: ", err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = Publish(ctx, pool); err != nil {
		t.Fatal("publish retired-status fixture: ", err)
	}
	var retiredTier string
	if err = pool.QueryRow(ctx, `SELECT a.tier FROM vacancy_assessments a JOIN parcels p ON p.id=a.parcel_id WHERE p.source_parcel_id='vacancy-status-b'`).Scan(&retiredTier); err != nil || retiredTier != "unknown" {
		t.Fatalf("unapproved structure status became %s: %v", retiredTier, err)
	}
	// Model the retained weak approval after migration: it remains recorded,
	// but neither publication nor the public API may use contract version 0.
	var formerID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM parcels WHERE source_parcel_id='vacancy-status-a'`).Scan(&formerID); err != nil {
		t.Fatal(err)
	}
	if got := request("/api/v1/parcels/" + formerID + "/vacancy"); got["tier"] != "strong_candidate" {
		t.Fatalf("fixture lacks formerly approved tier: %v", got)
	}
	weak, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer weak.Rollback(ctx)
	if _, err := weak.Exec(ctx, `ALTER TABLE vacancy_live_validation DISABLE TRIGGER vacancy_admit_capture_receipts_trigger`); err != nil {
		t.Fatal(err)
	}
	if _, err := weak.Exec(ctx, `UPDATE vacancy_live_validation SET capture_contract_version=0 WHERE capture_id=(SELECT capture_id FROM vacancy_latest_observations WHERE source_name='union_assessed_value_0')`); err != nil {
		t.Fatal(err)
	}
	if _, err := weak.Exec(ctx, `ALTER TABLE vacancy_live_validation ENABLE TRIGGER vacancy_admit_capture_receipts_trigger`); err != nil {
		t.Fatal(err)
	}
	if err := weak.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := Publish(ctx, pool); err == nil {
		t.Fatal("weak approval republished")
	}
	if got := request("/api/v1/parcels/" + formerID + "/vacancy"); got["tier"] != "unknown" || got["available"] != false {
		t.Fatalf("weak approval still public: %v", got)
	}
	checkAssessmentParcelLockOrder(t, pool)
}
