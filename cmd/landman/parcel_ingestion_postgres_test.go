//go:build postgres

package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/JustinMonty20/landman/internal/connector/union/parceljob"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JustinMonty20/landman/internal/connector/union"
	"github.com/JustinMonty20/landman/internal/jobs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func parcelFixtureDB(t *testing.T) (context.Context, string, *pgxpool.Pool) {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL required")
	}
	evidenceDir := t.TempDir()
	if err := os.Chmod(evidenceDir, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PARCEL_EVIDENCE_DIR", evidenceDir)
	ctx := context.Background()
	base, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("ingestion_%d", time.Now().UnixNano())
	name := pgx.Identifier{schema}.Sanitize()
	if _, err := base.Exec(ctx, "CREATE SCHEMA "+name); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"parcels", "parcel_sales", "job_attempts", "quarantined_records", "parcel_evidence_captures", "ingest_captures", "ingest_replay_attempts", "parcel_evidence_observations", "parcel_evidence_current", "parcel_evidence_history", "parcel_derived_handoff"} {
		if _, err := base.Exec(ctx, "CREATE TABLE "+name+"."+table+" (LIKE public."+table+" INCLUDING ALL) WITH (autovacuum_enabled=false)"); err != nil {
			t.Fatal(err)
		}
	}
	for _, sql := range []string{"ALTER TABLE " + name + ".parcel_sales ADD FOREIGN KEY(parcel_id) REFERENCES " + name + ".parcels(id)", "ALTER TABLE " + name + ".quarantined_records ADD FOREIGN KEY(attempt_id) REFERENCES " + name + ".job_attempts(id)", "ALTER TABLE " + name + ".parcel_evidence_captures ADD FOREIGN KEY(attempt_id) REFERENCES " + name + ".job_attempts(id)", "ALTER TABLE " + name + ".ingest_captures ADD FOREIGN KEY(attempt_id) REFERENCES " + name + ".job_attempts(id)", "ALTER TABLE " + name + ".ingest_replay_attempts ADD FOREIGN KEY(capture_id) REFERENCES " + name + ".ingest_captures(id)", "ALTER TABLE " + name + ".ingest_replay_attempts ADD FOREIGN KEY(attempt_id) REFERENCES " + name + ".job_attempts(id)", "ALTER TABLE " + name + ".parcel_evidence_observations ADD FOREIGN KEY(attempt_id) REFERENCES " + name + ".job_attempts(id)", "ALTER TABLE " + name + ".parcel_evidence_history ADD FOREIGN KEY(attempt_id) REFERENCES " + name + ".job_attempts(id)", "ALTER TABLE " + name + ".parcel_evidence_current ADD FOREIGN KEY(observation_id) REFERENCES " + name + ".parcel_evidence_observations(id)", "ALTER TABLE " + name + ".parcel_evidence_history ADD FOREIGN KEY(old_observation_id) REFERENCES " + name + ".parcel_evidence_observations(id)", "ALTER TABLE " + name + ".parcel_evidence_history ADD FOREIGN KEY(new_observation_id) REFERENCES " + name + ".parcel_evidence_observations(id)", "ALTER TABLE " + name + ".parcel_derived_handoff ADD FOREIGN KEY(artifact_id) REFERENCES " + name + ".parcel_evidence_observations(artifact_id)"} {
		if _, err := base.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		u, err := url.Parse(dsn)
		if err != nil {
			t.Fatal(err)
		}
		q := u.Query()
		q.Set("search_path", schema+",public")
		u.RawQuery = q.Encode()
		dsn = u.String()
	} else {
		dsn += " search_path=" + schema + ",public"
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		if _, err := base.Exec(ctx, "DROP SCHEMA "+name+" CASCADE"); err != nil {
			t.Error(err)
		}
		base.Close()
	})
	return ctx, dsn, pool
}

type parcelHTTPFixture struct {
	mu         sync.Mutex
	features   []map[string]interface{}
	failOffset int
	failStatus int
	requests   int
	pidQueries int
	crs        int
}

func feature(oid int, pid, owner string, valid bool, sales int) map[string]interface{} {
	attrs := map[string]interface{}{"OBJECTID": oid, "PID": pid, "CURR_NAME1": owner, "YEARBLT": 0, "FMV_IMPRV": 0, "LAND_CODE": "01"}
	for i := 1; i <= sales; i++ {
		attrs[fmt.Sprintf("s%d_SALESAMT", i)] = i * 100
		attrs[fmt.Sprintf("s%d_SALEDATE", i)] = 1577836800000
	}
	var rings interface{} = []interface{}{}
	if valid {
		x := oid * 2
		rings = [][][]int{{{x, 0}, {x, 1}, {x + 1, 1}, {x + 1, 0}, {x, 0}}}
	}
	return map[string]interface{}{"attributes": attrs, "geometry": map[string]interface{}{"rings": rings}}
}
func (f *parcelHTTPFixture) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests++
	q := r.URL.Query()
	where := q.Get("where")
	var selected []map[string]interface{}
	for _, v := range f.features {
		a := v["attributes"].(map[string]interface{})
		oid := a["OBJECTID"].(int)
		pid, _ := a["PID"].(string)
		match := where == "1=1"
		if strings.HasPrefix(where, "OBJECTID = ") {
			n, _ := strconv.Atoi(strings.TrimPrefix(where, "OBJECTID = "))
			match = oid == n
		} else if where != "1=1" {
			start := strings.Index(where, "PID = '")
			if start < 0 {
				http.Error(w, "unexpected predicate", 400)
				return
			}
			text := where[start+7:]
			var value strings.Builder
			for i := 0; i < len(text); i++ {
				if text[i] == '\'' {
					if i+1 < len(text) && text[i+1] == '\'' {
						value.WriteByte('\'')
						i++
						continue
					}
					break
				}
				value.WriteByte(text[i])
			}
			match = pid == value.String() || (strings.Contains(where, "PID IS NULL") && pid == "" && strconv.Itoa(oid) == value.String())
		}
		if match {
			selected = append(selected, v)
		}
	}
	if q.Get("returnCountOnly") == "true" {
		if where != "1=1" {
			f.pidQueries++
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"count": len(selected)})
		return
	}
	offset, _ := strconv.Atoi(q.Get("resultOffset"))
	limit, _ := strconv.Atoi(q.Get("resultRecordCount"))
	if f.failStatus != 0 && offset == f.failOffset {
		http.Error(w, "fixture failure", f.failStatus)
		return
	}
	if offset > len(selected) {
		offset = len(selected)
	}
	end := min(offset+limit, len(selected))
	if limit == 0 {
		end = len(selected)
	}
	crs := f.crs
	if crs == 0 {
		crs = 3857
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"spatialReference": map[string]interface{}{"wkid": crs}, "features": selected[offset:end]})
}
func (f *parcelHTTPFixture) source(t *testing.T, batch int) *union.GISSource {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(server.Close)
	source, err := union.NewGISSource(union.GISSourceConfig{BaseURL: server.URL, RateLimit: 1000000, BatchSize: batch})
	if err != nil {
		t.Fatal(err)
	}
	return source
}
func runParcelFixture(ctx context.Context, pool *pgxpool.Pool, dsn string, source *union.GISSource, repair bool, limit int) (jobs.Outcome, jobs.Result, error) {
	runner, _ := jobs.NewRunner(pool)
	q := jobs.NewQuarantineStore(pool)
	definition := jobs.Definition{ID: parcelJobID, Run: func(ctx context.Context) (jobs.Result, error) {
		return parceljob.Job{Root: os.Getenv("PARCEL_EVIDENCE_DIR"), DSN: dsn, Source: source, Quarantine: q}.Import(ctx)
	}}
	if repair {
		definition.Kind = jobs.AttemptReprocess
		definition.Run = func(ctx context.Context) (jobs.Result, error) {
			return parceljob.Job{Root: os.Getenv("PARCEL_EVIDENCE_DIR"), DSN: dsn, Source: source, Quarantine: q}.Reprocess(ctx, limit)
		}
	}
	return runner.Run(ctx, definition)
}
func parcelSnapshot(t *testing.T, ctx context.Context, pool *pgxpool.Pool) []string {
	t.Helper()
	rows, err := pool.Query(ctx, `SELECT ((to_jsonb(p)-'updated_at') || jsonb_build_object('shape',encode(ST_AsEWKB(p.shape),'hex'),'sales',
 COALESCE((SELECT jsonb_agg(to_jsonb(s)-'updated_at' ORDER BY sequence) FROM parcel_sales s WHERE s.parcel_id=p.id),'[]'::jsonb)))::text FROM parcels p ORDER BY source_parcel_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var values []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			t.Fatal(err)
		}
		values = append(values, v)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return values
}
func countRows(t *testing.T, ctx context.Context, pool *pgxpool.Pool, table string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestParcelQuarantineWholePIDAcrossPages(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		for _, existing := range []bool{false, true} {
			t.Run(fmt.Sprintf("reverse=%v/existing=%v", reverse, existing), func(t *testing.T) {
				ctx, dsn, pool := parcelFixtureDB(t)
				fixture := &parcelHTTPFixture{features: []map[string]interface{}{feature(1, "A", "original", true, 3)}}
				source := fixture.source(t, 2)
				var before []string
				if existing {
					if outcome, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err != nil || outcome != jobs.OutcomeSuccess {
						t.Fatal(outcome, err)
					}
					before = parcelSnapshot(t, ctx, pool)
				}
				good, bad := feature(1, "A", "must not publish", true, 0), feature(4, "A", "invalid", false, 2)
				if reverse {
					good, bad = feature(1, "A", "invalid", false, 2), feature(4, "A", "must not publish", true, 0)
				}
				fixture.features = []map[string]interface{}{good, feature(2, "B", "first", true, 3), feature(3, "B", "last", true, 1), bad}
				for range 2 {
					outcome, result, err := runParcelFixture(ctx, pool, dsn, source, false, 0)
					if err != nil || outcome != jobs.OutcomeSuccessWithQuarantine || result.Processed != 2 || result.Quarantined != 1 {
						t.Fatalf("outcome=%s result=%+v err=%v", outcome, result, err)
					}
				}
				after := parcelSnapshot(t, ctx, pool)
				if existing {
					if len(after) != 2 || after[0] != before[0] {
						t.Fatalf("rejected existing A changed\nbefore=%v\nafter=%v", before, after)
					}
				} else if len(after) != 1 {
					t.Fatalf("new rejected A published: %v", after)
				}
				var owner string
				var area float64
				var sales int
				if err := pool.QueryRow(ctx, `SELECT current_name_1,ST_Area(shape),(SELECT count(*) FROM parcel_sales s WHERE s.parcel_id=p.id) FROM parcels p WHERE source_parcel_id='B'`).Scan(&owner, &area, &sales); err != nil {
					t.Fatal(err)
				}
				if owner != "last" || area != 2 || sales != 1 {
					t.Fatalf("B owner=%s area=%v sales=%d", owner, area, sales)
				}
				var occurrences int
				var pending bool
				if err := pool.QueryRow(ctx, `SELECT occurrence_count,reprocessed_at IS NULL FROM quarantined_records`).Scan(&occurrences, &pending); err != nil {
					t.Fatal(err)
				}
				if occurrences != 2 || !pending || countRows(t, ctx, pool, "quarantined_records") != 1 {
					t.Fatalf("occurrences=%d pending=%v", occurrences, pending)
				}
			})
		}
	}
}

func TestParcelQuarantineSurvivesFailureAndStorageFailureRollsBack(t *testing.T) {
	for _, kind := range []string{"http", "sql", "dissolve", "quarantine"} {
		t.Run(kind, func(t *testing.T) {
			ctx, dsn, pool := parcelFixtureDB(t)
			fixture := &parcelHTTPFixture{features: []map[string]interface{}{feature(1, "A", "bad", false, 1), feature(2, "B", "good", true, 1), feature(3, "C", "good", true, 1)}}
			source := fixture.source(t, 2)
			switch kind {
			case "http":
				fixture.failOffset = 2
				fixture.failStatus = 500
			case "sql":
				if _, err := pool.Exec(ctx, `ALTER TABLE parcels ADD CHECK(source_parcel_id <> 'B')`); err != nil {
					t.Fatal(err)
				}
			case "dissolve":
				if _, err := pool.Exec(ctx, `ALTER TABLE parcels ADD CHECK(ST_GeometryType(shape) <> 'ST_MultiPolygon')`); err != nil {
					t.Fatal(err)
				}
			case "quarantine":
				if _, err := pool.Exec(ctx, `ALTER TABLE quarantined_records ADD CHECK(reason <> 'invalid geometry')`); err != nil {
					t.Fatal(err)
				}
			}
			outcome, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0)
			if err == nil || outcome != jobs.OutcomeFailed {
				t.Fatalf("outcome=%s error=%v", outcome, err)
			}
			if countRows(t, ctx, pool, "parcels") != 0 || countRows(t, ctx, pool, "parcel_sales") != 0 {
				t.Fatal("failed scan published partial rows")
			}
			want := 1
			if kind == "http" {
				want = 1
			}
			if kind == "quarantine" {
				want = 0
			}
			if countRows(t, ctx, pool, "quarantined_records") != want {
				t.Fatal("wrong surviving evidence")
			}
		})
	}
}

func TestParcelReprocessCurrentMultipartAndRecurrence(t *testing.T) {
	ctx, dsn, pool := parcelFixtureDB(t)
	fixture := &parcelHTTPFixture{features: []map[string]interface{}{feature(1, "A", "stale payload", false, 3), feature(2, "B", "B", true, 1), feature(3, "A", "also invalid", false, 3)}}
	source := fixture.source(t, 2)
	if outcome, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err != nil || outcome != jobs.OutcomeSuccessWithQuarantine {
		t.Fatal(outcome, err)
	}
	fixture.features = []map[string]interface{}{feature(1, "A", "current first", true, 3), feature(2, "B", "B", true, 1), feature(3, "A", "current last", true, 1)}
	for range 2 {
		outcome, result, err := runParcelFixture(ctx, pool, dsn, source, true, 100)
		if err != nil || outcome != jobs.OutcomeSuccess {
			t.Fatal(outcome, err)
		}
		if result.Processed != 0 && result.Processed != 2 {
			t.Fatalf("result=%+v", result)
		}
	}
	var owner string
	var area float64
	var sales int
	if err := pool.QueryRow(ctx, `SELECT current_name_1,ST_Area(shape),(SELECT count(*) FROM parcel_sales s WHERE s.parcel_id=p.id) FROM parcels p WHERE source_parcel_id='A'`).Scan(&owner, &area, &sales); err != nil {
		t.Fatal(err)
	}
	if owner != "current last" || area != 2 || sales != 1 {
		t.Fatalf("owner=%s area=%v sales=%d", owner, area, sales)
	}
	if fixture.pidQueries != 1 {
		t.Fatalf("PID fetched %d times, want coalesced single fetch", fixture.pidQueries)
	}
	if countRows(t, ctx, pool, "quarantined_records WHERE reprocessed_at IS NULL") != 0 {
		t.Fatal("recovered evidence still pending")
	}
	if countRows(t, ctx, pool, "job_attempts WHERE attempt_kind='reprocess' AND outcome='success'") != 2 {
		t.Fatal("repair attempt kind lost")
	}
	before := parcelSnapshot(t, ctx, pool)
	fixture.features[0] = feature(1, "A", "recurrence evidence", false, 0)
	if outcome, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err != nil || outcome != jobs.OutcomeSuccessWithQuarantine {
		t.Fatal(outcome, err)
	}
	after := parcelSnapshot(t, ctx, pool)
	if after[0] != before[0] {
		t.Fatal("recurrence changed existing A")
	}
	var occurrences int
	var current bool
	var reference, identity string
	if err := pool.QueryRow(ctx, `SELECT occurrence_count,reprocessed_at IS NULL,payload_reference,source_parcel_id FROM quarantined_records WHERE raw_record_key='1'`).Scan(&occurrences, &current, &reference, &identity); err != nil {
		t.Fatal(err)
	}
	path, digestIndex, valid := strings.Cut(reference, "@")
	if !valid || !strings.HasSuffix(digestIndex, "#0") {
		t.Fatalf("recurrence reference %q lacks exact feature identity", reference)
	}
	body, err := os.ReadFile(filepath.Join(os.Getenv("PARCEL_EVIDENCE_DIR"), path))
	if err != nil || !strings.HasPrefix(digestIndex, fmt.Sprintf("%x", sha256.Sum256(body))) || !strings.Contains(string(body), "recurrence evidence") {
		t.Fatalf("recurrence does not reference verified current bytes: %q %v", reference, err)
	}
	if occurrences != 2 || !current || identity != "A" {
		t.Fatalf("occurrences=%d current=%v identity=%q", occurrences, current, identity)
	}
	fixture.features[0] = feature(1, "A", "recovered again", true, 0)
	if _, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err != nil {
		t.Fatal(err)
	}
	if countRows(t, ctx, pool, "quarantined_records WHERE reprocessed_at IS NULL") != 0 {
		t.Fatal("full scan failed to resolve recovery")
	}
}

func TestParcelReprocessRefusesChangedIdentityAndIncompleteParts(t *testing.T) {
	for _, kind := range []string{"missing", "changed", "http", "bad again", "resolution"} {
		t.Run(kind, func(t *testing.T) {
			ctx, dsn, pool := parcelFixtureDB(t)
			fixture := &parcelHTTPFixture{features: []map[string]interface{}{feature(1, "A", "bad", false, 1), feature(2, "B", "B", true, 1)}}
			source := fixture.source(t, 2)
			if _, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err != nil {
				t.Fatal(err)
			}
			before := parcelSnapshot(t, ctx, pool)
			fixture.features[0] = feature(1, "A", "current", true, 1)
			switch kind {
			case "missing":
				fixture.features = fixture.features[1:]
			case "changed":
				fixture.features[0] = feature(1, "Z", "changed", true, 1)
			case "http":
				fixture.failStatus = 503
			case "bad again":
				fixture.features[0] = feature(1, "A", "still bad", false, 1)
			case "resolution":
				if _, err := pool.Exec(ctx, `ALTER TABLE quarantined_records ADD CONSTRAINT fixture_resolution_failure CHECK(reprocessed_at IS NULL)`); err != nil {
					t.Fatal(err)
				}
			}
			outcome, _, err := runParcelFixture(ctx, pool, dsn, source, true, 1)
			if kind == "bad again" {
				if err != nil || outcome != jobs.OutcomeSuccessWithQuarantine {
					t.Fatal(outcome, err)
				}
			} else if err == nil || outcome != jobs.OutcomeFailed {
				t.Fatal(outcome, err)
			}
			if !reflect.DeepEqual(before, parcelSnapshot(t, ctx, pool)) {
				t.Fatal("unsafe repair changed parcels")
			}
			if countRows(t, ctx, pool, "quarantined_records WHERE reprocessed_at IS NULL") != 1 {
				t.Fatal("unsafe repair resolved evidence")
			}
			if kind == "resolution" {
				if _, err := pool.Exec(ctx, `ALTER TABLE quarantined_records DROP CONSTRAINT fixture_resolution_failure`); err != nil {
					t.Fatal(err)
				}
				if outcome, _, err := runParcelFixture(ctx, pool, dsn, source, true, 1); err != nil || outcome != jobs.OutcomeSuccess {
					t.Fatal(outcome, err)
				}
				if countRows(t, ctx, pool, "parcels") != 2 || countRows(t, ctx, pool, "quarantined_records WHERE reprocessed_at IS NULL") != 0 {
					t.Fatal("atomic rollback was not safely retryable")
				}
			}

		})
	}
}

func TestParcelReprocessLimitAndSharedExclusion(t *testing.T) {
	ctx, dsn, pool := parcelFixtureDB(t)
	fixture := &parcelHTTPFixture{}
	for i := 1; i <= 101; i++ {
		fixture.features = append(fixture.features, feature(i, fmt.Sprintf("P%d", i), "bad", false, 0))
	}
	source := fixture.source(t, 2000)
	if _, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err != nil {
		t.Fatal(err)
	}
	for i := range fixture.features {
		fixture.features[i] = feature(i+1, fmt.Sprintf("P%d", i+1), "current", true, 0)
	}
	if outcome, result, err := runParcelFixture(ctx, pool, dsn, source, true, 1); err != nil || outcome != jobs.OutcomeSuccess || result.Processed != 1 {
		t.Fatal(outcome, result, err)
	}
	if countRows(t, ctx, pool, "quarantined_records WHERE reprocessed_at IS NULL") != 100 {
		t.Fatal("repair did not honor selection limit")
	}
	before := parcelSnapshot(t, ctx, pool)
	if outcome, _, err := runParcelFixture(ctx, pool, dsn, source, true, 100); err == nil || outcome != jobs.OutcomeFailed {
		t.Fatal(outcome, err)
	}
	if !reflect.DeepEqual(before, parcelSnapshot(t, ctx, pool)) || countRows(t, ctx, pool, "quarantined_records WHERE reprocessed_at IS NULL") != 100 {
		t.Fatal("request-budget failure published partial repair")
	}
	runner, _ := jobs.NewRunner(pool)
	_, _, err := runner.Run(ctx, jobs.Definition{ID: parcelJobID, Run: func(runCtx context.Context) (jobs.Result, error) {
		outcome, _, err := runParcelFixture(runCtx, pool, dsn, source, true, 1)
		if err != nil || outcome != jobs.OutcomeSkippedOverlap {
			t.Errorf("overlap=%s %v", outcome, err)
		}
		return jobs.Result{}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	var kind string
	if err := pool.QueryRow(ctx, `SELECT attempt_kind FROM job_attempts WHERE outcome='skipped_overlap'`).Scan(&kind); err != nil || kind != "reprocess" {
		t.Fatal(kind, err)
	}
}

func TestParcelAttemptKindMigrationRejectsLossyRollback(t *testing.T) {
	ctx, _, pool := parcelFixtureDB(t)
	if _, err := pool.Exec(ctx, `INSERT INTO job_attempts(job_id,outcome,attempt_kind) VALUES('fixture.repair','success','reprocess')`); err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("db/migrations/000016_job_attempt_kind.sql")
	if err != nil {
		t.Fatal(err)
	}
	down := strings.Split(string(migration), "-- +goose Down")[1]
	if _, err := pool.Exec(ctx, down); err == nil {
		t.Fatal("rollback discarded repair identity")
	}
	if countRows(t, ctx, pool, "job_attempts WHERE attempt_kind='reprocess'") != 1 {
		t.Fatal("failed rollback altered repair history")
	}
}

func TestParcelReprocessResolvesOnlySelectedEvidence(t *testing.T) {
	for _, sameOID := range []bool{false, true} {
		t.Run(fmt.Sprintf("sameOID=%v", sameOID), func(t *testing.T) {
			ctx, dsn, pool := parcelFixtureDB(t)
			fixture := &parcelHTTPFixture{}
			if sameOID {
				fixture.features = []map[string]interface{}{feature(1, "A", "bad", false, 0)}
			} else {
				for i := 1; i <= 101; i++ {
					fixture.features = append(fixture.features, feature(i, "A", "bad", false, 0))
				}
			}
			source := fixture.source(t, 2000)
			if outcome, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err != nil || outcome != jobs.OutcomeSuccessWithQuarantine {
				t.Fatal(outcome, err)
			}
			if sameOID {
				fixture.features[0] = feature(1, "A", "bad year", true, 0)
				fixture.features[0]["attributes"].(map[string]interface{})["YEARBLT"] = 1200
				if outcome, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err != nil || outcome != jobs.OutcomeSuccessWithQuarantine {
					t.Fatal(outcome, err)
				}
			}
			var selectedID string
			if err := pool.QueryRow(ctx, `SELECT id::text FROM quarantined_records ORDER BY last_seen_at,id LIMIT 1`).Scan(&selectedID); err != nil {
				t.Fatal(err)
			}
			pending := countRows(t, ctx, pool, "quarantined_records WHERE reprocessed_at IS NULL")
			for i := range fixture.features {
				fixture.features[i] = feature(i+1, "A", fmt.Sprintf("current %d", i+1), true, 1)
			}
			outcome, result, err := runParcelFixture(ctx, pool, dsn, source, true, 1)
			remaining := countRows(t, ctx, pool, "quarantined_records WHERE reprocessed_at IS NULL")
			if err != nil || outcome != jobs.OutcomeSuccess || result.Processed != len(fixture.features) || remaining != pending-1 {
				t.Fatalf("limit=1 outcome=%s result=%+v pending_before=%d pending_after=%d err=%v", outcome, result, pending, remaining, err)
			}
			t.Logf("limit=1 outcome=%s processed=%d pending_before=%d pending_after=%d", outcome, result.Processed, pending, remaining)
			var resolved bool
			if err := pool.QueryRow(ctx, `SELECT reprocessed_at IS NOT NULL FROM quarantined_records WHERE id=$1`, selectedID).Scan(&resolved); err != nil || !resolved {
				t.Fatalf("selected UUID resolved=%v error=%v", resolved, err)
			}
			var area float64
			var owner string
			if err := pool.QueryRow(ctx, `SELECT ST_Area(shape),current_name_1 FROM parcels WHERE source_parcel_id='A'`).Scan(&area, &owner); err != nil {
				t.Fatal(err)
			}
			if area != float64(len(fixture.features)) || owner != fmt.Sprintf("current %d", len(fixture.features)) || countRows(t, ctx, pool, "parcel_sales") != 1 {
				t.Fatalf("incomplete recovery area=%v owner=%s", area, owner)
			}
			if outcome, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err != nil || outcome != jobs.OutcomeSuccess || countRows(t, ctx, pool, "quarantined_records WHERE reprocessed_at IS NULL") != 0 {
				t.Fatalf("full scan recovery outcome=%s err=%v", outcome, err)
			}
		})
	}
}

func TestParcelNumericStorageBoundaries(t *testing.T) {
	for _, tc := range []struct {
		field, column string
		value         interface{}
		stored        string
	}{
		{"SHAPE.STArea()", "source_area", 1e14, ""},
		{"SHAPE.STLength()", "source_length", 1e14, ""},
		{"SHAPE.STArea()", "source_area", "99999999999999.999999", ""},
		{"SHAPE.STLength()", "source_length", "99999999999999.999999", ""},
		{"mapped_acres", "mapped_acres", "99999999.9999995", ""},
		{"FMV_LAND", "valuation_land", "999999999999.995", ""},
		{"s1_SALESAMT", "amount", "999999999999.995", ""},
		{"VAL_SQFT", "valued_sqft", "99999999999999.995", ""},
		{"SHAPE.STArea()", "source_area", "NaN", ""},
		{"SHAPE.STLength()", "source_length", "+Inf", ""},
		{"SHAPE.STLength()", "source_length", "-Inf", ""},
		{"SHAPE.STArea()", "source_area", -1, ""},
		{"mapped_acres", "mapped_acres", "99999999.99999948", "99999999.999999"},
		{"FMV_LAND", "valuation_land", "999999999999.9949", "999999999999.99"},
		{"s1_SALESAMT", "amount", "999999999999.9949", "999999999999.99"},
		{"VAL_SQFT", "valued_sqft", "99999999999999.98", "99999999999999.98"},
		{"SHAPE.STArea()", "source_area", "99999999999999.98", "99999999999999.980000"},
		{"SHAPE.STLength()", "source_length", "99999999999999.98", "99999999999999.980000"},
		{"SHAPE.STArea()", "source_area", 0, "0.000000"},
		{"YEARBLT", "year_built", 2200, "2200"},
		{"parcel_year", "parcel_year", 2200, "2200"},
	} {
		t.Run(fmt.Sprintf("%s=%v", tc.field, tc.value), func(t *testing.T) {
			ctx, dsn, pool := parcelFixtureDB(t)
			a := feature(1, "A", "A", true, 1)
			a["attributes"].(map[string]interface{})[tc.field] = tc.value
			fixture := &parcelHTTPFixture{features: []map[string]interface{}{a, feature(2, "B", "B", true, 1)}}
			outcome, result, err := runParcelFixture(ctx, pool, dsn, fixture.source(t, 2), false, 0)
			wantOutcome, wantProcessed, wantQuarantined := jobs.OutcomeSuccessWithQuarantine, 1, 1
			if tc.stored != "" {
				wantOutcome, wantProcessed, wantQuarantined = jobs.OutcomeSuccess, 2, 0
			}
			if err != nil || outcome != wantOutcome || result.Processed != wantProcessed || result.Quarantined != wantQuarantined || countRows(t, ctx, pool, "parcels WHERE source_parcel_id='B'") != 1 {
				t.Fatalf("outcome=%s result=%+v err=%v", outcome, result, err)
			}
			if tc.stored == "" {
				if countRows(t, ctx, pool, "parcels WHERE source_parcel_id='A'") != 0 || countRows(t, ctx, pool, "quarantined_records WHERE reprocessed_at IS NULL") != 1 {
					t.Fatal("overflow did not quarantine A")
				}
				return
			}
			query := "SELECT " + tc.column + "::text FROM parcels WHERE source_parcel_id='A'"
			if tc.column == "amount" {
				query = "SELECT amount::text FROM parcel_sales s JOIN parcels p ON p.id=s.parcel_id WHERE source_parcel_id='A'"
			}
			var stored string
			if err := pool.QueryRow(ctx, query).Scan(&stored); err != nil || stored != tc.stored {
				t.Fatalf("stored=%s want=%s err=%v", stored, tc.stored, err)
			}
		})
	}
}

func TestParcelContradictoryObjectIdentityFailsAtomically(t *testing.T) {
	for _, batch := range []int{2, 4} {
		for _, validity := range []string{"both valid", "bad first", "bad last"} {
			t.Run(fmt.Sprintf("batch=%d/%s", batch, validity), func(t *testing.T) {
				ctx, dsn, pool := parcelFixtureDB(t)
				fixture := &parcelHTTPFixture{features: []map[string]interface{}{feature(1, "A", "original", true, 3), feature(2, "B", "original", true, 3)}}
				source := fixture.source(t, batch)
				if outcome, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err != nil || outcome != jobs.OutcomeSuccess {
					t.Fatal(outcome, err)
				}
				before := parcelSnapshot(t, ctx, pool)
				fixture.features = []map[string]interface{}{
					feature(3, "C", "durable evidence", false, 0),
					feature(1, "A", "changed A", validity != "bad first", 1),
					feature(2, "unrelated", "must roll back", true, 1),
					feature(1, "B", "changed B", validity != "bad last", 1),
				}
				outcome, result, err := runParcelFixture(ctx, pool, dsn, source, false, 0)
				if err == nil || outcome != jobs.OutcomeFailed || result.Processed != 0 {
					t.Fatalf("ambiguous identity outcome=%s result=%+v err=%v", outcome, result, err)
				}
				if !reflect.DeepEqual(before, parcelSnapshot(t, ctx, pool)) {
					t.Fatal("contradictory identity published parcel or sale changes")
				}
				if countRows(t, ctx, pool, "quarantined_records WHERE raw_record_key='3' AND reprocessed_at IS NULL") != 1 {
					t.Fatal("rollback erased quarantine evidence")
				}
			})
		}
	}
}

func TestParcelRepeatedObjectSamePIDPreservesLastPart(t *testing.T) {
	ctx, dsn, pool := parcelFixtureDB(t)
	fixture := &parcelHTTPFixture{features: []map[string]interface{}{feature(1, "A", "first", true, 3), feature(2, "B", "B", true, 0), feature(1, "A", "last", true, 1)}}
	outcome, result, err := runParcelFixture(ctx, pool, dsn, fixture.source(t, 2), false, 0)
	if err != nil || outcome != jobs.OutcomeSuccess || result.Processed != 3 {
		t.Fatal(outcome, result, err)
	}
	var owner string
	var area float64
	if err := pool.QueryRow(ctx, `SELECT current_name_1,ST_Area(shape) FROM parcels WHERE source_parcel_id='A'`).Scan(&owner, &area); err != nil || owner != "last" || area != 1 || countRows(t, ctx, pool, "parcel_sales") != 1 {
		t.Fatalf("owner=%s area=%v err=%v", owner, area, err)
	}
}

func TestParcelReprocessRetainsChangedEvidenceToken(t *testing.T) {
	ctx, dsn, pool := parcelFixtureDB(t)
	fixture := &parcelHTTPFixture{features: []map[string]interface{}{feature(1, "A", "bad", false, 0)}}
	if outcome, _, err := runParcelFixture(ctx, pool, dsn, fixture.source(t, 2), false, 0); err != nil || outcome != jobs.OutcomeSuccessWithQuarantine {
		t.Fatal(outcome, err)
	}
	fixture.features[0] = feature(1, "A", "current", true, 1)
	var once sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		once.Do(func() {
			if _, err := pool.Exec(ctx, `UPDATE quarantined_records SET last_seen_at=last_seen_at+interval '1 microsecond'`); err != nil {
				t.Error(err)
			}
		})
		fixture.serve(w, r)
	}))
	defer server.Close()
	source, err := union.NewGISSource(union.GISSourceConfig{BaseURL: server.URL, RateLimit: 1000000, BatchSize: 2})
	if err != nil {
		t.Fatal(err)
	}
	if outcome, result, err := runParcelFixture(ctx, pool, dsn, source, true, 1); err != nil || outcome != jobs.OutcomeSuccess || result.Processed != 1 {
		t.Fatal(outcome, result, err)
	}
	if countRows(t, ctx, pool, "parcels WHERE source_parcel_id='A'") != 1 || countRows(t, ctx, pool, "quarantined_records WHERE reprocessed_at IS NULL") != 1 {
		t.Fatal("stale selection resolved changed evidence or prevented parcel recovery")
	}
	var olderThanAttempt bool
	if err := pool.QueryRow(ctx, `SELECT q.last_seen_at <= a.started_at FROM quarantined_records q CROSS JOIN job_attempts a WHERE a.attempt_kind='reprocess'`).Scan(&olderThanAttempt); err != nil || !olderThanAttempt {
		t.Fatalf("changed token did not isolate selection check: older=%v err=%v", olderThanAttempt, err)
	}
	if outcome, _, err := runParcelFixture(ctx, pool, dsn, source, true, 1); err != nil || outcome != jobs.OutcomeSuccess || countRows(t, ctx, pool, "quarantined_records WHERE reprocessed_at IS NULL") != 0 {
		t.Fatalf("fresh token retry outcome=%s err=%v", outcome, err)
	}
}
