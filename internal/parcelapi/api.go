// Package parcelapi serves bounded reads of published Union County evidence.
package parcelapi

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/JustinMonty20/landman/internal/county"
	"github.com/JustinMonty20/landman/internal/soil"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const countyID = "union"
const parcelSource = "union_county_gis"
const maxBytes = 1 << 20

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type Bounds struct{ West, South, East, North float64 }
type Filters struct {
	AcresMin               *float64
	AcresMax               *float64
	LikelyVacant           *bool
	Delinquent             *bool
	OutOfStateMailing      *string
	EntityNameIndicator    *string
	CorporateNameIndicator *string
	Bounds                 *Bounds
	PageSize               int
	After                  string
}
type cursor struct {
	After  string `json:"after"`
	Filter string `json:"filter"`
	Stamp  string `json:"stamp"`
}
type Parcel struct {
	ID           string    `json:"id"`
	ParcelNumber *string   `json:"parcel_number"`
	Address      *string   `json:"address"`
	MappedAcres  *string   `json:"mapped_acres"`
	LikelyVacant bool      `json:"likely_vacant"`
	UpdatedAt    time.Time `json:"updated_at"`
}
type Feature struct {
	Parcel
	Geometry json.RawMessage `json:"geometry"`
	Match    bool            `json:"match"`
}
type Handler struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) http.Handler { return &Handler{pool: pool} }

func parse(q url.Values, allowed ...string) error {
	keys := make(map[string]bool, len(allowed))
	for _, k := range allowed {
		keys[k] = true
	}
	for k, v := range q {
		if !keys[k] || len(v) != 1 {
			return fmt.Errorf("unknown or duplicate query key: %s", k)
		}
	}
	return nil
}
func finite(raw string, min, max float64) (float64, error) {
	v, e := strconv.ParseFloat(raw, 64)
	if e != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < min || v > max {
		return 0, errors.New("invalid number")
	}
	return v, nil
}
func parseBounds(raw string) (*Bounds, error) {
	a := strings.Split(raw, ",")
	if len(a) != 4 {
		return nil, errors.New("bbox requires west,south,east,north")
	}
	var b Bounds
	vals := []*float64{&b.West, &b.South, &b.East, &b.North}
	for i, s := range a {
		min, max := -180.0, 180.0
		if i%2 == 1 {
			min, max = -85.05112878, 85.05112878
		}
		v, e := finite(s, min, max)
		if e != nil {
			return nil, e
		}
		*vals[i] = v
	}
	if b.West >= b.East || b.South >= b.North {
		return nil, errors.New("bbox is not ordered")
	}
	return &b, nil
}
func parseFilters(q url.Values) (Filters, error) {
	return parseFilterValues(q, false)
}

func parseFilterValues(q url.Values, mapQuery bool) (Filters, error) {
	f := Filters{PageSize: 50}
	allowed := []string{"min_acres", "max_acres", "likely_vacant", "delinquent", "out_of_state_mailing", "entity_name_indicator", "corporate_name_indicator", "bbox", "page_size", "cursor"}
	if mapQuery {
		allowed = []string{"min_acres", "max_acres", "likely_vacant", "delinquent", "out_of_state_mailing", "entity_name_indicator", "corporate_name_indicator", "bbox", "zoom"}
	}
	if err := parse(q, allowed...); err != nil {
		return f, err
	}
	for _, item := range []struct {
		key    string
		target **float64
	}{{"min_acres", &f.AcresMin}, {"max_acres", &f.AcresMax}} {
		if raw := q.Get(item.key); raw != "" {
			v, e := finite(raw, 0, 1000000)
			if e != nil {
				return f, e
			}
			*item.target = &v
		} else if _, ok := q[item.key]; ok {
			return f, errors.New("empty filter")
		}
	}
	if f.AcresMin != nil && f.AcresMax != nil && *f.AcresMin > *f.AcresMax {
		return f, errors.New("acreage range reversed")
	}
	for _, item := range []struct {
		key    string
		target **bool
	}{{"likely_vacant", &f.LikelyVacant}, {"delinquent", &f.Delinquent}} {
		if raw, ok := q[item.key]; ok {
			v, e := strconv.ParseBool(raw[0])
			if e != nil || (raw[0] != "true" && raw[0] != "false") {
				return f, errors.New("invalid boolean")
			}
			*item.target = &v
		}
	}
	for _, item := range []struct {
		key    string
		target **string
		valid  []string
	}{
		{"out_of_state_mailing", &f.OutOfStateMailing, []string{"yes", "no", "unknown"}},
		{"entity_name_indicator", &f.EntityNameIndicator, []string{"matched", "not_detected", "unknown"}},
		{"corporate_name_indicator", &f.CorporateNameIndicator, []string{"matched", "not_detected", "unknown"}},
	} {
		if v, ok := q[item.key]; ok {
			good := false
			for _, allowed := range item.valid {
				if v[0] == allowed {
					good = true
				}
			}
			if !good {
				return f, errors.New("invalid owner label filter")
			}
			*item.target = &v[0]
		}
	}
	if raw, ok := q["bbox"]; ok {
		var e error
		f.Bounds, e = parseBounds(raw[0])
		if e != nil {
			return f, e
		}
	}
	if raw, ok := q["page_size"]; ok {
		n, e := strconv.Atoi(raw[0])
		if e != nil || n < 1 || n > 100 {
			return f, errors.New("page_size must be 1..100")
		}
		f.PageSize = n
	}
	if raw, ok := q["cursor"]; ok {
		if len(raw[0]) > 2048 || raw[0] == "" {
			return f, errors.New("invalid cursor")
		}
		f.After = raw[0]
	}
	return f, nil
}
func (f Filters) active() bool {
	return f.AcresMin != nil || f.AcresMax != nil || f.LikelyVacant != nil || f.Delinquent != nil || f.OutOfStateMailing != nil || f.EntityNameIndicator != nil || f.CorporateNameIndicator != nil
}
func (f Filters) digest() string {
	copy := f
	copy.After = ""
	copy.PageSize = 0
	b, _ := json.Marshal(copy)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func decodeCursor(raw string) (cursor, error) {
	var c cursor
	b, e := base64.RawURLEncoding.DecodeString(raw)
	if e != nil || len(b) > 1024 || json.Unmarshal(b, &c) != nil || !uuidPattern.MatchString(c.After) || len(c.Filter) != 64 || len(c.Stamp) != 64 {
		return c, errors.New("invalid cursor")
	}
	return c, nil
}
func encodeCursor(c cursor) string {
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}
func optFloat(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}
func optBool(p *bool) any {
	if p == nil {
		return nil
	}
	return *p
}
func optString(p *string) any {
	if p == nil {
		return nil
	}
	return *p
}
func (f Filters) args() []any {
	a := []any{countyID, parcelSource, optFloat(f.AcresMin), optFloat(f.AcresMax), optBool(f.LikelyVacant), optBool(f.Delinquent), nil, nil, nil, nil}
	if f.Bounds != nil {
		a[6] = f.Bounds.West
		a[7] = f.Bounds.South
		a[8] = f.Bounds.East
		a[9] = f.Bounds.North
	}
	a = append(a, optString(f.OutOfStateMailing), optString(f.EntityNameIndicator), optString(f.CorporateNameIndicator))
	return a
}

const predicate = `p.county_id=$1 AND p.source_name=$2
 AND ($3::float8 IS NULL OR p.mapped_acres >= $3)
 AND ($4::float8 IS NULL OR p.mapped_acres <= $4)
 AND ($5::boolean IS NULL OR p.likely_vacant=$5)
 AND ($6::boolean IS NULL OR EXISTS (SELECT 1 FROM tax_delinquent_balances d WHERE d.parcel_id=p.id AND d.balance_due>0 AND EXISTS (SELECT 1 FROM tax_file_imports i WHERE i.active AND i.county_id=d.county_id AND i.source_name=d.source_name) )=$6)
 AND ($11::text IS NULL OR EXISTS(SELECT 1 FROM parcel_owner_labels l WHERE l.parcel_id=p.id AND l.out_of_state_mailing=$11))
AND ($12::text IS NULL OR EXISTS(SELECT 1 FROM parcel_owner_labels l WHERE l.parcel_id=p.id AND l.entity_name_indicator=$12))
AND ($13::text IS NULL OR EXISTS(SELECT 1 FROM parcel_owner_labels l WHERE l.parcel_id=p.id AND l.corporate_name_indicator=$13))
AND ($7::float8 IS NULL OR (p.shape && ST_Transform(ST_MakeEnvelope($7,$8,$9,$10,4326),3857) AND ST_Intersects(p.shape,ST_Transform(ST_MakeEnvelope($7,$8,$9,$10,4326),3857))))`
const parcelColumns = `p.id::text,p.parcel_number,p.situs_line_1,p.mapped_acres::text,p.likely_vacant,p.updated_at`

func scanParcel(row pgx.Row) (Parcel, error) {
	var p Parcel
	err := row.Scan(&p.ID, &p.ParcelNumber, &p.Address, &p.MappedAcres, &p.LikelyVacant, &p.UpdatedAt)
	return p, err
}

func respond(w http.ResponseWriter, v any) {
	b, e := json.Marshal(v)
	if e != nil {
		http.Error(w, "Cannot encode response", 500)
		return
	}
	if len(b) > maxBytes {
		http.Error(w, "Result exceeds response limit; narrow the bounds", http.StatusRequestEntityTooLarge)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(b)
}
func bad(w http.ResponseWriter, e error) { http.Error(w, e.Error(), http.StatusBadRequest) }
func unavailable(w http.ResponseWriter) {
	http.Error(w, "Data unavailable", http.StatusServiceUnavailable)
}
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if _, err := url.ParseQuery(r.URL.RawQuery); err != nil {
		bad(w, errors.New("invalid query encoding"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	switch {
	case r.URL.Path == "/api/v1/parcels/lookup":
		h.lookup(w, r)
	case r.URL.Path == "/api/v1/parcels":
		h.list(w, r)
	case r.URL.Path == "/api/v1/parcels/map":
		h.mapView(w, r)
	case r.URL.Path == "/api/v1/flood/map":
		h.floodMap(w, r)
	case r.URL.Path == "/api/v1/qualification/map":
		h.qualificationMap(w, r)
	case r.URL.Path == "/api/v1/sources":
		h.sources(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/v1/parcels/"):
		h.detailRoute(w, r)
	default:
		http.NotFound(w, r)
	}
}
func (h *Handler) tx(ctx context.Context) (pgx.Tx, error) {
	tx, err := h.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, "SET LOCAL jit = off"); err != nil {
		tx.Rollback(context.Background())
		return nil, err
	}
	return tx, nil
}

// stamp identifies the source records visible in this transaction, not a global snapshot.
func stamp(ctx context.Context, tx pgx.Tx) (string, error) {
	var parcelTime *time.Time
	var parcelCount int64
	var files string
	err := tx.QueryRow(ctx, `SELECT updated_at FROM parcels WHERE county_id=$1 AND source_name=$2 ORDER BY updated_at DESC LIMIT 1`, countyID, parcelSource).Scan(&parcelTime)
	if errors.Is(err, pgx.ErrNoRows) {
		parcelTime = nil
	} else if err != nil {
		return "", err
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM parcels WHERE county_id=$1 AND source_name=$2`, countyID, parcelSource).Scan(&parcelCount); err != nil {
		return "", err
	}
	err = tx.QueryRow(ctx, `SELECT COALESCE(string_agg(source_name||':'||encode(file_sha256,'hex')||':'||fetched_at::text,',' ORDER BY source_name),'') FROM tax_file_imports WHERE county_id=$1 AND active`, countyID).Scan(&files)
	if err != nil {
		return "", err
	}
	var attempts string
	err = tx.QueryRow(ctx, `SELECT COALESCE(string_agg(job_id||':'||outcome||':'||started_at::text,',' ORDER BY job_id),'') FROM (SELECT DISTINCT ON (job_id) job_id,outcome,started_at FROM job_attempts WHERE job_id IN ('union.gis.parcels','union.gis.soil','union.soil.screening','union.tax') ORDER BY job_id,started_at DESC) latest`).Scan(&attempts)
	if err != nil {
		return "", err
	}
	var soilHeads string
	err = tx.QueryRow(ctx, `SELECT concat_ws(':',h.run_id::text,rs.failed_at::text,s.set_id::text,r.fresh_until::text) FROM soil_screening_heads h LEFT JOIN soil_polygon_set_heads s ON s.county_id=h.county_id AND s.source_name=h.source_name LEFT JOIN soil_screening_runs r ON r.id=h.run_id LEFT JOIN soil_screening_refresh_status rs ON rs.county_id=h.county_id AND rs.source_name=h.source_name WHERE h.county_id=$1 AND h.source_name=$2`, countyID, parcelSource).Scan(&soilHeads)
	if errors.Is(err, pgx.ErrNoRows) {
		soilHeads = ""
	} else if err != nil {
		return "", err
	}
	var labelRevision int64
	if err := tx.QueryRow(ctx, `SELECT revision FROM parcel_owner_label_revision WHERE id`).Scan(&labelRevision); err != nil {
		return "", err
	}
	h := sha256.Sum256([]byte(fmt.Sprint(parcelTime, parcelCount, files, attempts, soilHeads, labelRevision)))
	return hex.EncodeToString(h[:]), nil
}
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	f, e := parseFilters(r.URL.Query())
	if e != nil {
		bad(w, e)
		return
	}
	tx, e := h.tx(r.Context())
	if e != nil {
		unavailable(w)
		return
	}
	defer tx.Rollback(context.Background())
	s, e := stamp(r.Context(), tx)
	if e != nil {
		unavailable(w)
		return
	}
	after := "00000000-0000-0000-0000-000000000000"
	if f.After != "" {
		c, err := decodeCursor(f.After)
		if err != nil || c.Filter != f.digest() || c.Stamp != s {
			bad(w, errors.New("stale or invalid cursor"))
			return
		}
		after = c.After
	}
	items := []Parcel{}
	count := int64(0)
	next := ""
	if f.active() {
		e = tx.QueryRow(r.Context(), `SELECT count(*) FROM parcels p WHERE `+predicate, f.args()...).Scan(&count)
		if e != nil {
			unavailable(w)
			return
		}
		a := append(f.args(), after, f.PageSize+1)
		rows, err := tx.Query(r.Context(), `SELECT `+parcelColumns+` FROM parcels p WHERE `+predicate+` AND p.id>$14::uuid ORDER BY p.id LIMIT $15`, a...)
		if err != nil {
			unavailable(w)
			return
		}
		for rows.Next() {
			p, err := scanParcel(rows)
			if err != nil {
				rows.Close()
				unavailable(w)
				return
			}
			items = append(items, p)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			unavailable(w)
			return
		}
		if len(items) > f.PageSize {
			items = items[:f.PageSize]
			next = encodeCursor(cursor{items[len(items)-1].ID, f.digest(), s})
		}
	}
	if e = tx.Commit(r.Context()); e != nil {
		unavailable(w)
		return
	}
	respond(w, struct {
		Items       []Parcel `json:"items"`
		Count       int64    `json:"count"`
		NextCursor  string   `json:"next_cursor,omitempty"`
		SourceStamp string   `json:"source_stamp"`
	}{items, count, next, s})
}
func (h *Handler) lookup(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if e := parse(q, "q"); e != nil {
		bad(w, e)
		return
	}
	term := strings.TrimSpace(q.Get("q"))
	if len(term) < 2 || len(term) > 80 {
		bad(w, errors.New("q must be 2..80 bytes"))
		return
	}
	// Escape LIKE metacharacters so the cap applies to a literal prefix.
	term = strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(term, `\`, `\\`), `%`, `\%`), `_`, `\_`)
	tx, e := h.tx(r.Context())
	if e != nil {
		unavailable(w)
		return
	}
	defer tx.Rollback(context.Background())
	if _, e = tx.Exec(r.Context(), "SET LOCAL plan_cache_mode = force_custom_plan"); e != nil {
		unavailable(w)
		return
	}
	prefix := strings.ToLower(term) + "%"
	var found bool
	e = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM parcels WHERE county_id=$1 AND source_name=$2 AND lower(left(parcel_number,80)) LIKE $3) OR EXISTS(SELECT 1 FROM parcels WHERE county_id=$1 AND source_name=$2 AND lower(left(situs_line_1,80)) LIKE $3)`, countyID, parcelSource, prefix).Scan(&found)
	if e != nil {
		unavailable(w)
		return
	}
	if !found {
		if e = tx.Commit(r.Context()); e != nil {
			unavailable(w)
			return
		}
		respond(w, map[string]any{"items": []Parcel{}})
		return
	}
	rows, e := tx.Query(r.Context(), `SELECT `+parcelColumns+` FROM parcels p WHERE p.county_id=$1 AND p.source_name=$2 AND (lower(left(p.parcel_number,80)) LIKE $3 OR lower(left(p.situs_line_1,80)) LIKE $3) ORDER BY p.id LIMIT 20`, countyID, parcelSource, prefix)
	if e != nil {
		unavailable(w)
		return
	}
	items := []Parcel{}
	for rows.Next() {
		p, err := scanParcel(rows)
		if err != nil {
			rows.Close()
			unavailable(w)
			return
		}
		items = append(items, p)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		unavailable(w)
		return
	}
	if e = tx.Commit(r.Context()); e != nil {
		unavailable(w)
		return
	}
	respond(w, map[string]any{"items": items})
}
func (f Filters) mapMatch(args *[]any) string {
	parts := []string{}
	add := func(value any, column, op string) {
		*args = append(*args, value)
		parts = append(parts, fmt.Sprintf("%s %s $%d", column, op, len(*args)))
	}
	if f.AcresMin != nil {
		add(*f.AcresMin, "p.mapped_acres", ">=")
	}
	if f.AcresMax != nil {
		add(*f.AcresMax, "p.mapped_acres", "<=")
	}
	if f.LikelyVacant != nil {
		add(*f.LikelyVacant, "p.likely_vacant", "=")
	}
	if f.Delinquent != nil {
		*args = append(*args, *f.Delinquent)
		parts = append(parts, fmt.Sprintf(`EXISTS (SELECT 1 FROM tax_delinquent_balances d JOIN tax_file_imports i ON i.active AND i.county_id=d.county_id AND i.source_name=d.source_name WHERE d.parcel_id=p.id AND d.balance_due>0) = $%d`, len(*args)))
	}
	for _, item := range []struct {
		status *string
		column string
	}{{f.OutOfStateMailing, "out_of_state_mailing"}, {f.EntityNameIndicator, "entity_name_indicator"}, {f.CorporateNameIndicator, "corporate_name_indicator"}} {
		if item.status != nil {
			*args = append(*args, *item.status)
			parts = append(parts, fmt.Sprintf("EXISTS(SELECT 1 FROM parcel_owner_labels l WHERE l.parcel_id=p.id AND l.%s=$%d)", item.column, len(*args)))
		}
	}
	if len(parts) == 0 {
		return "false"
	}
	return "COALESCE((" + strings.Join(parts, " AND ") + "), false)"
}

func (h *Handler) mapView(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if e := parse(q, "bbox", "zoom", "min_acres", "max_acres", "likely_vacant", "delinquent", "out_of_state_mailing", "entity_name_indicator", "corporate_name_indicator"); e != nil {
		bad(w, e)
		return
	}
	if q.Get("bbox") == "" {
		bad(w, errors.New("bbox required"))
		return
	}
	z, e := strconv.Atoi(q.Get("zoom"))
	if e != nil || z < 0 || z > 22 {
		bad(w, errors.New("zoom must be 0..22"))
		return
	}
	f, e := parseFilterValues(q, true)
	if e != nil {
		bad(w, e)
		return
	}
	tx, e := h.tx(r.Context())
	if e != nil {
		unavailable(w)
		return
	}
	defer tx.Rollback(context.Background())
	s, e := stamp(r.Context(), tx)
	if e != nil {
		unavailable(w)
		return
	}
	b := f.Bounds
	contextArgs := []any{countyID, parcelSource, b.West, b.South, b.East, b.North}
	const contextWhere = `p.county_id=$1 AND p.source_name=$2 AND p.shape && ST_Transform(ST_MakeEnvelope($3,$4,$5,$6,4326),3857) AND ST_Intersects(p.shape,ST_Transform(ST_MakeEnvelope($3,$4,$5,$6,4326),3857))`
	var total, matches int64
	if z < 13 {
		type cell struct {
			X       int   `json:"x"`
			Y       int   `json:"y"`
			Count   int64 `json:"count"`
			Matches int64 `json:"matches"`
		}
		cells := []cell{}
		args := append([]any(nil), contextArgs...)
		match := f.mapMatch(&args)
		// Cell position is display-only; intersection and match use the original polygon.
		rows, err := tx.Query(r.Context(), `SELECT greatest(0,least(15,floor((ST_X(location)-$3)*16/($5-$3))))::int AS x,
 greatest(0,least(15,floor((ST_Y(location)-$4)*16/($6-$4))))::int AS y,
 count(*),count(*) FILTER (WHERE match)
 FROM (SELECT ST_Transform(ST_PointOnSurface(p.shape),4326) AS location, (`+match+`) AS match
 FROM parcels p WHERE `+contextWhere+`) located GROUP BY x,y ORDER BY x,y`, args...)
		if err != nil {
			unavailable(w)
			return
		}
		for rows.Next() {
			var c cell
			if err = rows.Scan(&c.X, &c.Y, &c.Count, &c.Matches); err != nil {
				rows.Close()
				unavailable(w)
				return
			}
			cells = append(cells, c)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			unavailable(w)
			return
		}
		for _, c := range cells {
			total += c.Count
			matches += c.Matches
		}
		if e = tx.Commit(r.Context()); e != nil {
			unavailable(w)
			return
		}
		respond(w, map[string]any{"mode": "grid", "cells": cells, "context_count": total, "match_count": matches, "source_stamp": s})
		return
	}
	if e = tx.QueryRow(r.Context(), `SELECT count(*) FROM parcels p WHERE `+contextWhere, contextArgs...).Scan(&total); e != nil {
		unavailable(w)
		return
	}
	if total > 200 {
		http.Error(w, "Too many features; narrow bounds or lower zoom", http.StatusRequestEntityTooLarge)
		return
	}
	args := append(append([]any(nil), contextArgs...), z)
	match := f.mapMatch(&args)
	rows, err := tx.Query(r.Context(), `SELECT `+parcelColumns+`,ST_AsGeoJSON(ST_Transform(ST_SimplifyPreserveTopology(p.shape, greatest(0.1, 50.0 / power(2.0,$7::float8-13))),4326))::jsonb, (`+match+`) FROM parcels p WHERE `+contextWhere+` ORDER BY p.id`, args...)
	if err != nil {
		unavailable(w)
		return
	}
	features := []Feature{}
	for rows.Next() {
		var item Feature
		var geometry []byte
		if err = rows.Scan(&item.ID, &item.ParcelNumber, &item.Address, &item.MappedAcres, &item.LikelyVacant, &item.UpdatedAt, &geometry, &item.Match); err != nil {
			rows.Close()
			unavailable(w)
			return
		}
		item.Geometry = geometry
		if item.Match {
			matches++
		}
		features = append(features, item)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		unavailable(w)
		return
	}
	if e = tx.Commit(r.Context()); e != nil {
		unavailable(w)
		return
	}
	respond(w, map[string]any{"mode": "features", "features": features, "context_count": total, "match_count": matches, "source_stamp": s})
}
func (h *Handler) detailRoute(w http.ResponseWriter, r *http.Request) {
	if e := parse(r.URL.Query()); e != nil {
		bad(w, e)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/parcels/")
	parts := strings.Split(path, "/")
	if len(parts) > 2 || !uuidPattern.MatchString(parts[0]) {
		http.NotFound(w, r)
		return
	}
	if len(parts) == 1 {
		h.detail(w, r, parts[0])
		return
	}
	switch parts[1] {
	case "tax":
		h.tax(w, r, parts[0])
	case "soil":
		h.soil(w, r, parts[0])
	case "flood":
		h.floodDetail(w, r, parts[0])
	case "vacancy":
		h.vacancy(w, r, parts[0])
	default:
		http.NotFound(w, r)
	}
}
func exists(ctx context.Context, tx pgx.Tx, id string) (bool, error) {
	var ok bool
	e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM parcels WHERE id=$1 AND county_id=$2 AND source_name=$3)`, id, countyID, parcelSource).Scan(&ok)
	return ok, e
}
func (h *Handler) detail(w http.ResponseWriter, r *http.Request, id string) {
	tx, e := h.tx(r.Context())
	if e != nil {
		unavailable(w)
		return
	}
	defer tx.Rollback(context.Background())
	var p Parcel
	var sourceID string
	var location []byte
	e = tx.QueryRow(r.Context(), `SELECT `+parcelColumns+`,p.source_parcel_id,ST_AsGeoJSON(ST_Transform(ST_PointOnSurface(p.shape),4326))::jsonb FROM parcels p WHERE p.id=$1 AND p.county_id=$2 AND p.source_name=$3`, id, countyID, parcelSource).Scan(&p.ID, &p.ParcelNumber, &p.Address, &p.MappedAcres, &p.LikelyVacant, &p.UpdatedAt, &sourceID, &location)
	if errors.Is(e, pgx.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if e != nil {
		unavailable(w)
		return
	}
	var labels []byte
	e = tx.QueryRow(r.Context(), `SELECT jsonb_build_object('out_of_state_mailing',out_of_state_mailing,'entity_name_indicator',entity_name_indicator,'corporate_name_indicator',corporate_name_indicator,'mailing_state',mailing_state,'matched_suffix',matched_suffix,'matched_owner_line',matched_owner_line,'classified_at',classified_at) FROM parcel_owner_labels WHERE parcel_id=$1`, id).Scan(&labels)
	if errors.Is(e, pgx.ErrNoRows) {
		labels = []byte("null")
	} else if e != nil {
		unavailable(w)
		return
	}
	var sales []byte
	e = tx.QueryRow(r.Context(), `SELECT COALESCE(jsonb_agg(jsonb_build_object('date',sale_date,'amount',amount::text,'deed_book',deed_book,'deed_page',deed_page) ORDER BY sequence),'[]'::jsonb) FROM parcel_sales WHERE parcel_id=$1`, id).Scan(&sales)
	if e != nil {
		unavailable(w)
		return
	}
	s, e := stamp(r.Context(), tx)
	if e != nil {
		unavailable(w)
		return
	}
	if e = tx.Commit(r.Context()); e != nil {
		unavailable(w)
		return
	}
	respond(w, map[string]any{"parcel": p, "county": countyID, "source": parcelSource, "source_parcel_id": sourceID, "location": json.RawMessage(location), "sales": json.RawMessage(sales), "owner_labels": json.RawMessage(labels), "source_stamp": s})
}
func (h *Handler) soil(w http.ResponseWriter, r *http.Request, id string) {
	// CurrentAssessment owns its own read-only snapshot and freshness logic.
	var ok bool
	e := h.pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM parcels WHERE id=$1 AND county_id=$2 AND source_name=$3)`, id, countyID, parcelSource).Scan(&ok)
	if e != nil {
		unavailable(w)
		return
	}
	if !ok {
		http.NotFound(w, r)
		return
	}
	a, e := soil.CurrentAssessment(r.Context(), h.pool, county.Union, parcelSource, id)
	if errors.Is(e, pgx.ErrNoRows) {
		respond(w, map[string]any{"available": false, "reason": "no published assessment"})
		return
	}
	if e != nil {
		unavailable(w)
		return
	}
	a.Freshness.RefreshError = nil
	respond(w, a)
}
func (h *Handler) sources(w http.ResponseWriter, r *http.Request) {
	if e := parse(r.URL.Query()); e != nil {
		bad(w, e)
		return
	}
	tx, e := h.tx(r.Context())
	if e != nil {
		unavailable(w)
		return
	}
	defer tx.Rollback(context.Background())
	var data []byte
	e = tx.QueryRow(r.Context(), `SELECT COALESCE(jsonb_agg(jsonb_build_object('source',source_name,'publication_version',publication_version,'source_url',source_url,'fetched_at',fetched_at,'sha256',encode(file_sha256,'hex'),'activated_at',activated_at) ORDER BY source_name),'[]'::jsonb) FROM tax_file_imports WHERE county_id=$1 AND active`, countyID).Scan(&data)
	if e != nil {
		unavailable(w)
		return
	}
	s, e := stamp(r.Context(), tx)
	if e != nil {
		unavailable(w)
		return
	}
	var parcelUpdated *time.Time
	if e = tx.QueryRow(r.Context(), `SELECT max(updated_at) FROM parcels WHERE county_id=$1 AND source_name=$2`, countyID, parcelSource).Scan(&parcelUpdated); e != nil {
		unavailable(w)
		return
	}
	var statuses []byte
	e = tx.QueryRow(r.Context(), `SELECT COALESCE(jsonb_agg(jsonb_build_object('job_id',job_id,'outcome',outcome,'started_at',started_at,'finished_at',finished_at) ORDER BY job_id),'[]'::jsonb) FROM (SELECT DISTINCT ON (job_id) job_id,outcome,started_at,finished_at FROM job_attempts WHERE job_id IN ('union.gis.parcels','union.gis.soil','union.soil.screening','union.tax') ORDER BY job_id,started_at DESC) latest`).Scan(&statuses)
	if e != nil {
		unavailable(w)
		return
	}
	var soilPublished []byte
	e = tx.QueryRow(r.Context(), `SELECT jsonb_build_object('soil_set_id',r.soil_set_id,'current_soil_set_id',s.set_id,'run_id',h.run_id,'fresh_until',r.fresh_until,'refresh_failed_at',COALESCE(h.refresh_failed_at,rs.failed_at),'geometry_set_changed',r.soil_set_id IS DISTINCT FROM s.set_id) FROM soil_screening_heads h LEFT JOIN soil_polygon_set_heads s ON s.county_id=h.county_id AND s.source_name=h.source_name LEFT JOIN soil_screening_runs r ON r.id=h.run_id LEFT JOIN soil_screening_refresh_status rs ON rs.county_id=h.county_id AND rs.source_name=h.source_name WHERE h.county_id=$1 AND h.source_name=$2`, countyID, parcelSource).Scan(&soilPublished)
	if errors.Is(e, pgx.ErrNoRows) {
		soilPublished = []byte("null")
	} else if e != nil {
		unavailable(w)
		return
	}
	var successes []byte
	e = tx.QueryRow(r.Context(), `SELECT COALESCE(jsonb_agg(jsonb_build_object('job_id',job_id,'outcome',outcome,'started_at',started_at,'finished_at',finished_at) ORDER BY job_id),'[]'::jsonb) FROM (SELECT DISTINCT ON (job_id) job_id,outcome,started_at,finished_at FROM job_attempts WHERE job_id IN ('union.gis.parcels','union.gis.soil','union.soil.screening','union.tax') AND outcome='success' AND (job_id <> 'union.gis.parcels' OR attempt_kind='full_scan') ORDER BY job_id,started_at DESC) last_success`).Scan(&successes)
	if e != nil {
		unavailable(w)
		return
	}
	if e = tx.Commit(r.Context()); e != nil {
		unavailable(w)
		return
	}
	respond(w, map[string]any{"county": countyID, "parcel_source": parcelSource, "parcel_updated_at": parcelUpdated, "active_tax_files": json.RawMessage(data), "import_status": json.RawMessage(statuses), "last_clean_success": json.RawMessage(successes), "published_soil": json.RawMessage(soilPublished), "source_stamp": s})
}
func (h *Handler) tax(w http.ResponseWriter, r *http.Request, id string) {
	tx, e := h.tx(r.Context())
	if e != nil {
		unavailable(w)
		return
	}
	defer tx.Rollback(context.Background())
	ok, e := exists(r.Context(), tx, id)
	if e != nil {
		unavailable(w)
		return
	}
	if !ok {
		http.NotFound(w, r)
		return
	}
	// File identity includes URL, digest and retrieval time; an old row from an inactive import is not current evidence.
	const active = `JOIN tax_file_imports i ON i.active AND i.county_id=t.county_id AND i.source_name=t.source_name`
	var bills, balances, pipeline, cases []byte
	var reportedTotal *string
	var delinquentYears int64
	e = tx.QueryRow(r.Context(), `SELECT COALESCE(jsonb_agg(jsonb_build_object('year',t.tax_year,'total_taxes',t.total_taxes::text,'source',t.source_name,'source_url',t.source_url,'fetched_at',t.fetched_at,'sha256',encode(t.file_sha256,'hex')) ORDER BY t.tax_year),'[]'::jsonb) FROM tax_bills t `+active+` WHERE t.parcel_id=$1`, id).Scan(&bills)
	if e != nil {
		unavailable(w)
		return
	}
	e = tx.QueryRow(r.Context(), `SELECT COALESCE(jsonb_agg(jsonb_build_object('year',t.tax_year,'balance_due',t.balance_due::text,'source',t.source_name,'source_url',t.source_url,'fetched_at',t.fetched_at,'sha256',encode(t.file_sha256,'hex')) ORDER BY t.tax_year),'[]'::jsonb),sum(t.balance_due)::text,count(DISTINCT t.tax_year) FROM tax_delinquent_balances t `+active+` WHERE t.parcel_id=$1`, id).Scan(&balances, &reportedTotal, &delinquentYears)
	if e != nil {
		unavailable(w)
		return
	}
	e = tx.QueryRow(r.Context(), `SELECT COALESCE(jsonb_agg(jsonb_build_object('case_number',t.case_number,'status',t.status,'sale_on',t.sale_on,'tax_value',t.tax_value::text,'redeemed',t.redeemed,'source',t.source_name,'source_url',t.source_url,'fetched_at',t.fetched_at,'sha256',encode(t.file_sha256,'hex'))),'[]'::jsonb) FROM tax_foreclosure_pipeline t `+active+` WHERE t.parcel_id=$1`, id).Scan(&pipeline)
	if e != nil {
		unavailable(w)
		return
	}
	e = tx.QueryRow(r.Context(), `SELECT COALESCE(jsonb_agg(jsonb_build_object('case_number',t.case_number,'status',t.status,'sale_on',t.sale_on,'opening_bid',t.opening_bid::text,'current_bid',t.current_bid::text,'upset_bid',t.upset_bid::text,'redeemed',t.redeemed,'bidding_closed',t.bidding_closed,'source',t.source_name,'source_url',t.source_url,'fetched_at',t.fetched_at,'sha256',encode(t.file_sha256,'hex'))),'[]'::jsonb) FROM tax_foreclosure_cases t JOIN tax_foreclosure_parcels fp ON fp.case_id=t.id `+active+` WHERE fp.parcel_id=$1 AND t.active`, id).Scan(&cases)
	if e != nil {
		unavailable(w)
		return
	}
	s, e := stamp(r.Context(), tx)
	if e != nil {
		unavailable(w)
		return
	}
	if e = tx.Commit(r.Context()); e != nil {
		unavailable(w)
		return
	}
	respond(w, map[string]any{"bills": json.RawMessage(bills), "delinquent_balances": json.RawMessage(balances), "reported_delinquent_total": reportedTotal, "delinquent_year_count": delinquentYears, "pipeline": json.RawMessage(pipeline), "foreclosure_cases": json.RawMessage(cases), "source_stamp": s, "note": "Missing evidence is not a verified zero or a payoff quote"})
}
