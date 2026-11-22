package parcelapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
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
	if err := parse(q, "bbox", "zoom", "layer", "overview", "jurisdiction"); err != nil {
		bad(w, err)
		return
	}
	overview, err := parseOverlayOverview(q)
	if err != nil {
		bad(w, err)
		return
	}
	group := q.Get("layer")
	minZoom, maxSpan := 9, 3.0
	viewport, county := zoneViewport, zoneOverview
	switch group {
	case "zoning":
	case "roads":
		minZoom, maxSpan = 13, 0.25
		viewport, county = roadViewport, roadOverview
	default:
		bad(w, errors.New("layer must be zoning or roads"))
		return
	}
	query := county
	var args []any
	if q.Has("jurisdiction") {
		key := q.Get("jurisdiction")
		if !overview || group != "zoning" || !validJurisdiction(key) {
			bad(w, errors.New("jurisdiction requires a zoning overview and a valid key"))
			return
		}
		query = jurisdictionOverview
		args = []any{key}
	} else if !overview {
		args, err = overlayViewportArgs(q, minZoom, maxSpan)
		if err != nil {
			bad(w, err)
			return
		}
		query = viewportBounds + viewport + viewportGeometry
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
	err = tx.QueryRow(ctx, `SELECT g.id::text,g.fetched_at,f.failed_at FROM qualification_heads h JOIN qualification_generations g ON g.id=h.generation_id AND g.source_group=h.source_group LEFT JOIN qualification_refresh_failures f ON f.source_group=h.source_group AND f.failed_at>=g.started_at WHERE h.source_group=$1 AND g.completed_at IS NOT NULL`, group).Scan(&gen.ID, &gen.FetchedAt, &gen.RefreshFailedAt)
	features := []qualificationFeature{}
	if errors.Is(err, pgx.ErrNoRows) {
		respond(w, map[string]any{"type": "FeatureCollection", "features": features, "generation": nil, "overview": overview})
		return
	}
	if err != nil {
		unavailable(w)
		return
	}
	rows, err := tx.Query(ctx, query, append([]any{gen.ID}, args...)...)
	if err != nil {
		unavailable(w)
		return
	}
	defer rows.Close()
	for candidates := 1; rows.Next(); candidates++ {
		if candidates > 500 {
			http.Error(w, "Too many features; zoom in", http.StatusRequestEntityTooLarge)
			return
		}
		var f qualificationFeature
		f.Type = "Feature"
		if err := rows.Scan(&f.Properties.Kind, &f.Properties.Name, &f.Geometry); err != nil {
			unavailable(w)
			return
		}
		if f.Geometry != nil {
			features = append(features, f)
		}
	}
	if rows.Err() != nil {
		unavailable(w)
		return
	}
	labelQuery := jurisdictionLabels
	if group == "roads" {
		labelQuery = roadLabels
	}
	labels, err := readMapLabels(ctx, tx, labelQuery, gen.ID)
	if err != nil {
		unavailable(w)
		return
	}
	for _, label := range labels {
		if !q.Has("jurisdiction") || labelJurisdictionKey(label) == q.Get("jurisdiction") {
			features = append(features, label)
		}
	}
	respond(w, map[string]any{"type": "FeatureCollection", "features": features, "generation": gen, "overview": overview})
}

const jurisdictionKeySQL = `(CASE j.layer_id WHEN 195 THEN 'municipality:' ELSE 'etj:' END || j.name)`

func validJurisdiction(key string) bool {
	kind, name, ok := strings.Cut(key, ":")
	return ok && (kind == "municipality" || kind == "etj") && strings.TrimSpace(name) != "" && len(key) <= 160
}

func labelJurisdictionKey(f qualificationFeature) string {
	return strings.TrimSuffix(f.Properties.Kind, "_label") + ":" + f.Properties.Name
}

const jurisdictionOverview = `SELECT 'jurisdiction', '', ST_AsGeoJSON(ST_Transform(ST_Collect(ST_Boundary(j.shape)),4326),7)::jsonb
 FROM qualification_jurisdictions j WHERE generation_id=$1 AND ` + jurisdictionKeySQL + `=$2 AND shape IS NOT NULL HAVING count(*)>0`

const jurisdictionLabels = `WITH anchors AS (
 SELECT DISTINCT ON (layer_id,name) layer_id,name,shape FROM qualification_jurisdictions
 WHERE generation_id=$1 AND shape IS NOT NULL AND name<>''
 ORDER BY layer_id,name,ST_Area(shape) DESC,source_id
 ) SELECT CASE layer_id WHEN 195 THEN 'municipality_label' ELSE 'etj_label' END,name,
 ST_AsGeoJSON(ST_Transform(ST_PointOnSurface(shape),4326),7)::jsonb FROM anchors ORDER BY layer_id DESC,name LIMIT 101`

const majorRoadClasses = `lower(trim(road_class)) IN ('primary','highway','expressway')`

const roadLabels = `WITH parts AS (
 SELECT trim(name) AS name, CASE lower(trim(road_class)) WHEN 'expressway' THEN 0 WHEN 'highway' THEN 1 ELSE 2 END AS priority,
 r.layer_id,r.source_id,d.path,d.geom,ST_Length(d.geom) AS length
 FROM qualification_roads r CROSS JOIN LATERAL ST_Dump(r.shape) d
 WHERE generation_id=$1 AND trim(name)<>'' AND ` + majorRoadClasses + `
 ), anchors AS (
 SELECT DISTINCT ON (lower(name)) name,priority,geom,length FROM parts WHERE length>0
 ORDER BY lower(name),priority,length DESC,layer_id,source_id,path
 ) SELECT 'road_label', name, ST_AsGeoJSON(ST_Transform(ST_LineInterpolatePoint(geom,0.5),4326),7)::jsonb
 FROM anchors ORDER BY priority,length DESC,name LIMIT 30`

func readMapLabels(ctx context.Context, tx pgx.Tx, query, generation string) ([]qualificationFeature, error) {
	rows, err := tx.Query(ctx, query, generation)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var features []qualificationFeature
	for rows.Next() {
		if len(features) == 100 {
			return nil, errors.New("map label limit exceeded")
		}
		f := qualificationFeature{Type: "Feature"}
		if err := rows.Scan(&f.Properties.Kind, &f.Properties.Name, &f.Geometry); err != nil {
			return nil, err
		}
		features = append(features, f)
	}
	return features, rows.Err()
}

func parseOverlayOverview(q url.Values) (bool, error) {
	if !q.Has("overview") {
		return false, nil
	}
	if q.Get("overview") != "true" || q.Has("bbox") || q.Has("zoom") {
		return false, errors.New("overview must be true and cannot include bbox or zoom")
	}
	return true, nil
}

func overlayViewportArgs(q url.Values, minZoom int, maxSpan float64) ([]any, error) {
	b, err := parseBounds(q.Get("bbox"))
	if err != nil {
		return nil, err
	}
	zoom, err := strconv.Atoi(q.Get("zoom"))
	if err != nil || zoom < minZoom || zoom > 22 || b.East-b.West > maxSpan || b.North-b.South > maxSpan {
		return nil, errors.New("zoom or bounds outside layer limits")
	}
	return []any{b.West, b.South, b.East, b.North, zoom}, nil
}

const zoneOverview = `SELECT 'zoning', '', ST_AsGeoJSON(ST_Transform(ST_Collect(ST_SimplifyPreserveTopology(shape,100)),4326),7)::jsonb
 FROM qualification_zones WHERE generation_id=$1 AND ST_Area(shape)>=35000 HAVING count(*)>0
 UNION ALL
 SELECT 'jurisdiction', '', ST_AsGeoJSON(ST_Transform(ST_Collect(ST_SimplifyPreserveTopology(ST_Boundary(shape),100)),4326),7)::jsonb
 FROM qualification_jurisdictions WHERE generation_id=$1 AND shape IS NOT NULL HAVING count(*)>0`

const roadOverview = `SELECT 'roads', '', ST_AsGeoJSON(ST_Transform(ST_Collect(ST_SimplifyPreserveTopology(shape,50)),4326),7)::jsonb
 FROM qualification_roads WHERE generation_id=$1 AND ` + majorRoadClasses + ` HAVING count(*)>0`

const viewportBounds = `WITH bounds AS (SELECT ST_Transform(ST_MakeEnvelope($2,$3,$4,$5,4326),3857) AS shape), hits AS MATERIALIZED (`
const zoneViewport = `
 SELECT 'zoning'::text AS kind, COALESCE(z.district_code,'') AS label,z.shape,3 AS dimension FROM qualification_zones z CROSS JOIN bounds b
 WHERE z.generation_id=$1 AND z.shape && b.shape AND ST_Intersects(z.shape,b.shape)
 UNION ALL
 SELECT 'jurisdiction',j.name,ST_Boundary(j.shape),2 FROM qualification_jurisdictions j CROSS JOIN bounds b
 WHERE j.generation_id=$1 AND j.shape && b.shape AND ST_Intersects(ST_Boundary(j.shape),b.shape)
 LIMIT 501`
const roadViewport = `
 SELECT 'roads'::text AS kind,COALESCE(r.name,'') AS label,r.shape,2 AS dimension FROM qualification_roads r CROSS JOIN bounds b
 WHERE r.generation_id=$1 AND r.shape && b.shape AND ST_Intersects(r.shape,b.shape)
 LIMIT 501`
const viewportGeometry = `), clipped AS MATERIALIZED (
 SELECT kind,label,ST_CollectionExtract(ST_Intersection(h.shape,b.shape),h.dimension) AS shape FROM hits h CROSS JOIN bounds b
) SELECT kind,label,CASE WHEN NOT ST_IsEmpty(shape) THEN ST_AsGeoJSON(ST_Transform(ST_SimplifyPreserveTopology(shape,GREATEST(0.1,50.0 / power(2.0,$6::int-9))),4326),7)::jsonb END FROM clipped`
