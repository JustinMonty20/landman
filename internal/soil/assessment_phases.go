package soil

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/JustinMonty20/landman/internal/county"
	"github.com/JustinMonty20/landman/internal/jobs"
	"github.com/jackc/pgx/v5"
)

const prepareAssessmentSoil = `CREATE TEMP TABLE soil_assess_soil ON COMMIT DROP AS
WITH input AS MATERIALIZED (
 SELECT m.polygon_id,m.map_unit_key,m.shape,p.object_id,ST_IsValid(m.shape) AND NOT ST_IsEmpty(m.shape) AS usable
 FROM soil_polygon_set_members m JOIN soil_polygons p ON p.id=m.polygon_id WHERE m.set_id=$1
), repaired AS MATERIALIZED (
 SELECT *,ST_Multi(ST_CollectionExtract(ST_MakeValid(shape,'method=linework'),3)) AS linework,
          ST_Multi(ST_CollectionExtract(ST_MakeValid(shape,'method=structure keepcollapsed=false'),3)) AS structure
 FROM input WHERE NOT usable
), boundaries AS MATERIALIZED (
 SELECT *,ST_UnaryUnion(ST_Boundary(shape)) AS original_boundary,
          ST_UnaryUnion(ST_Boundary(linework)) AS repaired_boundary
 FROM repaired
)
SELECT polygon_id,map_unit_key,object_id,shape FROM input WHERE usable
UNION ALL
SELECT polygon_id,map_unit_key,object_id,
 CASE WHEN ST_IsValid(linework) AND ST_IsValid(structure) AND NOT ST_IsEmpty(linework) AND NOT ST_IsEmpty(structure)
      THEN CASE WHEN ST_Equals(linework,structure) AND ST_Equals(original_boundary,repaired_boundary) THEN linework END
 END FROM boundaries`

func measuredAcres(geom string) string {
	return `CASE WHEN ST_Area(` + geom + `)>0 THEN NULLIF(ST_Area(ST_Transform(ST_Segmentize(ST_Simplify(` + geom + `,0),10),2264)),0)/43560 ELSE 0 END`
}

func assessChangedParcels(ctx context.Context, tx pgx.Tx, runID, setID string, countyID county.ID, source string) error {
	attempt, _ := jobs.AttemptID(ctx)
	if _, err := tx.Exec(ctx, prepareAssessmentSoil, setID); err != nil {
		return fmt.Errorf("prepare soil boundaries for set %s: %w", setID, err)
	}
	var objectID string
	err := tx.QueryRow(ctx, `SELECT object_id FROM soil_assess_soil WHERE shape IS NULL ORDER BY object_id LIMIT 1`).Scan(&objectID)
	switch {
	case err == nil:
		return fmt.Errorf("soil set %s polygon OBJECTID %s: invalid geometry does not preserve its boundary after repair", setID, objectID)
	case !errors.Is(err, pgx.ErrNoRows):
		return fmt.Errorf("check soil boundaries for set %s: %w", setID, err)
	}
	steps := []struct {
		name string
		sql  string
		args []any
	}{
		{"parcel boundaries", `CREATE TEMP TABLE soil_assess_parcels ON COMMIT DROP AS
SELECT id,CASE WHEN parcel_boundary_usable(shape) THEN shape END AS shape,
       COALESCE(md5(ST_AsEWKB(shape)),'missing') AS fingerprint
FROM parcels WHERE county_id=$1 AND source_name=$2`, []any{countyID.String(), source}},
		{"parcel index", `CREATE INDEX ON soil_assess_parcels USING GIST(shape) WHERE shape IS NOT NULL`, nil},
		{"parcel statistics", `ANALYZE soil_assess_parcels`, nil},
		{"intersections", `CREATE TEMP TABLE soil_assess_hits ON COMMIT DROP AS
SELECT p.id AS parcel_id,s.polygon_id,s.map_unit_key,hit.geom
FROM soil_assess_soil s CROSS JOIN LATERAL (
 SELECT p.id,p.shape FROM soil_assess_parcels p WHERE p.shape IS NOT NULL AND p.shape && s.shape OFFSET 0
) p CROSS JOIN LATERAL (
 SELECT ST_CollectionExtract(ST_Intersection(p.shape,s.shape),3) AS geom WHERE ST_Intersects(p.shape,s.shape) OFFSET 0
) hit WHERE ST_Area(hit.geom)>0`, nil},
		{"map unit geometry", `CREATE TEMP TABLE soil_assess_key ON COMMIT DROP AS
SELECT parcel_id,map_unit_key,ST_UnaryUnion(ST_Collect(geom ORDER BY polygon_id)) AS geom,
 array_agg(polygon_id ORDER BY polygon_id) AS polygon_ids
FROM soil_assess_hits GROUP BY parcel_id,map_unit_key`, nil},
		{"map unit index", `CREATE INDEX ON soil_assess_key(parcel_id)`, nil},
		{"map unit statistics", `ANALYZE soil_assess_key`, nil},
		{"conflicts", `CREATE TEMP TABLE soil_assess_conflicts ON COMMIT DROP AS
SELECT a.parcel_id,ST_UnaryUnion(ST_Collect(hit.geom ORDER BY a.map_unit_key,b.map_unit_key)) AS geom
FROM soil_assess_key a JOIN soil_assess_key b
 ON a.parcel_id=b.parcel_id AND a.map_unit_key<b.map_unit_key AND a.geom && b.geom
CROSS JOIN LATERAL (
 SELECT ST_CollectionExtract(ST_Intersection(a.geom,b.geom),3) AS geom WHERE ST_Intersects(a.geom,b.geom) OFFSET 0
) hit WHERE ST_Area(hit.geom)>0 GROUP BY a.parcel_id`, nil},
		{"coverage", `CREATE TEMP TABLE soil_assess_coverage ON COMMIT DROP AS
SELECT parcel_id,ST_UnaryUnion(ST_Collect(geom ORDER BY map_unit_key)) AS geom
FROM soil_assess_key GROUP BY parcel_id`, nil},
		{"assessments", `INSERT INTO soil_parcel_assessments
 (run_id,parcel_id,geometry_fingerprint,total_acres,uncovered_acres,ambiguous_acres)
SELECT $1,p.id,p.fingerprint,NULLIF(` + measuredAcres("p.shape") + `,0),
 ` + measuredAcres("gap.geom") + `, ` + measuredAcres("a.geom") + `
FROM soil_assess_parcels p
LEFT JOIN soil_assess_coverage c ON c.parcel_id=p.id
LEFT JOIN soil_assess_conflicts a ON a.parcel_id=p.id
CROSS JOIN LATERAL (SELECT CASE WHEN c.geom IS NULL THEN p.shape ELSE ST_Difference(p.shape,c.geom) END AS geom OFFSET 0) gap`, []any{runID}},
		{"map unit areas", `INSERT INTO soil_assessment_map_units
 (run_id,parcel_id,map_unit_key,exclusive_acres,mapped_acres,polygon_ids)
SELECT $1,k.parcel_id,k.map_unit_key,` + measuredAcres("exclusive.geom") + `,
 ` + measuredAcres("k.geom") + `,k.polygon_ids
FROM soil_assess_key k LEFT JOIN soil_assess_conflicts a ON a.parcel_id=k.parcel_id
CROSS JOIN LATERAL (SELECT CASE WHEN a.geom IS NULL THEN k.geom ELSE ST_Difference(k.geom,a.geom) END AS geom OFFSET 0) exclusive`, []any{runID}},
		{"individual overlaps", `INSERT INTO soil_polygon_parcels (soil_polygon_id,parcel_id,overlap_acres)
SELECT polygon_id,parcel_id,acres FROM (
 SELECT polygon_id,parcel_id,(ST_Area(ST_Transform(geom,2264))/43560)::numeric(14,6) AS acres FROM soil_assess_hits
) measured WHERE acres>0
ON CONFLICT (soil_polygon_id,parcel_id) DO UPDATE SET overlap_acres=EXCLUDED.overlap_acres,updated_at=now()
WHERE soil_polygon_parcels.overlap_acres IS DISTINCT FROM EXCLUDED.overlap_acres`, nil},
	}
	for _, step := range steps {
		log.Printf("[union.soil.screening] attempt=%s soil_set=%s run=%s: %s", attempt, setID, runID, step.name)
		tag, err := tx.Exec(ctx, step.sql, step.args...)
		if err != nil {
			return fmt.Errorf("soil set %s screening run %s: %s: %w", setID, runID, step.name, err)
		}
		if tag.RowsAffected() > 0 {
			log.Printf("[union.soil.screening] attempt=%s soil_set=%s run=%s: %s rows=%d", attempt, setID, runID, step.name, tag.RowsAffected())
		}
	}
	return nil
}
