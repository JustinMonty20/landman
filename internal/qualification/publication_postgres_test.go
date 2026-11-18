//go:build postgres

package qualification

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/JustinMonty20/landman/internal/connector/union"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestQualificationGroupPublicationAndRetirement(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("requires disposable PostGIS DATABASE_URL")
	}
	ctx := context.Background()
	pool, e := pgxpool.New(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(pool.Close)
	store := Store{Pool: pool}
	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM qualification_heads WHERE source_group='roads'`)
		pool.Exec(ctx, `DELETE FROM qualification_refresh_failures WHERE source_group='roads'`)
		pool.Exec(ctx, `DELETE FROM qualification_layer_receipts WHERE layer_id=170`)
		pool.Exec(ctx, `DELETE FROM qualification_roads WHERE layer_id=170`)
		pool.Exec(ctx, `DELETE FROM qualification_generations WHERE source_group='roads'`)
	})
	meta, e := os.ReadFile(filepath.Join("..", "connector", "union", "testdata", "qualification", "170-metadata.json"))
	if e != nil {
		t.Fatal(e)
	}
	sample, e := os.ReadFile(filepath.Join("..", "connector", "union", "testdata", "qualification", "170-sample.json"))
	if e != nil {
		t.Fatal(e)
	}
	var response struct {
		Features         []json.RawMessage `json:"features"`
		SpatialReference json.RawMessage   `json:"spatialReference"`
	}
	if e = json.Unmarshal(sample, &response); e != nil {
		t.Fatal(e)
	}
	var phase atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := phase.Load()
		if strings.HasSuffix(r.URL.Path, "/170") {
			w.Write(meta)
			return
		}
		if r.URL.Query().Get("returnCountOnly") == "true" {
			count := 2
			if p == 2 {
				count = 1
			}
			fmt.Fprintf(w, `{"count":%d}`, count)
			return
		}
		if p == 1 {
			fmt.Fprint(w, `{"error":{"code":403,"message":"source changed"}}`)
			return
		}
		features := response.Features
		if p == 2 {
			features = features[:1]
		}
		json.NewEncoder(w).Encode(map[string]any{"features": features, "spatialReference": response.SpatialReference})
	}))
	defer server.Close()
	cfg := union.DefaultGISSourceConfig()
	cfg.BaseURL = server.URL + "/server/rest/services/OperationalLayers/MapServer/215/query"
	cfg.RateLimit = 1000
	source, e := union.NewGISSource(cfg)
	if e != nil {
		t.Fatal(e)
	}
	n, e := store.Refresh(ctx, source, union.QualificationRoads)
	if e != nil || n != 2 {
		t.Fatalf("initial: %d %v", n, e)
	}
	var receiptSRID int
	var receiptName, receiptURL string
	e = pool.QueryRow(ctx, `SELECT source_srid,source_metadata->>'name',service_url FROM qualification_layer_receipts WHERE layer_id=170 ORDER BY generation_id DESC LIMIT 1`).Scan(&receiptSRID, &receiptName, &receiptURL)
	if e != nil || receiptSRID != 102100 || receiptName != "Road Centerlines" || !strings.HasSuffix(receiptURL, "/170") {
		t.Fatalf("source metadata: srid=%d name=%q url=%q err=%v", receiptSRID, receiptName, receiptURL, e)
	}
	var first, head string
	var count int
	e = pool.QueryRow(ctx, `SELECT generation_id::text FROM qualification_heads WHERE source_group='roads'`).Scan(&first)
	if e != nil {
		t.Fatal(e)
	}
	phase.Store(1)
	_, e = store.Refresh(ctx, source, union.QualificationRoads)
	if e == nil {
		t.Fatal("invalid response published")
	}
	e = pool.QueryRow(ctx, `SELECT h.generation_id::text,(SELECT count(*) FROM qualification_refresh_failures WHERE source_group='roads') FROM qualification_heads h WHERE source_group='roads'`).Scan(&head, &count)
	if e != nil || head != first || count != 1 {
		t.Fatalf("failed refresh changed head=%s failure=%d err=%v", head, count, e)
	}
	phase.Store(2)
	n, e = store.Refresh(ctx, source, union.QualificationRoads)
	if e != nil || n != 1 {
		t.Fatalf("retire: %d %v", n, e)
	}
	e = pool.QueryRow(ctx, `SELECT h.generation_id::text,(SELECT count(*) FROM qualification_roads WHERE generation_id=h.generation_id) FROM qualification_heads h WHERE source_group='roads'`).Scan(&head, &count)
	if e != nil || head == first || count != 1 {
		t.Fatalf("retire head=%s active=%d err=%v", head, count, e)
	}
	e = pool.QueryRow(ctx, `SELECT count(*) FROM qualification_roads WHERE generation_id=$1`, first).Scan(&count)
	if e != nil || count != 2 {
		t.Fatalf("retained old generation: %d %v", count, e)
	}
	e = pool.QueryRow(ctx, `SELECT count(*) FROM qualification_refresh_failures WHERE source_group='roads'`).Scan(&count)
	if e != nil || count != 1 {
		t.Fatalf("failure observation not retained: %d %v", count, e)
	}
	readTx, e := pool.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer readTx.Rollback(ctx)
	fresh, e := unavailable(ctx, readTx, union.QualificationRoads)
	if e != nil || fresh.RefreshFailedAt != nil {
		t.Fatalf("superseded failure still active: %+v %v", fresh, e)
	}
}
