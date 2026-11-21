//go:build postgres

package parcelapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestTaxUnmatchedReview(t *testing.T) {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, os.Getenv("API_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var db string
	if err := pool.QueryRow(ctx, "SELECT current_database()").Scan(&db); err != nil || !strings.HasPrefix(db, "landman_api_test_") {
		t.Fatal("requires disposable API database")
	}
	var id string
	err = pool.QueryRow(ctx, `INSERT INTO tax_file_imports(county_id,source_name,source_url,fetched_at,byte_size,file_sha256,artifact_key,storage_mode,validation_result,active) VALUES('union','union_billing','https://example.test/bills',now(),0,decode(repeat('ab',32),'hex'),'tax/union_billing/sha256/'||repeat('ab',32),'local','valid',false) RETURNING id::text`).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, "DELETE FROM tax_file_imports WHERE id=$1", id)
	for _, key := range []string{"REVIEW-A", "REVIEW-B"} {
		if _, err := pool.Exec(ctx, `INSERT INTO tax_unmatched_records(import_id,parcel_key,tax_year) VALUES($1,$2,2026)`, id, key); err != nil {
			t.Fatal(err)
		}
	}
	handler := New(pool)
	get := func(path string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		return w
	}
	base := "/api/v1/tax/unmatched?source=union_billing&import_id=" + id
	if w := get("/api/v1/tax/unmatched?source=union_billing&import_id=" + strings.ToUpper(id)); w.Code != 200 {
		t.Fatalf("uppercase UUID: %d %s", w.Code, w.Body)
	}
	w := get(base + "&limit=1")
	var result struct {
		Records []unmatchedTaxRecord `json:"records"`
		Next    *int64               `json:"next_after"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || len(result.Records) != 1 || result.Next == nil || result.Records[0].MatchedNow {
		t.Fatalf("first page: %d %s", w.Code, w.Body)
	}
	if strings.Contains(w.Body.String(), "artifact") || strings.Contains(w.Body.String(), "taxpayer") {
		t.Fatal("private fields exposed")
	}
	next := *result.Next
	if _, err := pool.Exec(ctx, `INSERT INTO parcels(county_id,county_code,source_name,source_parcel_id) VALUES('union','union','union_county_gis','REVIEW-A')`); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM parcels WHERE source_parcel_id='REVIEW-A'`)
	w = get(base)
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || !result.Records[0].MatchedNow {
		t.Fatalf("resolved: %d %s", w.Code, w.Body)
	}
	w = get(base + fmt.Sprintf("&after=%d", next))
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || len(result.Records) != 1 || result.Records[0].ParcelKey != "REVIEW-B" {
		t.Fatalf("next page: %d %s", w.Code, w.Body)
	}
	for _, suffix := range []string{"&limit=501", "&after=-1", "&unknown=x", "&source=union_delinquent"} {
		if w := get(base + suffix); w.Code != 400 {
			t.Fatalf("validation %s: %d", suffix, w.Code)
		}
	}
	if w := get("/api/v1/tax/unmatched?source=union_delinquent&import_id=" + id); w.Code != 404 {
		t.Fatalf("source isolation: %d", w.Code)
	}
}
