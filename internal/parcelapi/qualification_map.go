package parcelapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

type qualificationFeature struct {
	Type       string          `json:"type"`
	Geometry   json.RawMessage `json:"geometry"`
	Properties struct {
		Kind string `json:"kind"`
		Name string `json:"name"`
	} `json:"properties"`
}

func (h *Handler) qualificationMap(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if err := parse(q, "bbox", "zoom", "layer"); err != nil {
		bad(w, err)
		return
	}
	b, err := parseBounds(q.Get("bbox"))
	if err != nil {
		bad(w, err)
		return
	}
	group := q.Get("layer")
	minZoom, query, dimension := 9, zoneViewport, 3
	switch group {
	case "zoning":
	case "roads":
		minZoom, query, dimension = 13, roadViewport, 2
	default:
		bad(w, errors.New("layer must be zoning or roads"))
		return
	}
	zoom, err := strconv.Atoi(q.Get("zoom"))
	maxSpan := 3.0
	if group == "roads" {
		maxSpan = 0.25
	}
	if err != nil || zoom < minZoom || zoom > 22 || b.East-b.West > maxSpan || b.North-b.South > maxSpan {
		bad(w, errors.New("zoom or bounds outside layer limits"))
		return
	}
	ctx := r.Context()
	tx, err := h.tx(ctx)
	if err != nil {
		unavailable(w)
		return
	}
	defer tx.Rollback(context.Background())
	type generation struct {
		ID              string     `json:"id"`
		FetchedAt       time.Time  `json:"fetched_at"`
		RefreshFailedAt *time.Time `json:"refresh_failed_at"`
	}
	var gen generation
	err = tx.QueryRow(ctx, `SELECT g.id::text,g.fetched_at,f.failed_at FROM qualification_heads h JOIN qualification_generations g ON g.id=h.generation_id AND g.source_group=h.source_group LEFT JOIN qualification_refresh_failures f ON f.source_group=h.source_group WHERE h.source_group=$1 AND g.completed_at IS NOT NULL`, group).Scan(&gen.ID, &gen.FetchedAt, &gen.RefreshFailedAt)
	features := []qualificationFeature{}
	if errors.Is(err, pgx.ErrNoRows) {
		respond(w, map[string]any{"type": "FeatureCollection", "features": features, "generation": nil})
		return
	}
	if err != nil {
		unavailable(w)
		return
	}
	rows, err := tx.Query(ctx, viewportBounds+query+viewportGeometry, gen.ID, b.West, b.South, b.East, b.North, zoom, dimension)
	if err != nil {
		unavailable(w)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var f qualificationFeature
		f.Type = "Feature"
		if err := rows.Scan(&f.Properties.Kind, &f.Properties.Name, &f.Geometry); err != nil {
			unavailable(w)
			return
		}
		features = append(features, f)
		if len(features) > 500 {
			http.Error(w, "Too many features; zoom in", http.StatusRequestEntityTooLarge)
			return
		}
	}
	if rows.Err() != nil {
		unavailable(w)
		return
	}
	respond(w, map[string]any{"type": "FeatureCollection", "features": features, "generation": gen})
}

const viewportBounds = `WITH bounds AS (SELECT ST_Transform(ST_MakeEnvelope($2,$3,$4,$5,4326),3857) AS shape), hits AS MATERIALIZED (`
const zoneViewport = `
 SELECT 'zoning'::text AS kind, COALESCE(z.district_code,'') AS label,z.shape FROM qualification_zones z CROSS JOIN bounds b
 WHERE z.generation_id=$1 AND z.shape && b.shape AND ST_Intersects(z.shape,b.shape)
 UNION ALL
 SELECT 'jurisdiction',j.name,j.shape FROM qualification_jurisdictions j CROSS JOIN bounds b
 WHERE j.generation_id=$1 AND j.shape && b.shape AND ST_Intersects(j.shape,b.shape)
 LIMIT 501`
const roadViewport = `
 SELECT 'roads'::text AS kind,COALESCE(r.name,'') AS label,r.shape FROM qualification_roads r CROSS JOIN bounds b
 WHERE r.generation_id=$1 AND r.shape && b.shape AND ST_Intersects(r.shape,b.shape)
 LIMIT 501`
const viewportGeometry = `), clipped AS MATERIALIZED (
 SELECT kind,label,ST_CollectionExtract(ST_Intersection(h.shape,b.shape),$7::int) AS shape FROM hits h CROSS JOIN bounds b
) SELECT kind,label,ST_AsGeoJSON(ST_Transform(ST_SimplifyPreserveTopology(shape,GREATEST(0.1,50.0 / power(2.0,$6::int-9))),4326),7)::jsonb FROM clipped WHERE NOT ST_IsEmpty(shape)`
