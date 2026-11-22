package parcelapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

type floodRelease struct {
	ID         string    `json:"id"`
	IssuedAt   time.Time `json:"issued_at"`
	ObservedAt time.Time `json:"observed_at"`
}

func activeFlood(ctx context.Context, tx pgx.Tx) (*floodRelease, error) {
	var release floodRelease
	err := tx.QueryRow(ctx, `SELECT r.capture_id::text,r.issued_at,r.observed_at FROM fema_release_head h JOIN fema_releases r ON r.capture_id=h.capture_id WHERE h.study_id='37179C'`).Scan(&release.ID, &release.IssuedAt, &release.ObservedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &release, err
}

type floodFeature struct {
	Type       string          `json:"type"`
	Geometry   json.RawMessage `json:"geometry"`
	Properties floodZone       `json:"properties"`
}

type floodZone struct {
	Code    string  `json:"code"`
	Subtype string  `json:"subtype"`
	SFHA    string  `json:"sfha"`
	Acres   float64 `json:"acres,omitempty"`
}

func (h *Handler) floodMap(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if err := parse(q, "bbox", "zoom", "overview"); err != nil {
		bad(w, err)
		return
	}
	overview, err := parseOverlayOverview(q)
	if err != nil {
		bad(w, err)
		return
	}
	query := floodOverview
	var args []any
	if !overview {
		args, err = overlayViewportArgs(q, 9, 3)
		if err != nil {
			bad(w, err)
			return
		}
		query = floodViewport
	}
	ctx := r.Context()
	tx, err := h.tx(ctx)
	if err != nil {
		unavailable(w)
		return
	}
	defer tx.Rollback(context.Background())
	release, err := activeFlood(ctx, tx)
	if err != nil {
		unavailable(w)
		return
	}
	features := []floodFeature{}
	if release != nil {
		rows, err := tx.Query(ctx, query, append([]any{release.ID}, args...)...)
		if err != nil {
			unavailable(w)
			return
		}
		defer rows.Close()
		for candidates := 1; rows.Next(); candidates++ {
			if candidates > 500 {
				http.Error(w, "Too many flood zones; zoom in", http.StatusRequestEntityTooLarge)
				return
			}
			item := floodFeature{Type: "Feature"}
			if err := rows.Scan(&item.Properties.Code, &item.Properties.Subtype, &item.Properties.SFHA, &item.Geometry); err != nil {
				unavailable(w)
				return
			}
			if item.Geometry != nil {
				features = append(features, item)
			}
		}
		if rows.Err() != nil {
			unavailable(w)
			return
		}
	}
	respond(w, map[string]any{"type": "FeatureCollection", "features": features, "release": release, "overview": overview})
}

const floodOverview = `SELECT zone_code,zone_subtype,sfha_flag,
 ST_AsGeoJSON(ST_Transform(ST_Collect(ST_SimplifyPreserveTopology(shape,0.0008)),4326),7)::jsonb
 FROM fema_zones WHERE capture_id=$1 GROUP BY zone_code,zone_subtype,sfha_flag
 ORDER BY (zone_subtype='FLOODWAY'),(sfha_flag='T'),zone_code,zone_subtype,sfha_flag`

const floodViewport = `WITH bounds AS (SELECT ST_Transform(ST_MakeEnvelope($2,$3,$4,$5,4326),4269) AS shape), hits AS MATERIALIZED (
 SELECT z.zone_code,z.zone_subtype,z.sfha_flag,ST_CollectionExtract(ST_Intersection(z.shape,b.shape),3) AS shape
 FROM fema_zones z CROSS JOIN bounds b WHERE z.capture_id=$1 AND z.shape && b.shape AND ST_Intersects(z.shape,b.shape)
 ORDER BY z.source_id LIMIT 501
 ) SELECT zone_code,zone_subtype,sfha_flag,CASE WHEN NOT ST_IsEmpty(shape) THEN ST_AsGeoJSON(ST_Transform(ST_SimplifyPreserveTopology(shape, GREATEST(0.000001,0.0005 / power(2.0,$6::int-9))),4326),7)::jsonb END FROM hits`

func (h *Handler) floodDetail(w http.ResponseWriter, r *http.Request, id string) {
	ctx := r.Context()
	tx, err := h.tx(ctx)
	if err != nil {
		unavailable(w)
		return
	}
	defer tx.Rollback(context.Background())
	release, err := activeFlood(ctx, tx)
	if err != nil {
		unavailable(w)
		return
	}
	var number string
	var hasShape bool
	err = tx.QueryRow(ctx, `SELECT source_parcel_id,parcel_boundary_usable(shape) FROM parcels WHERE id=$1 AND county_id=$2 AND source_name=$3`, id, countyID, parcelSource).Scan(&number, &hasShape)
	if errors.Is(err, pgx.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		unavailable(w)
		return
	}
	if release == nil || !hasShape {
		reason := "no_release"
		if release != nil {
			reason = "missing_geometry"
		}
		respond(w, map[string]any{"status": reason, "parcel_number": number, "release": release})
		return
	}
	var totals struct {
		Parcel   float64 `json:"parcel"`
		Unknown  float64 `json:"unknown"`
		Floodway float64 `json:"floodway"`
	}
	err = tx.QueryRow(ctx, floodTotals, id, release.ID).Scan(&totals.Parcel, &totals.Unknown, &totals.Floodway)
	if err != nil {
		unavailable(w)
		return
	}
	zones, err := readFloodZones(ctx, tx, id, release.ID)
	if err != nil {
		unavailable(w)
		return
	}
	type panel struct {
		Number    string  `json:"number"`
		Effective *string `json:"effective_date"`
	}
	panels := []panel{}
	rows, err := tx.Query(ctx, `WITH parcel AS (SELECT ST_Transform(shape,4269) AS g FROM parcels WHERE id=$1)
 SELECT DISTINCT panel_number,effective_date::text FROM fema_panels f CROSS JOIN parcel p WHERE f.capture_id=$2 AND f.shape && p.g AND ST_Area(ST_Intersection(f.shape,p.g))>0 ORDER BY panel_number,effective_date::text`, id, release.ID)
	if err != nil {
		unavailable(w)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var item panel
		if err := rows.Scan(&item.Number, &item.Effective); err != nil {
			unavailable(w)
			return
		}
		panels = append(panels, item)
	}
	if rows.Err() != nil {
		unavailable(w)
		return
	}
	respond(w, map[string]any{"status": "available", "parcel_number": number, "release": release, "acres": totals, "zones": zones, "panels": panels})
}

const floodTotals = `WITH parcel AS MATERIALIZED (SELECT ST_Transform(shape,4269) AS g FROM parcels WHERE id=$1), covered AS (
 SELECT p.g, ST_Intersection(ST_Intersection(p.g,c.panel_extent),c.mapped_extent) AS known
 FROM parcel p JOIN fema_coverage c ON c.capture_id=$2
), floodway AS (
 SELECT ST_Union(ST_CollectionExtract(ST_Intersection(z.shape,p.g),3)) AS g
 FROM fema_zones z CROSS JOIN parcel p WHERE z.capture_id=$2 AND z.zone_subtype='FLOODWAY' AND z.shape && p.g AND ST_Intersects(z.shape,p.g)
) SELECT ST_Area(ST_Transform(c.g,2264))/43560.0,ST_Area(ST_Transform(ST_Difference(c.g,c.known),2264))/43560.0,COALESCE(ST_Area(ST_Transform(f.g,2264))/43560.0,0) FROM covered c CROSS JOIN floodway f`

func readFloodZones(ctx context.Context, tx pgx.Tx, id, release string) ([]floodZone, error) {
	rows, err := tx.Query(ctx, `WITH parcel AS MATERIALIZED (SELECT ST_Transform(shape,4269) AS g FROM parcels WHERE id=$1), pieces AS MATERIALIZED (
 SELECT z.zone_code,z.zone_subtype,z.sfha_flag,ST_CollectionExtract(ST_Intersection(z.shape,p.g),3) AS shape
 FROM fema_zones z CROSS JOIN parcel p WHERE z.capture_id=$2 AND z.shape && p.g AND ST_Intersects(z.shape,p.g)
 ) SELECT zone_code,zone_subtype,sfha_flag,ST_Area(ST_Transform(ST_Union(shape),2264))/43560.0 FROM pieces WHERE ST_Area(shape)>0 GROUP BY zone_code,zone_subtype,sfha_flag ORDER BY zone_code,zone_subtype,sfha_flag`, id, release)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []floodZone{}
	for rows.Next() {
		var z floodZone
		if err := rows.Scan(&z.Code, &z.Subtype, &z.SFHA, &z.Acres); err != nil {
			return nil, err
		}
		result = append(result, z)
	}
	return result, rows.Err()
}
