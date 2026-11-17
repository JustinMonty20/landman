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
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestTaxActiveFeedIdentity(t *testing.T) {
	dsn := os.Getenv("API_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("API_TEST_DATABASE_URL not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var db string
	if err = pool.QueryRow(context.Background(), "SELECT current_database()").Scan(&db); err != nil || !strings.HasPrefix(db, "landman_api_test_") {
		t.Fatal("refusing non-disposable database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	var parcel, empty string
	for n, target := range []*string{&parcel, &empty} {
		if err = tx.QueryRow(ctx, `INSERT INTO parcels(county_id,county_code,source_name,source_parcel_id,shape) VALUES('union','union','union_county_gis',$1,ST_Transform(ST_MakeEnvelope(-79.8,33.2,-79.799,33.201,4326),3857)) RETURNING id::text`, fmt.Sprintf("tax-api-fixture-%d-%d", time.Now().UnixNano(), n)).Scan(target); err != nil {
			t.Fatal(err)
		}
	}
	deferred := []string{parcel, empty}
	defer func() {
		for _, id := range deferred {
			pool.Exec(context.Background(), "DELETE FROM parcels WHERE id=$1", id)
		}
	}()
	names := []string{"api_bills", "api_balances", "api_pipeline", "api_cases"}
	for n, name := range names {
		url := "https://example.test/" + name
		hash := fmt.Sprintf("%064x", n+1)
		if _, err = tx.Exec(ctx, `INSERT INTO tax_file_imports(county_id,source_name,source_url,fetched_at,byte_size,file_sha256,artifact_key,storage_mode,validation_result,active) VALUES('union',$1,$2,'2026-01-02',100,decode($3,'hex'),$4,'local','valid',true)`, name, url, hash, "tax/"+name+"/sha256/"+hash); err != nil {
			t.Fatal(err)
		}
	}
	file := func(n int) []any {
		return []any{names[n], "https://example.test/" + names[n], fmt.Sprintf("%064x", n+1)}
	}
	f := file(0)
	if _, err = tx.Exec(ctx, `INSERT INTO tax_bills(county_id,source_name,parcel_id,tax_year,total_taxes,source_url,file_sha256,fetched_at) VALUES('union',$1,$2,2025,123.45,$3,decode($4,'hex'),'2026-01-02')`, f[0], parcel, f[1], f[2]); err != nil {
		t.Fatal(err)
	}
	f = file(1)
	for year, balance := range map[int]string{2023: "0.00", 2024: "12.34", 2025: "56.78"} {
		if _, err = tx.Exec(ctx, `INSERT INTO tax_delinquent_balances(county_id,source_name,parcel_id,tax_year,balance_due,primary_source,source_url,file_sha256,fetched_at) VALUES('union',$1,$2,$3,$4,'RE',$5,decode($6,'hex'),'2026-01-02')`, f[0], parcel, year, balance, f[1], f[2]); err != nil {
			t.Fatal(err)
		}
	}
	f = file(2)
	if _, err = tx.Exec(ctx, `INSERT INTO tax_foreclosure_pipeline(county_id,source_name,parcel_id,case_number,status,redeemed,source_url,file_sha256,fetched_at) VALUES('union',$1,$2,'PIPE','listed',true,$3,decode($4,'hex'),'2026-01-02')`, f[0], parcel, f[1], f[2]); err != nil {
		t.Fatal(err)
	}
	f = file(3)
	var caseID string
	if err = tx.QueryRow(ctx, `INSERT INTO tax_foreclosure_cases(county_id,source_name,case_number,parcel_set_sha256,legal_sha256,status,redeemed,bidding_closed,source_url,file_sha256,fetched_at) VALUES('union',$1,'CASE',decode(repeat('01',32),'hex'),decode(repeat('02',32),'hex'),'closed',true,true,$2,decode($3,'hex'),'2026-01-02') RETURNING id::text`, f[0], f[1], f[2]).Scan(&caseID); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO tax_foreclosure_parcels(case_id,parcel_id) VALUES($1,$2)`, caseID, parcel); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	defer func() {
		pool.Exec(context.Background(), "DELETE FROM tax_file_imports WHERE source_name=ANY($1)", names)
		pool.Exec(context.Background(), "DELETE FROM tax_foreclosure_cases WHERE id=$1", caseID)
	}()
	get := func(id string) map[string]any {
		t.Helper()
		w := httptest.NewRecorder()
		New(pool).ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/parcels/"+id+"/tax", nil))
		if w.Code != 200 {
			t.Fatalf("tax status %d", w.Code)
		}
		var out map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	tax := get(parcel)
	if tax["reported_delinquent_total"] != "69.12" || tax["delinquent_year_count"] != float64(3) {
		t.Fatalf("wrong balance %v", tax)
	}
	for _, key := range []string{"bills", "delinquent_balances", "pipeline", "foreclosure_cases"} {
		rows := tax[key].([]any)
		if len(rows) == 0 {
			t.Fatalf("missing %s", key)
		}
		for _, row := range rows {
			v := row.(map[string]any)
			if v["source_url"] == nil || v["sha256"] == nil || v["fetched_at"] == nil {
				t.Fatalf("missing identity %s %v", key, v)
			}
		}
	}
	if tax["pipeline"].([]any)[0].(map[string]any)["redeemed"] != true || tax["foreclosure_cases"].([]any)[0].(map[string]any)["bidding_closed"] != true {
		t.Fatal("foreclosure status lost")
	}
	if _, ok := tax["pipeline"].([]any)[0].(map[string]any)["opening_bid"]; ok {
		t.Fatal("pipeline mislabeled as bid")
	}
	if get(empty)["reported_delinquent_total"] != nil {
		t.Fatal("missing evidence became zero")
	}
}
