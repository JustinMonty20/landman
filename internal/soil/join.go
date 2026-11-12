package soil

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/JustinMonty20/landman/internal/county"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/twpayne/go-geom/encoding/ewkb"
)

const overlapInsert = `WITH hit AS (
 SELECT s.id AS soil_polygon_id, p.id AS parcel_id,
 ST_Area(ST_Transform(ST_Intersection(p.shape, s.shape), 2264)) / 43560 AS overlap_acres
 FROM parcels AS p JOIN soil_polygons AS s
 ON s.county_id = p.county_id AND s.source_name = p.source_name
 AND ST_Intersects(p.shape, s.shape)
 WHERE p.county_id = $1 AND p.source_name = $2 AND p.shape IS NOT NULL
)
INSERT INTO soil_polygon_parcels (soil_polygon_id, parcel_id, overlap_acres)
SELECT soil_polygon_id, parcel_id, overlap_acres FROM hit WHERE overlap_acres > 0
ON CONFLICT (soil_polygon_id, parcel_id) DO UPDATE
SET overlap_acres = EXCLUDED.overlap_acres, updated_at = now()`

// Store upserts a county soil snapshot and intersects it with every stored parcel in one transaction.
// It never deletes retired polygon or intersection rows.
func Store(ctx context.Context, connectionString string, countyID county.ID, source string, polygons []Polygon, units []MapUnit) error {
	pool, err := pgxpool.New(ctx, connectionString)
	if err != nil {
		return err
	}
	defer pool.Close()
	return store(ctx, pool, countyID, source, polygons, units, http.DefaultClient, soilDataAccessURL)
}

type beginner interface {
	Begin(context.Context) (pgx.Tx, error)
}

func store(ctx context.Context, db beginner, countyID county.ID, source string, polygons []Polygon, units []MapUnit, client *http.Client, endpoint string) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	distinct := make(map[string]MapUnit, len(units))
	for _, unit := range units {
		if unit.MapUnitKey == "" {
			return fmt.Errorf("soil: empty map unit key")
		}
		distinct[unit.MapUnitKey] = unit
	}
	missing := make([]string, 0, len(distinct))
	for key, unit := range distinct {
		if _, err = tx.Exec(ctx, `INSERT INTO soil_map_units (map_unit_key, map_unit_symbol, area_symbol) VALUES ($1,$2,$3)
   ON CONFLICT (map_unit_key) DO UPDATE SET map_unit_symbol=EXCLUDED.map_unit_symbol, area_symbol=EXCLUDED.area_symbol`, key, unit.MapUnitSymbol, unit.AreaSymbol); err != nil {
			return err
		}
		var needsName bool
		if err = tx.QueryRow(ctx, `SELECT map_unit_name IS NULL FROM soil_map_units WHERE map_unit_key=$1`, key).Scan(&needsName); err != nil {
			return err
		}
		if needsName {
			missing = append(missing, key)
		}
	}
	// Names are optional; canceled requests, unlike service failures, abort the whole import.
	names, nameErr := fetchNames(ctx, client, endpoint, missing)
	if err := ctx.Err(); err != nil {
		return err
	}
	if nameErr == nil {
		for key, result := range names {
			if _, ok := distinct[key]; !ok {
				continue
			}
			if _, err = tx.Exec(ctx, `UPDATE soil_map_units SET map_unit_name=NULLIF($2, ''), map_unit_symbol=$3, area_symbol=$4, area_name=$5, fetched_at=now() WHERE map_unit_key=$1`, key, result.name, result.symbol, result.areaSymbol, result.areaName); err != nil {
				return err
			}
		}
	}
	now := time.Now()
	for _, polygon := range polygons {
		if polygon.County != countyID || polygon.SourceName != source {
			return fmt.Errorf("soil: polygon source differs from import")
		}
		if polygon.Shape == nil {
			return fmt.Errorf("soil: polygon %s has no shape", polygon.ObjectID)
		}
		shape, err := ewkb.Marshal(*polygon.Shape, ewkb.NDR)
		if err != nil {
			return err
		}
		survey := polygon.SurveySavedOn
		if result, ok := names[polygon.MapUnitKey]; nameErr == nil && ok && result.surveyDate != nil {
			survey = result.surveyDate
		}
		if _, err = tx.Exec(ctx, `INSERT INTO soil_polygons (
   county_id, source_name, object_id, map_unit_key, spatial_version, survey_saved_on, shape, fetched_at
  ) VALUES ($1,$2,$3,$4,$5,$6,ST_GeomFromEWKB($7),$8)
  ON CONFLICT (county_id, source_name, object_id) DO UPDATE SET
   map_unit_key=EXCLUDED.map_unit_key, spatial_version=EXCLUDED.spatial_version,
   survey_saved_on=COALESCE(EXCLUDED.survey_saved_on, soil_polygons.survey_saved_on),
   shape=EXCLUDED.shape, fetched_at=EXCLUDED.fetched_at`, countyID.String(), source, polygon.ObjectID, polygon.MapUnitKey, polygon.SpatialVersion, survey, shape, now); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, overlapInsert, countyID.String(), source); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
