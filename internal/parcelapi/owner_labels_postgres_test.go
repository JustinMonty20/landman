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

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestOwnerLabelsAPI(t *testing.T) {
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
		t.Fatal("refusing non-disposable API database")
	}
	ctx := context.Background()
	var id string
	err = pool.QueryRow(ctx, `INSERT INTO parcels(county_id,county_code,source_name,source_parcel_id,parcel_number,shape) VALUES('union','union','union_county_gis',$1,'LABELTEST',ST_Transform(ST_MakeEnvelope(-79.9,33.1,-79.899,33.101,4326),3857)) RETURNING id::text`, fmt.Sprintf("owner-api-%d", time.Now().UnixNano())).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM parcels WHERE id=$1`, id)
	_, err = pool.Exec(ctx, `INSERT INTO parcel_owner_labels(parcel_id,out_of_state_mailing,entity_name_indicator,corporate_name_indicator,mailing_state,matched_suffix,matched_owner_line,classified_at) VALUES($1,'yes','matched','not_detected','SC','LLC',1,now())`, id)
	if err != nil {
		t.Fatal(err)
	}
	request := func(path string) (int, map[string]any) {
		t.Helper()
		w := httptest.NewRecorder()
		New(pool).ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		var body map[string]any
		if w.Code == 200 {
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
		}
		return w.Code, body
	}
	if code, body := request("/api/v1/parcels/" + id); code != 200 || body["owner_labels"].(map[string]any)["matched_suffix"] != "LLC" || body["source_parcel_id"] == nil {
		t.Fatalf("detail %d %v", code, body)
	}
	for _, path := range []string{"/api/v1/parcels?out_of_state_mailing=yes&entity_name_indicator=matched", "/api/v1/parcels?corporate_name_indicator=not_detected"} {
		if code, body := request(path); code != 200 || body["count"].(float64) < 1 {
			t.Fatalf("list %s %d %v", path, code, body)
		}
	}
	if code, body := request("/api/v1/parcels/map?bbox=-79.901,33.099,-79.898,33.102&zoom=14&out_of_state_mailing=yes"); code != 200 || body["match_count"].(float64) < 1 {
		t.Fatalf("map %d %v", code, body)
	}
	var secondID string
	err = pool.QueryRow(ctx, `INSERT INTO parcels(county_id,county_code,source_name,source_parcel_id,parcel_number) VALUES('union','union','union_county_gis',$1,'LABELTEST2') RETURNING id::text`, fmt.Sprintf("owner-api-next-%d", time.Now().UnixNano())).Scan(&secondID)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM parcels WHERE id=$1`, secondID)
	_, err = pool.Exec(ctx, `INSERT INTO parcel_owner_labels(parcel_id,out_of_state_mailing,entity_name_indicator,corporate_name_indicator,mailing_state,matched_suffix,matched_owner_line,classified_at) VALUES($1,'yes','matched','not_detected','SC','LLC',1,now())`, secondID)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Rollback(context.Background())
	var snapshotRevision, sameSnapshotRevision, committedRevision int64
	if err := reader.QueryRow(ctx, `SELECT revision FROM parcel_owner_label_revision WHERE id`).Scan(&snapshotRevision); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE parcel_owner_labels SET classified_at=now()+interval '30 minutes' WHERE parcel_id=$1`, secondID); err != nil {
		t.Fatal(err)
	}
	if err := reader.QueryRow(ctx, `SELECT revision FROM parcel_owner_label_revision WHERE id`).Scan(&sameSnapshotRevision); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT revision FROM parcel_owner_label_revision WHERE id`).Scan(&committedRevision); err != nil {
		t.Fatal(err)
	}
	if sameSnapshotRevision != snapshotRevision || committedRevision != snapshotRevision+1 {
		t.Fatalf("revision not snapshot-visible: %d %d %d", snapshotRevision, sameSnapshotRevision, committedRevision)
	}
	if err := reader.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	code, page := request("/api/v1/parcels?out_of_state_mailing=yes&page_size=1")
	if code != 200 || page["next_cursor"] == nil {
		t.Fatalf("owner cursor %d %v", code, page)
	}
	cursor := page["next_cursor"].(string)
	_, err = pool.Exec(ctx, `UPDATE parcel_owner_labels SET classified_at=now()+interval '1 hour' WHERE parcel_id=$1`, secondID)
	if err != nil {
		t.Fatal(err)
	}
	if code, _ = request("/api/v1/parcels?out_of_state_mailing=yes&page_size=1&cursor=" + cursor); code != 400 {
		t.Fatalf("owner backfill failed to invalidate cursor: %d", code)
	}
	older, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer older.Rollback(context.Background())
	if _, err = older.Exec(ctx, `UPDATE parcel_owner_labels SET out_of_state_mailing='no', mailing_state='NC' WHERE parcel_id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE parcel_owner_labels SET classified_at=now()+interval '2 hours' WHERE parcel_id=$1`, secondID); err != nil {
		t.Fatal(err)
	}
	code, page = request("/api/v1/parcels?out_of_state_mailing=yes&page_size=1")
	if code != 200 || page["next_cursor"] == nil {
		t.Fatalf("cursor before delayed commit %d %v", code, page)
	}
	cursor = page["next_cursor"].(string)
	if err = older.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if code, _ = request("/api/v1/parcels?out_of_state_mailing=yes&page_size=1&cursor=" + cursor); code != 400 {
		t.Fatalf("out-of-order commit did not invalidate cursor: %d", code)
	}
	for _, path := range []string{"/api/v1/parcels?out_of_state_mailing=resident", "/api/v1/parcels/map?bbox=-79.901,33.099,-79.898,33.102&zoom=14&entity_name_indicator=person"} {
		if code, _ := request(path); code != 400 {
			t.Fatalf("accepted invalid filter %s: %d", path, code)
		}
	}
}
