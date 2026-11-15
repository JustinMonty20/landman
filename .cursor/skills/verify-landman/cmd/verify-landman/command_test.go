package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestClassifyCommandSkipsSalesWhenImportDidNotCommit(t *testing.T) {
	dir := writeEvidence(t, "", slowDriveLog(30*time.Second)+"context deadline exceeded\n", "1", fastMetadataProbe(30*time.Second))
	text, code := runClassify(verifyContainerName, "postgres://landman/landman", filepath.Join(dir, "missing.sql"), dir, classifyDeps{
		query: func(ctx context.Context, container, databaseURL, sql string) (string, error) {
			t.Fatal("sale query ran")
			return "", nil
		},
		get: func(ctx context.Context, rawURL string) (int, []byte, error) {
			t.Fatal("county request ran")
			return 0, nil, nil
		},
	})
	if code != 2 || strings.TrimSpace(text) != string(verdictCountySlow) {
		t.Fatalf("exit %d verdict %q", code, text)
	}
	raw := readOutcome(t, dir)
	if jsonHasBool(mustJSON(t, raw)) {
		t.Fatalf("boolean in %s", raw)
	}
	if !strings.Contains(string(raw), `"sales": null`) && !strings.Contains(string(raw), `"sales":null`) {
		t.Fatalf("outcome %s", raw)
	}
	if !strings.Contains(string(raw), `"seededSales": "not-run"`) {
		t.Fatalf("outcome %s", raw)
	}
}

func TestClassifyCommandRefusesOtherContainerBeforeReads(t *testing.T) {
	_, code := runClassify("landman-postgis", "postgres://landman/landman", "seed.sql", "missing-evidence", classifyDeps{
		query: func(ctx context.Context, container, databaseURL, sql string) (string, error) {
			t.Fatal("query ran")
			return "", nil
		},
		get: func(ctx context.Context, rawURL string) (int, []byte, error) {
			t.Fatal("county request ran")
			return 0, nil, nil
		},
	})
	if code == 0 {
		t.Fatal("accepted other container")
	}
}

func TestClassifyCommandPassUsesSeedSaleQuery(t *testing.T) {
	seed := filepath.Join(t.TempDir(), "seed.sql")
	if err := os.WriteFile(seed, []byte(twoParcelSeed), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := writeEvidence(t, "Imported parcels, 2 likely-vacant feature IDs\n", "", "0", fastMetadataProbe(0))
	var sawSale, sawPost bool
	text, code := runClassify(verifyContainerName, "postgres://landman/landman", seed, dir, classifyDeps{
		get: func(ctx context.Context, rawURL string) (int, []byte, error) {
			if strings.Contains(rawURL, "/query?") {
				if !strings.Contains(rawURL, "orderByFields=OBJECTID") || !strings.Contains(rawURL, "PID+IN+") {
					t.Fatalf("sale page %s", rawURL)
				}
				return 200, []byte(salePageJSON), nil
			}
			return 200, []byte(`{"maxRecordCount":2000}`), nil
		},
		query: func(ctx context.Context, container, databaseURL, sql string) (string, error) {
			if strings.Contains(sql, "parcel_sales") && strings.Contains(sql, "count(*)") {
				t.Fatalf("county-wide sales count: %s", sql)
			}
			if strings.Contains(sql, "sale_date::text") {
				sawSale = true
				if !strings.Contains(sql, "LEFT JOIN parcel_sales") || !strings.Contains(sql, "'07102010'") || !strings.Contains(sql, "'07102051'") {
					t.Fatalf("sale sql %s", sql)
				}
				return "07102010\t1\t2026-09-30\t0.00\t9472\t0680\n", nil
			}
			if strings.Contains(sql, "soil_screening_heads") {
				sawPost = true
				if strings.Contains(sql, "parcel_sales") {
					t.Fatalf("post-import reads sales: %s", sql)
				}
				return strings.Join([]string{
					"county_parcels\t3",
					"soil_polygons\t1",
					"overlaps\t1",
					"parcel_jobs\t1",
					"soil_jobs\t1",
					"screening_jobs\t1",
					"heads\t1",
					"assessments\t3",
				}, "\n"), nil
			}
			t.Fatalf("unexpected sql %s", sql)
			return "", nil
		},
	})
	if !sawSale || !sawPost {
		t.Fatalf("sale %v post %v", sawSale, sawPost)
	}
	if code != 0 || strings.TrimSpace(text) != string(verdictPass) {
		t.Fatalf("exit %d verdict %q", code, text)
	}
	if jsonHasBool(mustJSON(t, readOutcome(t, dir))) {
		t.Fatal("boolean in drive outcome")
	}
}

func TestClassifyCommandCountySlowAfterCommitKeepsSalesMet(t *testing.T) {
	seed := filepath.Join(t.TempDir(), "seed.sql")
	if err := os.WriteFile(seed, []byte(twoParcelSeed), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := writeEvidence(t, "Imported parcels, 2 likely-vacant feature IDs\n", "context deadline exceeded\n", "1", fastMetadataProbe(30*time.Second))
	text, code := runClassify(verifyContainerName, "postgres://landman/landman", seed, dir, classifyDeps{
		get: salePageGetter(t),
		query: func(ctx context.Context, container, databaseURL, sql string) (string, error) {
			if strings.Contains(sql, "soil_screening_heads") {
				t.Fatal("post-import ran before exit 0")
			}
			return "07102010\t1\t2026-09-30\t0.00\t9472\t0680\n", nil
		},
	})
	if code != 2 || strings.TrimSpace(text) != string(verdictCountySlow) {
		t.Fatalf("exit %d verdict %q", code, text)
	}
	raw := readOutcome(t, dir)
	if !strings.Contains(string(raw), `"seededSales": "met"`) || !strings.Contains(string(raw), `"fullImport": "missed"`) {
		t.Fatalf("outcome %s", raw)
	}
}

func TestSaleQueryRejectsUnsafeParcelID(t *testing.T) {
	if _, err := saleQuerySQL([]string{"07102010';drop"}); err == nil {
		t.Fatal("accepted an unsafe parcel id")
	}
	sqlText, err := saleQuerySQL([]string{"07102010"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(saleSelectSQL, "ANY($1::text[])") || !strings.Contains(sqlText, "ARRAY['07102010']::text[]") {
		t.Fatalf("sql %s", sqlText)
	}
	if strings.Contains(postImportSQL, "parcel_sales") {
		t.Fatal("post-import SQL counts parcel_sales")
	}
}

func TestProbeWritesTimingsAndAcceptsSlowPage(t *testing.T) {
	dir := t.TempDir()
	code := runProbe(dir, func(ctx context.Context, rawURL string) (int, []byte, error) {
		return 200, []byte(`{"ok":true}`), nil
	})
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "page-timings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var timings pageTimings
	if err := json.Unmarshal(raw, &timings); err != nil {
		t.Fatal(err)
	}
	timings.ParcelPage.DurationMS = (30 * time.Second).Milliseconds()
	if timings.ParcelMetadata.HTTPStatus != 200 || !metaFast(timings.ParcelMetadata) || !callSlow(timings.ParcelPage) {
		t.Fatalf("metadata %#v page %#v", timings.ParcelMetadata, timings.ParcelPage)
	}
	parcel := parcelPageURL()
	for _, part := range []string{"where=1%3D1", "outFields=%2A", "returnGeometry=true", "orderByFields=OBJECTID", "resultRecordCount=2000", "f=json"} {
		if !strings.Contains(parcel, part) {
			t.Fatalf("parcel page %s missing %s", parcel, part)
		}
	}
	soil := soilPageURL()
	for _, part := range []string{"orderByFields=OBJECTID_1", "returnGeometry=true", "resultRecordCount=2000", "OBJECTID_1", "MUSYM", "MUKEY"} {
		if !strings.Contains(soil, part) {
			t.Fatalf("soil page %s missing %s", soil, part)
		}
	}
}

const twoParcelSeed = "" +
	"INSERT INTO parcels (id, source_parcel_id, situs_line_1, likely_vacant) VALUES ('11111111-1111-1111-1111-111111111111'::uuid, '07102010', '105 CUPPED OAK DR', 't');\n" +
	"INSERT INTO parcels (id, source_parcel_id, situs_line_1, likely_vacant) VALUES ('22222222-2222-2222-2222-222222222222'::uuid, '07102051', '203 WHITE OAK LN', 'f');\n"

const salePageJSON = `{"features":[
  {"attributes":{"PID":"07102010","OBJECTID":1,"s1_SALEDATE":1790726400000,"s1_SALESAMT":0,"s1_DEED_BOOK":"9472","s1_DEED_PAGE":"0680"}},
  {"attributes":{"PID":"07102051","OBJECTID":2}}
]}`

func salePageGetter(t *testing.T) httpGetter {
	t.Helper()
	return func(ctx context.Context, rawURL string) (int, []byte, error) {
		if strings.Contains(rawURL, "/query?") {
			return 200, []byte(salePageJSON), nil
		}
		return 200, []byte(`{"maxRecordCount":2000}`), nil
	}
}

func writeEvidence(t *testing.T, stdout, stderr, exitText string, probe pageTimings) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "import.stdout.txt"), stdout)
	writeFile(t, filepath.Join(dir, "import.stderr.txt"), stderr)
	writeFile(t, filepath.Join(dir, "import.exit.txt"), exitText)
	raw, err := json.Marshal(probe)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "page-timings.json"), string(raw))
	return dir
}

func writeFile(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readOutcome(t *testing.T, dir string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "drive-outcome.json"))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func mustJSON(t *testing.T, raw []byte) any {
	t.Helper()
	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	return payload
}
