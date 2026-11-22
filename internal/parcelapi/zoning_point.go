package parcelapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5"
)

type zoningDistrict struct {
	Jurisdiction  string `json:"jurisdiction"`
	Code          string `json:"code"`
	Description   string `json:"description"`
	CaseReference string `json:"case_reference"`
	SourceDate    string `json:"source_date"`
}

func (h *Handler) zoningAtPoint(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if err := parse(q, "lon", "lat"); err != nil {
		bad(w, err)
		return
	}
	lon, lonErr := strconv.ParseFloat(q.Get("lon"), 64)
	lat, latErr := strconv.ParseFloat(q.Get("lat"), 64)
	if lonErr != nil || latErr != nil || !(lon >= -180 && lon <= 180 && lat >= -85.051129 && lat <= 85.051129) {
		bad(w, errors.New("valid longitude and latitude are required"))
		return
	}
	ctx := r.Context()
	tx, err := h.tx(ctx)
	if err != nil {
		unavailable(w)
		return
	}
	defer tx.Rollback(context.Background())
	result := struct {
		GenerationID *string          `json:"generation_id"`
		Districts    []zoningDistrict `json:"districts"`
	}{Districts: []zoningDistrict{}}
	var generation string
	err = tx.QueryRow(ctx, `SELECT g.id::text FROM qualification_heads h
 JOIN qualification_generations g ON g.id=h.generation_id AND g.source_group=h.source_group
 WHERE h.source_group='zoning' AND g.completed_at IS NOT NULL`).Scan(&generation)
	if errors.Is(err, pgx.ErrNoRows) {
		respond(w, result)
		return
	}
	if err != nil {
		unavailable(w)
		return
	}
	result.GenerationID = &generation
	rows, err := tx.Query(ctx, `WITH location AS (SELECT ST_Transform(ST_SetSRID(ST_Point($2,$3),4326),3857) AS point)
 SELECT DISTINCT jurisdiction,COALESCE(trim(district_code),'') AS code,COALESCE(trim(district_description),'') AS description,
 COALESCE(trim(case_reference),'') AS case_reference,COALESCE(trim(source_date),'') AS source_date
 FROM qualification_zones CROSS JOIN location
 WHERE generation_id=$1 AND ST_Covers(shape,point) ORDER BY jurisdiction,code,description,case_reference,source_date LIMIT 51`, generation, lon, lat)
	if err != nil {
		unavailable(w)
		return
	}
	defer rows.Close()
	for rows.Next() {
		if len(result.Districts) == 50 {
			http.Error(w, "Too many zoning records at this location", http.StatusRequestEntityTooLarge)
			return
		}
		var district zoningDistrict
		if err := rows.Scan(&district.Jurisdiction, &district.Code, &district.Description, &district.CaseReference, &district.SourceDate); err != nil {
			unavailable(w)
			return
		}
		result.Districts = append(result.Districts, district)
	}
	if rows.Err() != nil {
		unavailable(w)
		return
	}
	respond(w, result)
}
