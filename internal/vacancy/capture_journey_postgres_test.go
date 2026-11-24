//go:build postgres

package vacancy

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JustinMonty20/landman/internal/connector/union"
	"github.com/JustinMonty20/landman/internal/connector/union/vacancyjob"
	"github.com/JustinMonty20/landman/internal/jobs"
	"github.com/JustinMonty20/landman/internal/parcelapi"
	"github.com/JustinMonty20/landman/internal/taxartifact"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCapturedReceiptsAdmissionPublicationAndAPI(t *testing.T) {
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
	var database string
	if err := pool.QueryRow(ctx, `SELECT current_database()`).Scan(&database); err != nil || !strings.HasPrefix(database, "landman_api_test_") {
		t.Fatal("requires disposable PostGIS database", err)
	}
	defer func() {
		for _, sql := range []string{
			`TRUNCATE vacancy_assessments,vacancy_live_generations CASCADE`,
			`DELETE FROM vacancy_latest_observations`,
			`DELETE FROM ingest_replay_attempts WHERE capture_id IN (SELECT id FROM ingest_captures WHERE source_name IN ('union_structures_171','union_assessed_value_0'))`,
			`DELETE FROM ingest_captures WHERE source_name IN ('union_structures_171','union_assessed_value_0')`,
			`DELETE FROM parcel_evidence_current WHERE source_parcel_id='capture-test'`,
			`DELETE FROM parcels WHERE source_parcel_id='capture-test'`,
			`DELETE FROM parcel_evidence_observations WHERE artifact_id='capture-test-parcel'`,
		} {
			if _, err := pool.Exec(ctx, sql); err != nil {
				t.Errorf("cleanup %s: %v", sql, err)
			}
		}
	}()
	if _, err := pool.Exec(ctx, `TRUNCATE vacancy_assessments,vacancy_live_generations CASCADE`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM vacancy_latest_observations`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM ingest_replay_attempts WHERE capture_id IN (SELECT id FROM ingest_captures WHERE source_name IN ('union_structures_171','union_assessed_value_0'))`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM ingest_captures WHERE source_name IN ('union_structures_171','union_assessed_value_0')`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM parcel_evidence_current WHERE source_parcel_id='capture-test'`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM parcels WHERE source_parcel_id='capture-test'`); err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO parcels(county_id,county_code,source_name,source_parcel_id,parcel_number,year_built,sqft,shape)
 VALUES('union','179','union_county_gis','capture-test','capture-test',0,0,ST_MakeEnvelope(0,0,10,10,3857))`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO parcel_evidence_observations(artifact_id,observed_at,attempt_kind,manifest) VALUES('capture-test-parcel',now(),'full_scan','{}')`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO parcel_evidence_current(county_id,source_name,source_parcel_id,observation_id,source_hash,source_hash_version,normalized_hash,normalizer_version,normalized_hash_version,geometry_hash,geometry_hash_version,source_features,parcel_row_version)
 SELECT 'union','union_county_gis',p.source_parcel_id,o.id,repeat('a',64),'test',repeat('b',64),'test','test',repeat('c',64),'test',jsonb_build_array(jsonb_build_object('attributes',jsonb_build_object('PID',p.source_parcel_id,'YEARBLT',0,'SQFT',0))),p.xmin::text
 FROM parcels p CROSS JOIN parcel_evidence_observations o WHERE p.source_parcel_id='capture-test' AND o.artifact_id='capture-test-parcel'`)
	if err != nil {
		t.Fatal(err)
	}
	fixtures := map[string][]byte{}
	for _, name := range []string{"structures-count", "structures-ids", "structures-page", "assessment-count", "assessment-page"} {
		fixtures[name], err = os.ReadFile(filepath.Join("testdata", "capture", name+".json"))
		if err != nil {
			t.Fatal(err)
		}
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := "assessment-"
		if strings.Contains(r.URL.Path, "/171/query") {
			name = "structures-"
		}
		if r.URL.Query().Get("returnCountOnly") == "true" {
			name += "count"
		} else if r.URL.Query().Get("returnIdsOnly") == "true" {
			name += "ids"
		} else {
			name += "page"
		}
		body, ok := fixtures[name]
		if !ok {
			http.Error(w, "unsupported local fixture", 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer server.Close()
	dir := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	cert := filepath.Join(dir, "fixture-cert.pem")
	if _, err := x509.ParseCertificate(server.Certificate().Raw); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cert, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SSL_CERT_FILE", cert)
	source, err := union.NewGISSource(union.GISSourceConfig{BaseURL: server.URL + "/server/rest/services/OperationalLayers/MapServer/215/query", RateLimit: 1000})
	if err != nil {
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
	for _, item := range []struct {
		id    string
		layer union.VacancyLayer
	}{{vacancyjob.StructuresID, union.StructuresLayer}, {vacancyjob.AssessedValueID, union.AssessedValueLayer}} {
		job := vacancyjob.Job{Source: source, Pool: pool, Artifacts: artifacts, Layer: item.layer}
		run := func() {
			t.Helper()
			if _, _, e := runner.Run(ctx, jobs.Definition{ID: item.id, Run: job.Import}); e != nil {
				t.Fatal(e)
			}
		}
		run()
		var generation, capture, hash, sourceURL string
		var count int
		err = pool.QueryRow(ctx, `SELECT g.id::text,g.capture_id::text,g.normalized_sha256,g.observed_count FROM vacancy_live_generations g JOIN ingest_captures c ON c.id=g.capture_id WHERE g.source_name=$1 ORDER BY g.retrieved_at DESC LIMIT 1`, map[string]string{vacancyjob.StructuresID: "union_structures_171", vacancyjob.AssessedValueID: "union_assessed_value_0"}[item.id]).Scan(&generation, &capture, &hash, &count)
		if err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("writer manifest count %d", count)
		}
		var manifest []byte
		if err = pool.QueryRow(ctx, `SELECT manifest FROM ingest_captures WHERE id=$1::uuid`, capture).Scan(&manifest); err != nil {
			t.Fatal(err)
		}
		var retained struct {
			Count    int `json:"count"`
			Receipts []struct {
				Request string `json:"request"`
			} `json:"receipts"`
		}
		if err = json.Unmarshal(manifest, &retained); err != nil || retained.Count != 1 || len(retained.Receipts) < 3 {
			t.Fatalf("retained writer manifest %s: %v", manifest, err)
		}
		sourceURL = strings.Split(retained.Receipts[0].Request, "?")[0]
		validation := `INSERT INTO vacancy_live_validation(generation_id,capture_id,source_name,normalized_sha256,source_url,terms_evidence,coverage_evidence,count_evidence,snapshot_evidence,source_as_of,status_evidence,current_statuses,effective_av_year)
 VALUES($1::uuid,$2::uuid,$3,$4,$5,'fixture terms reviewed','fixture coverage reviewed','fixture count reviewed','fixture snapshot reviewed',current_date,$6,$7,$8)`
		var status any
		var statuses any = []string{}
		var year any
		sourceName := "union_structures_171"
		if item.layer == union.StructuresLayer {
			status = "existing status reviewed"
			statuses = []string{"existing"}
		} else {
			sourceName = "union_assessed_value_0"
			year = 2026
		}
		if _, err = pool.Exec(ctx, validation, generation, capture, sourceName, hash, sourceURL, status, statuses, year); err != nil {
			t.Fatal(err)
		}
		run()
		var distinct, validated int
		if err = pool.QueryRow(ctx, `SELECT count(*),count(*) FILTER (WHERE v.generation_id IS NOT NULL) FROM vacancy_live_generations g LEFT JOIN vacancy_live_validation v ON v.generation_id=g.id WHERE g.source_name=$1`, sourceName).Scan(&distinct, &validated); err != nil || distinct != 2 || validated != 1 {
			t.Fatalf("repeat generation=%d validated=%d: %v", distinct, validated, err)
		}
		// The latest repeat is not admitted by the earlier review.
		if err = Publish(ctx, pool); err == nil {
			t.Fatal("published after unreviewed repeat")
		}
		if _, err = pool.Exec(ctx, `INSERT INTO vacancy_live_validation(generation_id,capture_id,source_name,normalized_sha256,source_url,terms_evidence,coverage_evidence,count_evidence,snapshot_evidence,source_as_of,status_evidence,current_statuses,effective_av_year)
 SELECT g.id,g.capture_id,g.source_name,g.normalized_sha256,$2,'repeat fixture terms reviewed','repeat fixture coverage reviewed','repeat fixture count checked','repeat fixture snapshot reviewed',current_date,$3,$4,$5
 FROM vacancy_live_generations g WHERE g.source_name=$1 AND g.capture_id=(SELECT capture_id FROM vacancy_latest_observations WHERE source_name=g.source_name)`, sourceName, sourceURL, status, statuses, year); err != nil {
			t.Fatal(err)
		}
	}
	if err = Publish(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var id string
	if err = pool.QueryRow(ctx, `SELECT id::text FROM parcels WHERE source_parcel_id='capture-test'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	parcelapi.New(pool).ServeHTTP(recorder, httptest.NewRequest("GET", fmt.Sprintf("/api/v1/parcels/%s/vacancy", id), nil))
	if recorder.Code != 200 || !strings.Contains(recorder.Body.String(), `"tier":"strong_candidate"`) {
		t.Fatalf("published API %d %s", recorder.Code, recorder.Body.String())
	}
}
