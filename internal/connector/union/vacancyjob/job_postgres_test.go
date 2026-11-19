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
	"strings"
	"testing"

	"github.com/JustinMonty20/landman/internal/connector/union"
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
	if query(`SELECT count(*) FROM vacancy_live_generations`) != 1 || query(`SELECT count(*) FROM ingest_captures WHERE source_name='union_assessed_value_0'`) != 2 {
		t.Fatal("repeat rewrote evidence or lost observation")
	}
	version = 3
	if err := run(); err != nil {
		t.Fatal(err)
	}
	if query(`SELECT count(*) FROM vacancy_live_generations`) != 1 {
		t.Fatal("format-only observation published")
	}
	var acceptedAfterEquivalent string
	if err := pool.QueryRow(ctx, `SELECT capture_id::text FROM vacancy_latest_observations WHERE source_name='union_assessed_value_0'`).Scan(&acceptedAfterEquivalent); err != nil {
		t.Fatal(err)
	}
	version = 2
	if err := run(); err == nil {
		t.Fatal("partial capture accepted")
	}
	if query(`SELECT count(*) FROM vacancy_live_generations`) != 1 {
		t.Fatal("failed capture published")
	}
	version = 4
	if err := run(); err == nil {
		t.Fatal("malformed JSON accepted")
	}
	if query(`SELECT count(*) FROM ingest_captures WHERE source_name='union_assessed_value_0' AND jsonb_array_length(convert_from(manifest,'UTF8')::jsonb->'receipts')=2`) < 1 {
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
		if len(receipt.Receipts) != 2 {
			t.Fatalf("missing failed capture page: %s", manifest)
		}
		body, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(receipt.Receipts[1].ArtifactKey)))
		if err != nil || string(body) != rejected.body {
			t.Fatalf("wrong failed capture bytes: %q %v", body, err)
		}
		if rejected.version == 5 {
			if receipt.Receipts[1].HTTPStatus != 403 {
				t.Fatalf("lost refusal status: %s", manifest)
			}
			var rejectedID string
			if err := pool.QueryRow(ctx, `SELECT id::text FROM ingest_captures WHERE source_name='union_assessed_value_0' ORDER BY observed_at DESC LIMIT 1`).Scan(&rejectedID); err != nil {
				t.Fatal(err)
			}
			before := requests
			_, _, replayErr := runner.Run(ctx, jobs.Definition{ID: AssessedValueID, Kind: jobs.AttemptReplay, Run: func(ctx context.Context) (jobs.Result, error) { return jobs.Result{}, job.Replay(ctx, rejectedID) }})
			if replayErr == nil || requests != before || query(`SELECT count(*) FROM vacancy_live_generations`) != 1 {
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
	if err != nil || requests != requestsBeforeEquivalentReplay || query(`SELECT count(*) FROM vacancy_live_generations`) != 1 || query(`SELECT count(*) FROM vacancy_live_generations WHERE capture_id=(SELECT capture_id FROM vacancy_latest_observations WHERE source_name='union_assessed_value_0')`) != 0 {
		t.Fatalf("stale replay outranked equivalent observation: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE vacancy_live_generations SET normalizer_version='vacancy-rows-v1' WHERE capture_id=$1::uuid`, older); err != nil {
		t.Fatal(err)
	}
	version = 1
	if err := run(); err != nil {
		t.Fatal(err)
	}
	if query(`SELECT count(*) FROM vacancy_live_generations`) != 2 {
		t.Fatal("changed capture not published")
	}
	version = 0
	if err := run(); err != nil {
		t.Fatal(err)
	}
	if query(`SELECT count(*) FROM vacancy_live_generations`) != 3 {
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
	if err != nil || requests != requestsBefore || query(`SELECT count(*) FROM vacancy_live_generations WHERE normalizer_version='vacancy-rows-v1'`) != 3 || query(`SELECT count(*) FROM vacancy_live_assessments WHERE market_building=999`) != 0 {
		t.Fatalf("correction replay failed: %v", err)
	}
	_, _, err = runner.Run(ctx, jobs.Definition{ID: AssessedValueID, Kind: jobs.AttemptReplay, Run: func(ctx context.Context) (jobs.Result, error) { return jobs.Result{}, job.Replay(ctx, older) }})
	if err != nil || requests != requestsBefore || query(`SELECT count(*) FROM vacancy_live_generations`) != 3 || query(`SELECT count(*) FROM vacancy_live_assessments WHERE market_building=999`) != 0 {
		t.Fatalf("older replay changed evidence or contacted HTTP: %v", err)
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
	if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(receipt.Receipts[1].ArtifactKey)), []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	_, _, err = runner.Run(ctx, jobs.Definition{ID: AssessedValueID, Kind: jobs.AttemptReplay, Run: func(ctx context.Context) (jobs.Result, error) { return jobs.Result{}, job.Replay(ctx, older) }})
	if err == nil || query(`SELECT count(*) FROM vacancy_live_generations`) != 3 || requests != requestsBefore {
		t.Fatalf("corrupt replay accepted or contacted HTTP: %v", err)
	}
}
