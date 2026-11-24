//go:build postgres

package vacancyjob

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/JustinMonty20/landman/internal/connector/union"
	"github.com/JustinMonty20/landman/internal/ingest"
	"github.com/JustinMonty20/landman/internal/jobs"
	"github.com/JustinMonty20/landman/internal/taxartifact"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestAssessmentCaptureReplayAndFailures(t *testing.T) {
	ctx := context.Background()
	dsn := os.Getenv("API_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("API_TEST_DATABASE_URL missing")
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var name string
	if err = pool.QueryRow(ctx, `SELECT current_database()`).Scan(&name); err != nil || !strings.HasPrefix(name, "landman_api_test_") {
		t.Fatal("refusing non-disposable database")
	}
	if _, err = pool.Exec(ctx, `TRUNCATE vacancy_live_assessments,vacancy_live_structures,vacancy_live_generations CASCADE`); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `DELETE FROM vacancy_latest_observations WHERE source_name='union_assessed_value_0'`); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `DELETE FROM ingest_replay_attempts WHERE capture_id IN (SELECT id FROM ingest_captures WHERE source_name='union_assessed_value_0')`); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `DELETE FROM ingest_captures WHERE source_name='union_assessed_value_0'`); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err = os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	artifacts, err := taxartifact.NewLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	runner, err := jobs.NewRunner(pool)
	if err != nil {
		t.Fatal(err)
	}
	version := 0
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Query().Get("returnCountOnly") == "true" {
			fmt.Fprint(w, `{"count":2}`)
			return
		}
		switch version {
		case 0:
			fmt.Fprint(w, `{"features":[{"attributes":{"OBJECTID":1,"parcel_number":"001","parcel_year":2027,"assess_category":"AV","market_building":0}},{"attributes":{"OBJECTID":1,"parcel_number":"001","parcel_year":2027,"assess_category":"AV","market_building":0}}]}`)
		case 1:
			fmt.Fprint(w, `{"features":[{"attributes":{"OBJECTID":1,"parcel_number":"001","parcel_year":2027,"assess_category":"AV","market_building":1}},{"attributes":{"OBJECTID":1,"parcel_number":"001","parcel_year":2027,"assess_category":"PY","market_building":0}}]}`)
		case 3:
			fmt.Fprint(w, `{ "features": [ {"attributes":{"market_building":0,"assess_category":"AV","parcel_year":2027,"parcel_number":"001","OBJECTID":1}}, {"attributes":{"OBJECTID":1,"parcel_number":"001","parcel_year":2027,"assess_category":"AV","market_building":0}} ] }`)
		case 4:
			fmt.Fprint(w, `{malformed`)
		case 5:
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"features":[{"attributes":{"OBJECTID":1,"parcel_number":"001","market_building":0}},{"attributes":{"OBJECTID":1,"parcel_number":"001","market_building":0}}]}`)
		default:
			fmt.Fprint(w, `{"features":[]}`)
		}
	}))
	defer server.Close()
	source, err := union.NewGISSource(union.GISSourceConfig{BaseURL: server.URL + "/server/rest/services/OperationalLayers/MapServer/215/query", RateLimit: 100})
	if err != nil {
		t.Fatal(err)
	}
	job := Job{Source: source, Pool: pool, Artifacts: artifacts, Layer: union.AssessedValueLayer}
	run := func() error {
		_, _, err := runner.Run(ctx, jobs.Definition{ID: AssessedValueID, Run: job.Import})
		return err
	}
	query := func(sql string) int {
		var n int
		if err := pool.QueryRow(ctx, sql).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if err := run(); err != nil {
		t.Fatal(err)
	}
	if query(`SELECT count(*) FROM vacancy_live_generations WHERE validation_note='bounded numeric-range assessment observation; county snapshot, query support, coverage and effective year unverified'`) != 1 {
		t.Fatal("assessment provenance incorrect")
	}
	if query(`SELECT count(*) FROM vacancy_live_assessments`) != 2 {
		t.Fatal("lost duplicate OIDs")
	}
	var older string
	if err := pool.QueryRow(ctx, `SELECT capture_id::text FROM vacancy_live_generations ORDER BY retrieved_at LIMIT 1`).Scan(&older); err != nil {
		t.Fatal(err)
	}
	if err := run(); err != nil {
		t.Fatal(err)
	}
	if query(`SELECT count(*) FROM vacancy_live_generations`) != 2 || query(`SELECT count(*) FROM ingest_captures WHERE source_name='union_assessed_value_0'`) != 2 {
		t.Fatal("repeat did not retain distinct generation and observation")
	}
	version = 3
	if err := run(); err != nil {
		t.Fatal(err)
	}
	if query(`SELECT count(*) FROM vacancy_live_generations`) != 3 {
		t.Fatal("format-only observation did not retain own generation")
	}
	var acceptedAfterEquivalent string
	if err := pool.QueryRow(ctx, `SELECT capture_id::text FROM vacancy_latest_observations WHERE source_name='union_assessed_value_0'`).Scan(&acceptedAfterEquivalent); err != nil {
		t.Fatal(err)
	}
	version = 2
	if err := run(); err == nil {
		t.Fatal("partial capture accepted")
	}
	if query(`SELECT count(*) FROM vacancy_live_generations`) != 3 {
		t.Fatal("failed capture published")
	}
	version = 4
	if err := run(); err == nil {
		t.Fatal("malformed JSON accepted")
	}
	if query(`SELECT count(*) FROM ingest_captures WHERE source_name='union_assessed_value_0' AND jsonb_array_length(convert_from(manifest,'UTF8')::jsonb->'receipts')=3`) < 1 {
		t.Fatal("malformed response not sealed")
	}
	for _, rejected := range []struct {
		version int
		body    string
	}{{4, `{malformed`}, {5, `{"features":[{"attributes":{"OBJECTID":1,"parcel_number":"001","market_building":0}},{"attributes":{"OBJECTID":1,"parcel_number":"001","market_building":0}}]}`}} {
		version = rejected.version
		if rejected.version == 5 && run() == nil {
			t.Fatal("HTTP refusal accepted")
		}
		var manifest []byte
		if err := pool.QueryRow(ctx, `SELECT manifest FROM ingest_captures WHERE source_name='union_assessed_value_0' ORDER BY observed_at DESC LIMIT 1`).Scan(&manifest); err != nil {
			t.Fatal(err)
		}
		var receipt struct {
			Receipts []struct {
				ArtifactKey string `json:"artifact_key"`
				HTTPStatus  int    `json:"http_status"`
			} `json:"receipts"`
		}
		if err := json.Unmarshal(manifest, &receipt); err != nil {
			t.Fatal(err)
		}
		if len(receipt.Receipts) != 3 {
			t.Fatalf("missing failed capture page: %s", manifest)
		}
		body, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(receipt.Receipts[2].ArtifactKey)))
		if err != nil || string(body) != rejected.body {
			t.Fatalf("wrong failed capture bytes: %q %v", body, err)
		}
		if rejected.version == 5 {
			if receipt.Receipts[2].HTTPStatus != 403 {
				t.Fatalf("lost refusal status: %s", manifest)
			}
			var rejectedID string
			if err := pool.QueryRow(ctx, `SELECT id::text FROM ingest_captures WHERE source_name='union_assessed_value_0' ORDER BY observed_at DESC LIMIT 1`).Scan(&rejectedID); err != nil {
				t.Fatal(err)
			}
			before := requests
			_, _, replayErr := runner.Run(ctx, jobs.Definition{ID: AssessedValueID, Kind: jobs.AttemptReplay, Run: func(ctx context.Context) (jobs.Result, error) { return jobs.Result{}, job.Replay(ctx, rejectedID) }})
			if replayErr == nil || requests != before || query(`SELECT count(*) FROM vacancy_live_generations`) != 3 {
				t.Fatalf("rejected response published on replay: %v", replayErr)
			}
		}

	}
	var acceptedBefore string
	if err := pool.QueryRow(ctx, `SELECT capture_id::text FROM vacancy_latest_observations WHERE source_name='union_assessed_value_0'`).Scan(&acceptedBefore); err != nil {
		t.Fatal(err)
	}
	if acceptedBefore == older || acceptedBefore != acceptedAfterEquivalent {
		t.Fatal("equivalent observation missing or failed attempts advanced cursor")
	}
	if _, err := pool.Exec(ctx, `UPDATE vacancy_live_generations SET normalizer_version='prior-normalizer' WHERE capture_id=$1::uuid`, older); err != nil {
		t.Fatal(err)
	}
	requestsBeforeEquivalentReplay := requests
	_, _, err = runner.Run(ctx, jobs.Definition{ID: AssessedValueID, Kind: jobs.AttemptReplay, Run: func(ctx context.Context) (jobs.Result, error) { return jobs.Result{}, job.Replay(ctx, older) }})
	if err != nil || requests != requestsBeforeEquivalentReplay || query(`SELECT count(*) FROM vacancy_live_generations`) != 3 || query(`SELECT count(*) FROM vacancy_live_generations WHERE capture_id=(SELECT capture_id FROM vacancy_latest_observations WHERE source_name='union_assessed_value_0')`) != 1 {
		t.Fatalf("stale replay outranked equivalent observation: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE vacancy_live_generations SET normalizer_version='vacancy-rows-v1' WHERE capture_id=$1::uuid`, older); err != nil {
		t.Fatal(err)
	}
	version = 1
	if err := run(); err != nil {
		t.Fatal(err)
	}
	if query(`SELECT count(*) FROM vacancy_live_generations`) != 4 {
		t.Fatal("changed capture not published")
	}
	version = 0
	if err := run(); err != nil {
		t.Fatal(err)
	}
	if query(`SELECT count(*) FROM vacancy_live_generations`) != 5 {
		t.Fatal("A-B-A reversion was not published")
	}
	if query(`SELECT count(*) FROM vacancy_live_assessments a JOIN vacancy_live_generations g ON a.generation_id=g.id WHERE g.retrieved_at=(SELECT max(retrieved_at) FROM vacancy_live_generations) AND a.market_building=0`) != 2 {
		t.Fatal("reverted assessment rows differ")
	}
	var current string
	if err := pool.QueryRow(ctx, `SELECT capture_id::text FROM vacancy_live_generations ORDER BY retrieved_at DESC LIMIT 1`).Scan(&current); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE vacancy_live_generations SET normalizer_version='prior-normalizer',normalized_sha256='outdated' WHERE capture_id=$1::uuid`, current); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE vacancy_live_assessments SET market_building=999 WHERE generation_id=(SELECT id FROM vacancy_live_generations WHERE capture_id=$1::uuid)`, current); err != nil {
		t.Fatal(err)
	}
	requestsBefore := requests
	_, _, err = runner.Run(ctx, jobs.Definition{ID: AssessedValueID, Kind: jobs.AttemptReplay, Run: func(ctx context.Context) (jobs.Result, error) { return jobs.Result{}, job.Replay(ctx, current) }})
	if err != nil || requests != requestsBefore || query(`SELECT count(*) FROM vacancy_live_generations WHERE normalizer_version='vacancy-rows-v1'`) != 5 || query(`SELECT count(*) FROM vacancy_live_assessments WHERE market_building=999`) != 0 {
		t.Fatalf("correction replay failed: %v", err)
	}
	// Simulate an upgraded retained capture whose old boolean proof was withdrawn.
	// The same actual captured bytes must regain proof without inheriting approval.
	if _, err := pool.Exec(ctx, `UPDATE vacancy_live_generations SET parser_admission_verified=false,parser_manifest_sha256=NULL WHERE capture_id=$1::uuid`, current); err != nil {
		t.Fatal(err)
	}
	_, _, err = runner.Run(ctx, jobs.Definition{ID: AssessedValueID, Kind: jobs.AttemptReplay, Run: func(ctx context.Context) (jobs.Result, error) { return jobs.Result{}, job.Replay(ctx, current) }})
	if err != nil || requests != requestsBefore || query(`SELECT count(*) FROM vacancy_live_generations g JOIN ingest_captures c ON c.id=g.capture_id WHERE g.capture_id='`+current+`'::uuid AND g.parser_admission_verified AND g.parser_manifest_sha256=c.manifest_sha256`) != 1 {
		t.Fatalf("identical replay failed to reissue exact-capture proof: %v", err)
	}
	for _, mutation := range []string{`UPDATE ingest_captures SET manifest=manifest||decode('20','hex') WHERE id=$1::uuid`, `UPDATE ingest_captures SET manifest_sha256=decode(repeat('a',64),'hex') WHERE id=$1::uuid`} {
		if _, err := pool.Exec(ctx, mutation, current); err == nil || !strings.Contains(err.Error(), "immutable") {
			t.Fatalf("capture proof bytes could be rewritten: %v", err)
		}
	}
	_, _, err = runner.Run(ctx, jobs.Definition{ID: AssessedValueID, Kind: jobs.AttemptReplay, Run: func(ctx context.Context) (jobs.Result, error) { return jobs.Result{}, job.Replay(ctx, older) }})
	if err != nil || requests != requestsBefore || query(`SELECT count(*) FROM vacancy_live_generations`) != 5 || query(`SELECT count(*) FROM vacancy_live_assessments WHERE market_building=999`) != 0 {
		t.Fatalf("older replay changed evidence or contacted HTTP: %v", err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE vacancy_live_assessments ADD CONSTRAINT reject_test_building CHECK (market_building <> 1) NOT VALID`); err != nil {
		t.Fatal(err)
	}
	version = 1
	if err := run(); err == nil {
		t.Fatal("expected publication to fail")
	}
	var retryID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM ingest_captures WHERE source_name='union_assessed_value_0' ORDER BY observed_at DESC LIMIT 1`).Scan(&retryID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE vacancy_live_assessments DROP CONSTRAINT reject_test_building`); err != nil {
		t.Fatal(err)
	}
	requestsBefore = requests
	_, _, err = runner.Run(ctx, jobs.Definition{ID: AssessedValueID, Kind: jobs.AttemptReplay, Run: func(ctx context.Context) (jobs.Result, error) { return jobs.Result{}, job.Replay(ctx, retryID) }})
	if err != nil || requests != requestsBefore || query(`SELECT count(*) FROM vacancy_live_generations`) != 6 {
		t.Fatalf("offline retry did not publish: %v", err)
	}
	if query(`SELECT count(*) FROM vacancy_live_generations WHERE capture_id='`+retryID+`'::uuid AND validation_note='bounded numeric-range assessment observation; county snapshot, query support, coverage and effective year unverified'`) != 1 {
		t.Fatal("offline retry lost partition provenance")
	}
	var acceptedAfterRetry string
	if err := pool.QueryRow(ctx, `SELECT capture_id::text FROM vacancy_latest_observations WHERE source_name='union_assessed_value_0'`).Scan(&acceptedAfterRetry); err != nil {
		t.Fatal(err)
	}
	if acceptedAfterRetry != current {
		t.Fatal("offline retry advanced the accepted live observation")
	}
	if _, err := pool.Exec(ctx, `UPDATE vacancy_live_generations SET normalizer_version='prior-normalizer' WHERE capture_id=$1::uuid`, current); err != nil {
		t.Fatal(err)
	}
	_, _, err = runner.Run(ctx, jobs.Definition{ID: AssessedValueID, Kind: jobs.AttemptReplay, Run: func(ctx context.Context) (jobs.Result, error) { return jobs.Result{}, job.Replay(ctx, current) }})
	if err != nil || requests != requestsBefore || query(`SELECT count(*) FROM vacancy_live_generations WHERE normalizer_version='prior-normalizer'`) != 1 {
		t.Fatalf("old live capture changed after a newer offline publication: %v", err)
	}
	var manifest []byte
	if err := pool.QueryRow(ctx, `SELECT manifest FROM ingest_captures WHERE id=$1::uuid`, older).Scan(&manifest); err != nil {
		t.Fatal(err)
	}
	var receipt struct {
		Receipts []struct {
			ArtifactKey string `json:"artifact_key"`
		} `json:"receipts"`
	}
	if err := json.Unmarshal(manifest, &receipt); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(receipt.Receipts[2].ArtifactKey)), []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	_, _, err = runner.Run(ctx, jobs.Definition{ID: AssessedValueID, Kind: jobs.AttemptReplay, Run: func(ctx context.Context) (jobs.Result, error) { return jobs.Result{}, job.Replay(ctx, older) }})
	if err == nil || query(`SELECT count(*) FROM vacancy_live_generations`) != 6 || requests != requestsBefore {
		t.Fatalf("corrupt replay accepted or contacted HTTP: %v", err)
	}
}

func TestStructurePagesAtomicAndOfflineReplay(t *testing.T) {
	ctx := context.Background()
	dsn := os.Getenv("API_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("API_TEST_DATABASE_URL missing")
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var db string
	if err = pool.QueryRow(ctx, `SELECT current_database()`).Scan(&db); err != nil || !strings.HasPrefix(db, "landman_api_test_") {
		t.Fatal("refusing non-disposable database")
	}
	for _, sql := range []string{`TRUNCATE vacancy_live_structures,vacancy_live_generations CASCADE`, `DELETE FROM vacancy_latest_observations WHERE source_name='union_structures_171'`, `DELETE FROM ingest_replay_attempts WHERE capture_id IN (SELECT id FROM ingest_captures WHERE source_name='union_structures_171')`, `DELETE FROM ingest_captures WHERE source_name='union_structures_171'`} {
		if _, err = pool.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	dir := t.TempDir()
	if err = os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	artifacts, err := taxartifact.NewLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	runner, err := jobs.NewRunner(pool)
	if err != nil {
		t.Fatal(err)
	}
	failed := false
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		q := r.URL.Query()
		switch {
		case q.Get("returnCountOnly") == "true":
			fmt.Fprint(w, `{"count":2001}`)
		case q.Get("returnIdsOnly") == "true":
			fmt.Fprint(w, `{"objectIdFieldName":"OBJECTID","objectIds":[`)
			for i := 1; i <= 2001; i++ {
				if i > 1 {
					fmt.Fprint(w, ",")
				}
				fmt.Fprint(w, i)
			}
			fmt.Fprint(w, `]}`)
		default:
			offset, _ := strconv.Atoi(q.Get("resultOffset"))
			if failed && offset == 2000 {
				fmt.Fprint(w, `{"features":[],"exceededTransferLimit":true}`)
				return
			}
			fmt.Fprint(w, `{"spatialReference":{"wkid":3857},"features":[`)
			end := offset + 2000
			if end > 2001 {
				end = 2001
			}
			for i := offset; i < end; i++ {
				if i > offset {
					fmt.Fprint(w, ",")
				}
				fmt.Fprintf(w, `{"attributes":{"OBJECTID":%d},"geometry":{"rings":[[[0,0],[1,0],[0,0]]]}}`, i+1)
			}
			if offset == 0 {
				fmt.Fprint(w, `],"exceededTransferLimit":true}`)
			} else {
				fmt.Fprint(w, `]}`)
			}
		}
	}))
	defer server.Close()
	source, err := union.NewGISSource(union.GISSourceConfig{BaseURL: server.URL + "/server/rest/services/OperationalLayers/MapServer/215/query", RateLimit: 1000})
	if err != nil {
		t.Fatal(err)
	}
	job := Job{Source: source, Pool: pool, Artifacts: artifacts, Layer: union.StructuresLayer}
	run := func() error { _, _, e := runner.Run(ctx, jobs.Definition{ID: StructuresID, Run: job.Import}); return e }
	count := func(sql string) int {
		var n int
		if e := pool.QueryRow(ctx, sql).Scan(&n); e != nil {
			t.Fatal(e)
		}
		return n
	}
	if err = run(); err != nil {
		t.Fatal(err)
	}
	if count(`SELECT count(*) FROM vacancy_live_structures`) != 2001 || count(`SELECT count(*) FROM vacancy_live_generations WHERE validation_note='paged source-ID-reconciled structures observation; county snapshot and coverage unverified'`) != 1 {
		t.Fatal("missing structure page or paged provenance")
	}
	var good string
	if err = pool.QueryRow(ctx, `SELECT capture_id::text FROM vacancy_live_generations WHERE source_name='union_structures_171'`).Scan(&good); err != nil {
		t.Fatal(err)
	}
	if err = run(); err != nil || count(`SELECT count(*) FROM vacancy_live_generations WHERE source_name='union_structures_171'`) != 2 {
		t.Fatalf("repeat did not retain new generation: %v", err)
	}
	failed = true
	if err = run(); err == nil {
		t.Fatal("accepted failed later page")
	}
	if count(`SELECT count(*) FROM vacancy_live_structures`) != 4002 || count(`SELECT count(*) FROM vacancy_live_generations WHERE source_name='union_structures_171'`) != 2 {
		t.Fatal("failed import partially published")
	}
	var rejected string
	if err = pool.QueryRow(ctx, `SELECT id::text FROM ingest_captures WHERE source_name='union_structures_171' ORDER BY observed_at DESC LIMIT 1`).Scan(&rejected); err != nil {
		t.Fatal(err)
	}
	before := requests
	_, _, err = runner.Run(ctx, jobs.Definition{ID: StructuresID, Kind: jobs.AttemptReplay, Run: func(ctx context.Context) (jobs.Result, error) { return jobs.Result{}, job.Replay(ctx, rejected) }})
	if err == nil || requests != before || count(`SELECT count(*) FROM vacancy_live_structures`) != 4002 {
		t.Fatalf("failed later page replay published: %v", err)
	}
	_, _, err = runner.Run(ctx, jobs.Definition{ID: StructuresID, Kind: jobs.AttemptReplay, Run: func(ctx context.Context) (jobs.Result, error) { return jobs.Result{}, job.Replay(ctx, good) }})
	if err != nil || requests != before || count(`SELECT count(*) FROM vacancy_live_structures`) != 4002 {
		t.Fatalf("offline replay changed publication: %v", err)
	}
}

func TestLegacyStructureOfflineReplayProvenance(t *testing.T) {
	dsn := os.Getenv("API_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("API_TEST_DATABASE_URL missing")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var db string
	if err = pool.QueryRow(ctx, `SELECT current_database()`).Scan(&db); err != nil || !strings.HasPrefix(db, "landman_api_test_") {
		t.Fatal("refusing non-disposable database")
	}
	for _, stmt := range []string{`TRUNCATE vacancy_live_structures,vacancy_live_generations CASCADE`, `DELETE FROM vacancy_latest_observations WHERE source_name='union_structures_171'`, `DELETE FROM ingest_replay_attempts WHERE capture_id IN (SELECT id FROM ingest_captures WHERE source_name='union_structures_171')`, `DELETE FROM ingest_captures WHERE source_name='union_structures_171'`} {
		if _, err = pool.Exec(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}
	dir := t.TempDir()
	if err = os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	store, err := taxartifact.NewLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	r, err := ingest.NewRecorder(store, "union_structures_171", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	base := "https://example.test/server/rest/services/OperationalLayers/MapServer/171/query?"
	for _, receipt := range []struct {
		url, body string
		crs       int
	}{
		{base + "f=json&returnCountOnly=true&where=1%3D1", `{"count":1}`, 0},
		{base + "f=json&outFields=%2A&outSR=3857&resultRecordCount=2000&returnGeometry=true&where=1%3D1", `{"spatialReference":{"wkid":3857},"features":[{"attributes":{"OBJECTID":7},"geometry":{"rings":[[[0,0],[1,0],[0,0]]]}}]}`, 3857},
	} {
		if err = r.RecordHTTP(ctx, receipt.url, "", receipt.crs, 200, true, strings.NewReader(receipt.body)); err != nil {
			t.Fatal(err)
		}
	}
	engine := ingest.Engine{Pool: pool, Store: store}
	runner, err := jobs.NewRunner(pool)
	if err != nil {
		t.Fatal(err)
	}
	var id string
	_, _, err = runner.Run(ctx, jobs.Definition{ID: StructuresID, Run: func(ctx context.Context) (jobs.Result, error) {
		attempt, ok := jobs.AttemptID(ctx)
		if !ok {
			return jobs.Result{}, fmt.Errorf("missing attempt")
		}
		if err := r.BindAttempt(StructuresID, attempt, "full_scan", 0); err != nil {
			return jobs.Result{}, err
		}
		var err error
		id, err = engine.Seal(ctx, r)
		return jobs.Result{}, err
	}})
	if err != nil {
		t.Fatal(err)
	}
	job := Job{Pool: pool, Artifacts: store, Layer: union.StructuresLayer}
	if err = job.Replay(ctx, id); err != nil {
		t.Fatal(err)
	}
	var note string
	var count int
	if err = pool.QueryRow(ctx, `SELECT validation_note,observed_count FROM vacancy_live_generations WHERE capture_id=$1::uuid`, id).Scan(&note, &count); err != nil || note != "bounded single-page structures observation; source ID membership and county coverage unverified" || count != 1 {
		t.Fatalf("legacy publication: note=%q count=%d err=%v", note, count, err)
	}
}
