//go:build perf

package auth

import (
	"context"
	"encoding/json"
	"math"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

const vacancyTierTruth = `COALESCE((SELECT a.tier FROM vacancy_assessments a
 JOIN parcel_evidence_current c ON c.county_id=p.county_id AND c.source_name=p.source_name AND c.source_parcel_id=p.source_parcel_id
 JOIN vacancy_live_validation sv ON sv.generation_id=a.structure_generation AND sv.source_name='union_structures_171'
 JOIN vacancy_live_generations sg ON sg.id=sv.generation_id
 JOIN vacancy_latest_observations sh ON sh.source_name=sg.source_name AND sh.capture_id=sg.capture_id
 JOIN vacancy_live_validation av ON av.generation_id=a.assessment_generation AND av.source_name='union_assessed_value_0'
 JOIN vacancy_live_generations ag ON ag.id=av.generation_id
 JOIN vacancy_latest_observations ah ON ah.source_name=ag.source_name AND ah.capture_id=ag.capture_id
 WHERE a.parcel_id=p.id AND a.parcel_generation=c.observation_id::text || ':' || c.normalized_hash || ':' || c.geometry_hash
 AND sv.normalized_sha256=sg.normalized_sha256 AND av.normalized_sha256=ag.normalized_sha256),'unknown')`

func nearestRank95(n int) int { return int(math.Ceil(.95*float64(n))) - 1 }

func TestNearestRank95(t *testing.T) {
	for _, tc := range []struct{ n, want int }{{1, 0}, {5, 4}, {20, 18}, {21, 19}} {
		if got := nearestRank95(tc.n); got != tc.want {
			t.Fatalf("n=%d index=%d want=%d", tc.n, got, tc.want)
		}
	}
}

func verifyCopiedResponse(t *testing.T, pool *pgxpool.Pool, label, path string, body, first []byte) {
	t.Helper()
	u, err := url.Parse(path)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	switch {
	case label == "tax":
		var got struct {
			Balances []struct {
				Year   int    `json:"year"`
				Amount string `json:"balance_due"`
			} `json:"delinquent_balances"`
			Total *string `json:"reported_delinquent_total"`
			Years int64   `json:"delinquent_year_count"`
		}
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatal(err)
		}
		id := strings.TrimSuffix(strings.TrimPrefix(u.Path, "/api/v1/parcels/"), "/tax")
		rows, err := pool.Query(context.Background(), `SELECT d.tax_year,d.balance_due::text FROM tax_delinquent_balances d JOIN tax_file_imports i ON i.active AND i.county_id=d.county_id AND i.source_name=d.source_name AND i.source_url=d.source_url AND i.file_sha256=d.file_sha256 AND i.fetched_at=d.fetched_at WHERE d.parcel_id=$1 ORDER BY d.tax_year`, id)
		if err != nil {
			t.Fatal(err)
		}
		var expected []struct {
			Year   int
			Amount string
		}
		unique := map[int]bool{}
		for rows.Next() {
			var y int
			var amount string
			if err := rows.Scan(&y, &amount); err != nil {
				t.Fatal(err)
			}
			expected = append(expected, struct {
				Year   int
				Amount string
			}{y, amount})
			unique[y] = true
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		rows.Close()
		if len(got.Balances) != len(expected) || got.Years != int64(len(unique)) || got.Total == nil {
			t.Fatal("active tax identity/count missing")
		}
		for i, b := range got.Balances {
			if b.Year != expected[i].Year || b.Amount != expected[i].Amount {
				t.Fatal("active tax year/value mismatch")
			}
		}
		var total *string
		if err := pool.QueryRow(context.Background(), `SELECT sum(d.balance_due)::text FROM tax_delinquent_balances d JOIN tax_file_imports i ON i.active AND i.county_id=d.county_id AND i.source_name=d.source_name AND i.source_url=d.source_url AND i.file_sha256=d.file_sha256 AND i.fetched_at=d.fetched_at WHERE d.parcel_id=$1`, id).Scan(&total); err != nil || total == nil || *total != *got.Total {
			t.Fatal("active tax total mismatch")
		}
	case label == "soil":
		var got struct {
			ParcelID  string `json:"parcel_id"`
			RunID     string `json:"interpretation_set_revision"`
			Available bool   `json:"available"`
			Uses      []struct {
				Use      string `json:"use"`
				MapUnits []struct {
					MapUnitKey string `json:"map_unit_key"`
				} `json:"map_units"`
			} `json:"uses"`
		}
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatal(err)
		}
		var runID string
		if err := pool.QueryRow(context.Background(), `SELECT a.run_id::text FROM soil_parcel_assessments a JOIN soil_screening_heads h ON h.run_id=a.run_id JOIN parcels p ON p.id=a.parcel_id AND p.county_id=h.county_id AND p.source_name=h.source_name WHERE a.parcel_id=$1`, got.ParcelID).Scan(&runID); err != nil || !got.Available || got.RunID != runID {
			t.Fatalf("published soil run mismatch: %v", err)
		}
		rows, err := pool.Query(context.Background(), `SELECT r.provenance->>'use',m.map_unit_key FROM soil_assessment_map_units m JOIN soil_screening_ratings r ON r.run_id=m.run_id AND r.map_unit_key=m.map_unit_key WHERE m.run_id=$1 AND m.parcel_id=$2`, runID, got.ParcelID)
		if err != nil {
			t.Fatal(err)
		}
		expected := map[string][]string{}
		for rows.Next() {
			var use, key string
			if err := rows.Scan(&use, &key); err != nil {
				t.Fatal(err)
			}
			expected[use] = append(expected[use], key)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		rows.Close()
		if len(got.Uses) != 3 {
			t.Fatalf("soil uses=%d expected=3", len(got.Uses))
		}
		for _, use := range got.Uses {
			keys := expected[use.Use]
			if len(keys) != len(use.MapUnits) {
				t.Fatalf("soil use=%s units=%d expected=%d", use.Use, len(use.MapUnits), len(keys))
			}
			sort.Strings(keys)
			actual := make([]string, 0, len(use.MapUnits))
			for _, unit := range use.MapUnits {
				actual = append(actual, unit.MapUnitKey)
			}
			sort.Strings(actual)
			for i := range keys {
				if keys[i] != actual[i] {
					t.Fatalf("soil use=%s unit mismatch", use.Use)
				}
			}
		}
	case strings.HasPrefix(label, "list-"):
		var got struct {
			Items []struct {
				ID string `json:"id"`
			} `json:"items"`
			Count int64 `json:"count"`
		}
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatal(err)
		}
		where := `p.county_id='union' AND p.source_name='union_county_gis' AND ` + vacancyTierTruth + `='unknown'`
		if q.Has("min_acres") {
			where += ` AND p.mapped_acres>=1`
		}
		if q.Has("delinquent") {
			exists := `EXISTS (SELECT 1 FROM tax_delinquent_balances d JOIN tax_file_imports i ON i.active AND i.county_id=d.county_id AND i.source_name=d.source_name AND i.source_url=d.source_url AND i.file_sha256=d.file_sha256 AND i.fetched_at=d.fetched_at WHERE d.parcel_id=p.id AND d.balance_due>0)`
			if q.Get("delinquent") == "false" {
				where += ` AND NOT ` + exists
			} else {
				where += ` AND ` + exists
			}
		}
		var expected int64
		if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM parcels p WHERE `+where).Scan(&expected); err != nil {
			t.Fatal(err)
		}
		if got.Count != expected {
			t.Fatalf("%s count got=%d expected=%d", label, got.Count, expected)
		}
		after := "00000000-0000-0000-0000-000000000000"
		if label == "list-later" {
			var previous struct {
				Items []struct {
					ID string `json:"id"`
				} `json:"items"`
			}
			if err := json.Unmarshal(first, &previous); err != nil || len(previous.Items) == 0 {
				t.Fatal("invalid first page")
			}
			after = previous.Items[len(previous.Items)-1].ID
		}
		rows, err := pool.Query(context.Background(), `SELECT p.id::text FROM parcels p WHERE `+where+` AND p.id>$1::uuid ORDER BY p.id LIMIT 20`, after)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		ids := []string{}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				t.Fatal(err)
			}
			ids = append(ids, id)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		if len(ids) != len(got.Items) {
			t.Fatalf("%s items=%d expected=%d", label, len(got.Items), len(ids))
		}
		for i, item := range got.Items {
			if item.ID != ids[i] {
				t.Fatalf("%s id mismatch at %d", label, i)
			}
		}
	case strings.HasPrefix(label, "grid") || label == "shape":
		var got struct {
			Mode    string `json:"mode"`
			Context int64  `json:"context_count"`
			Matches int64  `json:"match_count"`
			Cells   []struct {
				X       int   `json:"x"`
				Y       int   `json:"y"`
				Count   int64 `json:"count"`
				Matches int64 `json:"matches"`
			} `json:"cells"`
			Features []struct {
				ID    string `json:"id"`
				Match bool   `json:"match"`
			} `json:"features"`
		}
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatal(err)
		}
		parts := strings.Split(q.Get("bbox"), ",")
		if len(parts) != 4 {
			t.Fatal("invalid bbox")
		}
		args := make([]any, 4)
		for i, p := range parts {
			v, e := strconv.ParseFloat(p, 64)
			if e != nil {
				t.Fatal(e)
			}
			args[i] = v
		}
		truth := `SELECT p.id::text,(` + vacancyTierTruth + `='unknown'),ST_X(ST_Transform(ST_PointOnSurface(p.shape),4326)),ST_Y(ST_Transform(ST_PointOnSurface(p.shape),4326)) FROM parcels p WHERE p.county_id='union' AND p.source_name='union_county_gis' AND ST_Intersects(p.shape,ST_Transform(ST_MakeEnvelope($1,$2,$3,$4,4326),3857)) ORDER BY p.id`
		rows, e := pool.Query(context.Background(), truth, args...)
		if e != nil {
			t.Fatal(e)
		}
		type cellKey struct{ x, y int }
		cells := map[cellKey][2]int64{}
		matchIDs := map[string]bool{}
		ids := []string{}
		var matches int64
		west, south, east, north := args[0].(float64), args[1].(float64), args[2].(float64), args[3].(float64)
		for rows.Next() {
			var id string
			var match bool
			var x, y float64
			if e := rows.Scan(&id, &match, &x, &y); e != nil {
				rows.Close()
				t.Fatal(e)
			}
			ids = append(ids, id)
			matchIDs[id] = match
			if match {
				matches++
			}
			cx := int(math.Max(0, math.Min(15, math.Floor((x-west)*16/(east-west)))))
			cy := int(math.Max(0, math.Min(15, math.Floor((y-south)*16/(north-south)))))
			key := cellKey{cx, cy}
			value := cells[key]
			value[0]++
			if match {
				value[1]++
			}
			cells[key] = value
		}
		if e := rows.Err(); e != nil {
			t.Fatal(e)
		}
		rows.Close()
		if got.Context != int64(len(ids)) || got.Matches != matches {
			t.Fatalf("%s context=%d/%d matches=%d/%d", label, got.Context, len(ids), got.Matches, matches)
		}
		if got.Mode == "grid" {
			var total, matched int64
			for _, c := range got.Cells {
				key := cellKey{c.X, c.Y}
				v, ok := cells[key]
				if !ok || v[0] != c.Count || v[1] != c.Matches {
					t.Fatalf("%s grid cell mismatch %v", label, key)
				}
				delete(cells, key)
				total += c.Count
				matched += c.Matches
			}
			if len(cells) != 0 || total != got.Context || matched != got.Matches {
				t.Fatalf("%s grid sums mismatch", label)
			}
		} else {
			if len(got.Features) != len(ids) {
				t.Fatalf("%s feature count mismatch", label)
			}
			sort.Strings(ids)
			for i, f := range got.Features {
				if f.ID != ids[i] || f.Match != matchIDs[f.ID] {
					t.Fatalf("%s feature mismatch %d", label, i)
				}
			}
		}
	}
}
