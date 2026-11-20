//go:build postgres && ingestioncapture

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JustinMonty20/landman/internal/connector"
	"github.com/JustinMonty20/landman/internal/connector/shared/httpclient"
	"github.com/JustinMonty20/landman/internal/connector/union"
	"github.com/JustinMonty20/landman/internal/jobs"
	"github.com/jackc/pgx/v5/pgxpool"
)

const parcelCaptureJobID = "union.parcels.capture"
const parcelCapturePageSize = 2000
const parcelCaptureMaxBytes = 32 << 20

type parcelCaptureResponse struct {
	File      string        `json:"file"`
	Query     string        `json:"query"`
	Status    int           `json:"status"`
	Bytes     int           `json:"bytes"`
	SHA256    string        `json:"sha256"`
	StartedAt time.Time     `json:"started_at"`
	Elapsed   time.Duration `json:"elapsed_ns"`
	Features  int           `json:"features"`
}

type parcelCaptureManifest struct {
	Version   int                     `json:"version"`
	Endpoint  string                  `json:"endpoint"`
	PageSize  int                     `json:"page_size"`
	Rate      int                     `json:"requests_per_second"`
	Count     int                     `json:"count"`
	Responses []parcelCaptureResponse `json:"responses"`
}

func captureDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func writeCaptureFile(dir, name string, data []byte) error {
	f, err := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func readCaptureFile(dir, name string, maxBytes int64) ([]byte, error) {
	path := filepath.Join(dir, name)
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Size() > maxBytes {
		return nil, fmt.Errorf("capture file must be regular, private, and within size limit")
	}
	return os.ReadFile(path)
}

func captureQuery(index int) string {
	params := union.ArcGISParams{Where: "1=1", ReturnCountOnly: true, Format: "json"}
	if index > 0 {
		params = union.ArcGISParams{Where: "1=1", ReturnGeometry: true, ResultOffset: (index - 1) * parcelCapturePageSize,
			ResultRecordCount: parcelCapturePageSize, Format: "json", OutFields: "*", OrderByFields: "OBJECTID"}
	}
	values, _ := params.ToURLValues()
	return values.Encode()
}

func validateCaptureBody(index, count int, body []byte) (int, error) {
	var response struct {
		Count    *int              `json:"count"`
		Features []json.RawMessage `json:"features"`
		Error    json.RawMessage   `json:"error"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return 0, fmt.Errorf("capture response is not valid JSON")
	}
	if len(response.Error) != 0 {
		return 0, fmt.Errorf("capture response contains ArcGIS error")
	}
	if index == 0 {
		if response.Count == nil || *response.Count <= 0 {
			return 0, fmt.Errorf("capture response requires positive integer count")
		}
		return *response.Count, nil
	}
	want := min(parcelCapturePageSize, count-(index-1)*parcelCapturePageSize)
	if response.Features == nil || want <= 0 || len(response.Features) != want {
		return 0, fmt.Errorf("capture page %d incomplete: got %d want %d", index, len(response.Features), want)
	}
	return len(response.Features), nil
}

func freezeParcelCapture(dir string, manifest parcelCaptureManifest) error {
	if len(manifest.Responses) != 1+(manifest.Count+parcelCapturePageSize-1)/parcelCapturePageSize {
		return fmt.Errorf("capture has incomplete response set")
	}
	body, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if err := writeCaptureFile(dir, "manifest.json", body); err != nil {
		return err
	}
	return writeCaptureFile(dir, "COMPLETE", []byte(captureDigest(body)+"\n"))
}

func loadParcelCapture(dir string) (parcelCaptureManifest, error) {
	var manifest parcelCaptureManifest
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
		return manifest, fmt.Errorf("capture directory must be private and not a symlink")
	}
	body, err := readCaptureFile(dir, "manifest.json", 1<<20)
	if err != nil {
		return manifest, err
	}
	marker, err := readCaptureFile(dir, "COMPLETE", 128)
	if err != nil || string(marker) != captureDigest(body)+"\n" {
		return manifest, fmt.Errorf("capture completion checksum mismatch or missing marker")
	}
	if err := json.Unmarshal(body, &manifest); err != nil {
		return manifest, fmt.Errorf("invalid capture manifest")
	}
	if manifest.Version != 1 || manifest.Endpoint != union.DefaultGISSourceConfig().BaseURL || manifest.PageSize != parcelCapturePageSize || manifest.Rate != 1 || manifest.Count <= 0 || len(manifest.Responses) != 1+(manifest.Count+parcelCapturePageSize-1)/parcelCapturePageSize {
		return manifest, fmt.Errorf("capture manifest contract mismatch")
	}
	for i, entry := range manifest.Responses {
		if entry.File != fmt.Sprintf("response-%04d.json", i) || entry.Query != captureQuery(i) || entry.Status < 200 || entry.Status > 299 {
			return manifest, fmt.Errorf("capture response %d metadata mismatch", i)
		}
		data, err := readCaptureFile(dir, entry.File, parcelCaptureMaxBytes)
		if err != nil {
			return manifest, err
		}
		n, err := validateCaptureBody(i, manifest.Count, data)
		if err != nil || entry.Bytes != len(data) || entry.SHA256 != captureDigest(data) || (i > 0 && entry.Features != n) || (i == 0 && n != manifest.Count) {
			return manifest, fmt.Errorf("capture response %d checksum, count, or schema mismatch", i)
		}
	}
	return manifest, nil
}

type parcelCaptureProxy struct {
	mu        sync.Mutex
	dir       string
	manifest  parcelCaptureManifest
	client    *http.Client
	replay    bool
	requested int
	completed int
	errors    int
	bytes     int
	failure   string
}

func (p *parcelCaptureProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	defer p.mu.Unlock()
	index := p.completed
	fail := func(reason string) {
		p.errors++
		p.failure = reason
		http.Error(w, "capture response unavailable or invalid; inspect private fixture metadata", http.StatusBadGateway)
	}
	if r.Method != http.MethodGet || r.URL.Path != "/" || r.URL.RawQuery != captureQuery(index) {
		fail("unexpected request method, path, or ordered query")
		return
	}
	p.requested++
	var body []byte
	var err error
	if p.replay {
		if index >= len(p.manifest.Responses) {
			fail("replay requested beyond complete response set")
			return
		}
		entry := p.manifest.Responses[index]
		body, err = readCaptureFile(p.dir, entry.File, parcelCaptureMaxBytes)
		if err != nil || len(body) != entry.Bytes || captureDigest(body) != entry.SHA256 {
			fail("replay response unavailable or checksum mismatch")
			return
		}
	} else {
		req, requestErr := http.NewRequestWithContext(r.Context(), http.MethodGet, p.manifest.Endpoint+"?"+r.URL.RawQuery, nil)
		if requestErr != nil {
			fail(fmt.Sprintf("construct official request: %v", requestErr))
			return
		}
		req.Header.Set("User-Agent", "ParcelDataCollection/0.0.1")
		started := time.Now()
		resp, requestErr := p.client.Do(req)
		if requestErr != nil {
			fail(fmt.Sprintf("official request failed: %v", requestErr))
			return
		}
		body, err = io.ReadAll(io.LimitReader(resp.Body, parcelCaptureMaxBytes+1))
		closeErr := resp.Body.Close()
		entry := parcelCaptureResponse{File: fmt.Sprintf("response-%04d.json", index), Query: r.URL.RawQuery,
			Status: resp.StatusCode, Bytes: len(body), SHA256: captureDigest(body), StartedAt: started.UTC(), Elapsed: time.Since(started)}
		p.completed++
		p.bytes += len(body)
		if writeErr := writeCaptureFile(p.dir, entry.File, body); writeErr != nil {
			fail(fmt.Sprintf("save private response: %v", writeErr))
			return
		}
		n, validationErr := validateCaptureBody(index, p.manifest.Count, body)
		if index == 0 {
			p.manifest.Count = n
		} else {
			entry.Features = n
		}
		p.manifest.Responses = append(p.manifest.Responses, entry)
		if err != nil || closeErr != nil || len(body) > parcelCaptureMaxBytes || resp.StatusCode < 200 || resp.StatusCode > 299 || validationErr != nil {
			fail(fmt.Sprintf("official response status=%d bytes=%d read_error=%v close_error=%v validation_error=%v", resp.StatusCode, len(body), err, closeErr, validationErr))
			return
		}
	}
	if p.replay {
		p.completed++
		p.bytes += len(body)
	}
	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(body); err != nil {
		p.errors++
	}
}

func parcelCaptureSource(t *testing.T, proxy *parcelCaptureProxy) *union.GISSource {
	t.Helper()
	server := httptest.NewServer(proxy)
	t.Cleanup(server.Close)
	config := union.DefaultGISSourceConfig()
	config.BaseURL = server.URL
	config.BatchSize = parcelCapturePageSize
	source, err := union.NewGISSource(config)
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func TestLiveParcelCaptureRepeatability(t *testing.T) {
	mode := os.Getenv("UNION_PARCEL_CAPTURE_MODE")
	if mode == "" {
		t.Skip("set UNION_PARCEL_CAPTURE_MODE=capture or replay for this disposable-DB gate")
	}
	if mode != "capture" && mode != "replay" {
		t.Fatal("UNION_PARCEL_CAPTURE_MODE must be capture or replay")
	}
	if mode == "capture" && os.Getenv("UNION_PARCEL_ALLOW_LIVE") != "1" {
		t.Fatal("live capture requires UNION_PARCEL_ALLOW_LIVE=1")
	}
	dir := os.Getenv("UNION_PARCEL_CAPTURE_DIR")
	if dir == "" {
		dir = "artifacts/verify/ingestion-gaps-live/capture"
	}
	dir = filepath.Clean(dir)
	if filepath.IsAbs(dir) || dir == "." || dir == ".." || strings.HasPrefix(dir, ".."+string(filepath.Separator)) {
		t.Fatal("UNION_PARCEL_CAPTURE_DIR must be a relative private fixture path")
	}
	if os.Getenv("UNION_PARCEL_DISPOSABLE_DB") != "1" || os.Getenv("DATABASE_URL") == "" {
		t.Fatal("explicit DATABASE_URL and UNION_PARCEL_DISPOSABLE_DB=1 required")
	}
	ctx := context.Background()
	dsn := os.Getenv("DATABASE_URL")
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	for _, table := range []string{"parcels", "parcel_sales", "job_attempts", "quarantined_records", "soil_polygons"} {
		if countRows(t, ctx, pool, table) != 0 {
			t.Fatalf("disposable migrated DB must start empty: %s", table)
		}
	}
	runner, err := jobs.NewRunner(pool)
	if err != nil {
		t.Fatal(err)
	}
	if mode == "capture" {
		if err := os.MkdirAll(filepath.Dir(dir), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal("capture path must not exist; never overwrite a fixture")
		}
		client := httpclient.NewHTTPClient()
		client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		defer client.CloseIdleConnections()
		proxy := &parcelCaptureProxy{dir: dir, client: client, manifest: parcelCaptureManifest{
			Version: 1, Endpoint: union.DefaultGISSourceConfig().BaseURL, PageSize: parcelCapturePageSize, Rate: 1}}
		source := parcelCaptureSource(t, proxy)
		captureCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		started := time.Now()
		outcome, result, err := runner.Run(captureCtx, jobs.Definition{ID: parcelCaptureJobID, Run: func(ctx context.Context) (jobs.Result, error) {
			var result jobs.Result
			err := source.FetchBatches(ctx, func(records []connector.RawRecord) error {
				result.Processed += len(records)
				return nil
			})
			if err == nil {
				proxy.mu.Lock()
				if proxy.errors != 0 || result.Processed != proxy.manifest.Count {
					err = fmt.Errorf("capture request or feature count mismatch")
				} else {
					err = freezeParcelCapture(dir, proxy.manifest)
				}
				proxy.mu.Unlock()
			}
			return result, err
		}})
		cancel()
		proxy.mu.Lock()
		t.Logf("live capture attempted_requests=%d completed_requests=%d errors=%d bytes=%d outcome=%s processed=%d elapsed=%s error=%v blocker=%s", proxy.requested, proxy.completed, proxy.errors, proxy.bytes, outcome, result.Processed, time.Since(started), err, proxy.failure)
		proxy.mu.Unlock()
		if err != nil || outcome != jobs.OutcomeSuccess {
			t.Fatal("live capture failed; no replay and no automatic network retry")
		}
	}
	manifest, err := loadParcelCapture(dir)
	if err != nil {
		t.Fatal(err)
	}
	if os.Getenv("UNION_PARCEL_FAIL_FIRST_PUBLICATION") == "1" {
		if mode != "replay" {
			t.Fatal("publication failure injection requires offline replay")
		}
		_, err := pool.Exec(ctx, `CREATE FUNCTION reject_fixture_publication() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN RAISE EXCEPTION 'fixture late publication failure'; END $$;
		CREATE TRIGGER reject_fixture_publication BEFORE INSERT ON parcel_derived_handoff
		FOR EACH STATEMENT EXECUTE FUNCTION reject_fixture_publication()`)
		if err != nil {
			t.Fatal(err)
		}
		source := parcelCaptureSource(t, &parcelCaptureProxy{dir: dir, manifest: manifest, replay: true})
		attemptCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		outcome, _, err := runParcelFixture(attemptCtx, pool, dsn, source, false, 0)
		cancel()
		if outcome != jobs.OutcomeFailed || err == nil || !strings.Contains(err.Error(), "fixture late publication failure") {
			t.Fatalf("expected injected failure, got %s: %v", outcome, err)
		}
		for _, table := range []string{"parcels", "parcel_sales", "parcel_owner_labels", "parcel_evidence_current", "parcel_evidence_history", "parcel_evidence_observations", "parcel_derived_handoff", "ingest_captures WHERE published_at IS NOT NULL"} {
			if n := countRows(t, ctx, pool, table); n != 0 {
				t.Fatalf("failed publication left %d rows in %s", n, table)
			}
		}
		if _, err := pool.Exec(ctx, `DROP TRIGGER reject_fixture_publication ON parcel_derived_handoff; DROP FUNCTION reject_fixture_publication()`); err != nil {
			t.Fatal(err)
		}
		t.Log("late publication failure rolled back all serving and parcel evidence rows")
	}
	for pass := 1; pass <= 2; pass++ {
		proxy := &parcelCaptureProxy{dir: dir, manifest: manifest, replay: true}
		source := parcelCaptureSource(t, proxy)
		attemptCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		started := time.Now()
		outcome, result, err := runParcelFixture(attemptCtx, pool, dsn, source, false, 0)
		cancel()
		proxy.mu.Lock()
		requests, requestErrors, bytes := proxy.completed, proxy.errors, proxy.bytes
		proxy.mu.Unlock()
		t.Logf("frozen replay=%d completed_requests=%d errors=%d bytes=%d source_features=%d outcome=%s processed=%d quarantined=%d parcels=%d sales=%d elapsed=%s error=%v", pass, requests, requestErrors, bytes, manifest.Count, outcome, result.Processed, result.Quarantined, countRows(t, ctx, pool, "parcels"), countRows(t, ctx, pool, "parcel_sales"), time.Since(started), err)
		if err != nil || outcome != jobs.OutcomeSuccess || result.Processed != manifest.Count || result.Quarantined != 0 || requests != len(manifest.Responses) || requestErrors != 0 {
			t.Fatal("frozen full import did not meet clean deadline/count gate; inspect private DB quarantine and attempt history")
		}
		if pass == 1 {
			if _, err := pool.Exec(ctx, `CREATE TABLE capture_parcels_before AS SELECT id,to_jsonb(p)-'updated_at' AS value,ST_AsEWKB(shape) AS shape FROM parcels p;
CREATE TABLE capture_sales_before AS SELECT id,to_jsonb(s)-'updated_at' AS value FROM parcel_sales s`); err != nil {
				t.Fatal(err)
			}
		}
	}
	var differentParcels, differentSales, invalidSRID, duplicateKeys int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM capture_parcels_before b FULL JOIN parcels p USING(id)
WHERE b.id IS NULL OR p.id IS NULL OR b.value IS DISTINCT FROM to_jsonb(p)-'updated_at' OR b.shape IS DISTINCT FROM ST_AsEWKB(p.shape)
OR ST_Equals(ST_GeomFromEWKB(b.shape),p.shape) IS DISTINCT FROM true`).Scan(&differentParcels); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM capture_sales_before b FULL JOIN parcel_sales s USING(id)
WHERE b.id IS NULL OR s.id IS NULL OR b.value IS DISTINCT FROM to_jsonb(s)-'updated_at'`).Scan(&differentSales); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM parcels WHERE shape IS NULL OR srid<>3857 OR ST_SRID(shape)<>3857`).Scan(&invalidSRID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*)-count(DISTINCT (county_id,source_name,source_parcel_id)) FROM parcels)
+ (SELECT count(*)-count(DISTINCT (parcel_id,sequence)) FROM parcel_sales)`).Scan(&duplicateKeys); err != nil {
		t.Fatal(err)
	}
	cleanAttempts := countRows(t, ctx, pool, "job_attempts WHERE job_id='"+parcelJobID+"' AND outcome='success' AND attempt_kind='full_scan'")
	quarantine := countRows(t, ctx, pool, "quarantined_records")
	t.Logf("equality parcel_differences=%d sale_differences=%d invalid_srid=%d duplicate_natural_keys=%d clean_full_scans=%d quarantine=%d", differentParcels, differentSales, invalidSRID, duplicateKeys, cleanAttempts, quarantine)
	if differentParcels != 0 || differentSales != 0 || invalidSRID != 0 || duplicateKeys != 0 || cleanAttempts != 2 || quarantine != 0 {
		t.Fatal("unchanged capture equality gate failed")
	}
}

func TestParcelCaptureIntegrity(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "capture")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	manifest := parcelCaptureManifest{Version: 1, Endpoint: union.DefaultGISSourceConfig().BaseURL, PageSize: parcelCapturePageSize, Rate: 1, Count: 1}
	for i, body := range []string{`{"count":1}`, `{"features":[{"attributes":{"OBJECTID":1,"PID":"A"}}]}`} {
		entry := parcelCaptureResponse{File: fmt.Sprintf("response-%04d.json", i), Query: captureQuery(i), Status: 200, Bytes: len(body), SHA256: captureDigest([]byte(body)), Features: i}
		if err := writeCaptureFile(dir, entry.File, []byte(body)); err != nil {
			t.Fatal(err)
		}
		manifest.Responses = append(manifest.Responses, entry)
	}
	if _, err := loadParcelCapture(dir); err == nil {
		t.Fatal("accepted incomplete capture")
	}
	if err := freezeParcelCapture(dir, manifest); err != nil {
		t.Fatal(err)
	}
	if _, err := loadParcelCapture(dir); err != nil {
		t.Fatal(err)
	}
	if err := freezeParcelCapture(dir, manifest); err == nil {
		t.Fatal("overwrote immutable manifest")
	}
	if err := os.WriteFile(filepath.Join(dir, "response-0001.json"), []byte(strconv.Quote("tampered")), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadParcelCapture(dir); err == nil {
		t.Fatal("accepted tampered response")
	}
}
