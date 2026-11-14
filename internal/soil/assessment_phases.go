package soil

import (
	"context"
	"fmt"

	"github.com/JustinMonty20/landman/internal/county"
	"github.com/jackc/pgx/v5"
)

// assessChangedParcels holds the input shapes in one transaction-local snapshot.
// Each phase gives PostgreSQL the actual row count before the next spatial join.
func assessChangedParcels(ctx context.Context, tx pgx.Tx, runID, setID string, countyID county.ID, source string) error {
	steps := []struct {
		name string
		sql  string
		args []any
	}{
		{"soil", `CREATE TEMP TABLE soil_assess_soil ON COMMIT DROP AS
SELECT polygon_id,map_unit_key,shape AS shape_3857,ST_Transform(shape,2264) AS geom
FROM soil_polygon_set_members WHERE set_id=$1`, []any{setID}},
		{"soil index", `CREATE UNIQUE INDEX ON soil_assess_soil(polygon_id)`, nil},
		{"soil statistics", `ANALYZE soil_assess_soil`, nil},
		{"parcels", `CREATE TEMP TABLE soil_assess_parcels ON COMMIT DROP AS
SELECT id,shape AS shape_3857,COALESCE(md5(ST_AsEWKB(shape)),'missing') AS fingerprint,
 ST_CollectionExtract(ST_Transform(shape,2264),3) AS geom
FROM parcels WHERE county_id=$1 AND source_name=$2`, []any{countyID.String(), source}},
		{"parcel index", `CREATE UNIQUE INDEX ON soil_assess_parcels(id)`, nil},
		{"parcel shape index", `CREATE INDEX ON soil_assess_parcels USING GIST(shape_3857)`, nil},
		{"parcel statistics", `ANALYZE soil_assess_parcels`, nil},
		{"assessment keys", `INSERT INTO soil_assessment_keys(parcel_id)
SELECT id FROM soil_assess_parcels ON CONFLICT DO NOTHING`, nil},
		{"pairs", `CREATE TEMP TABLE soil_assess_pairs ON COMMIT DROP AS
SELECT hit.id AS parcel_id,s.polygon_id,s.map_unit_key,
 CASE WHEN s.shape_3857 ~ hit.shape_3857 THEN ST_Covers(s.shape_3857,hit.shape_3857) ELSE false END AS covered
FROM soil_assess_soil s CROSS JOIN LATERAL (
 SELECT p.id,p.shape_3857 FROM soil_assess_parcels p
 WHERE p.shape_3857 && s.shape_3857 AND p.shape_3857 IS NOT NULL
 AND ST_Intersects(p.shape_3857,s.shape_3857) OFFSET 0
) hit`, nil},
		{"pair index", `CREATE INDEX ON soil_assess_pairs(polygon_id)`, nil},
		{"pair statistics", `ANALYZE soil_assess_pairs`, nil},
		{"hits", `CREATE TEMP TABLE soil_assess_hits ON COMMIT DROP AS
SELECT pair.parcel_id,pair.map_unit_key,pair.polygon_id,
 CASE WHEN pair.covered THEN p.geom
 ELSE ST_CollectionExtract(ST_Intersection(p.geom,s.geom),3) END AS geom
FROM soil_assess_pairs pair
JOIN soil_assess_parcels p ON p.id=pair.parcel_id
JOIN soil_assess_soil s ON s.polygon_id=pair.polygon_id
WHERE ST_Area(p.geom)>0`, nil},
		{"hit statistics", `ANALYZE soil_assess_hits`, nil},
		{"map unit geometry", `CREATE TEMP TABLE soil_assess_key ON COMMIT DROP AS
SELECT parcel_id,map_unit_key,ST_UnaryUnion(ST_Collect(geom)) AS geom,
 array_agg(DISTINCT polygon_id ORDER BY polygon_id) AS polygon_ids
FROM soil_assess_hits WHERE ST_Area(geom)>0 GROUP BY parcel_id,map_unit_key`, nil},
		{"map unit index", `CREATE INDEX ON soil_assess_key(parcel_id)`, nil},
		{"map unit statistics", `ANALYZE soil_assess_key`, nil},
		{"conflicts", `CREATE TEMP TABLE soil_assess_conflicts ON COMMIT DROP AS
SELECT a.parcel_id,ST_UnaryUnion(ST_Collect(ST_Intersection(a.geom,b.geom))) AS geom
FROM soil_assess_key a JOIN soil_assess_key b
 ON a.parcel_id=b.parcel_id AND a.map_unit_key<b.map_unit_key
WHERE a.geom && b.geom AND ST_Intersects(a.geom,b.geom) GROUP BY a.parcel_id`, nil},
		{"conflict statistics", `ANALYZE soil_assess_conflicts`, nil},
		{"coverage", `CREATE TEMP TABLE soil_assess_coverage ON COMMIT DROP AS
SELECT parcel_id,ST_UnaryUnion(ST_Collect(geom)) AS geom
FROM soil_assess_key GROUP BY parcel_id`, nil},
		{"coverage statistics", `ANALYZE soil_assess_coverage`, nil},
		{"assessments", `INSERT INTO soil_parcel_assessments
 (run_id,parcel_id,geometry_fingerprint,total_acres,uncovered_acres,ambiguous_acres)
SELECT $1,p.id,p.fingerprint,NULLIF(ST_Area(p.geom)/43560,0),
 CASE WHEN ST_Area(p.geom)>0 THEN ST_Area(CASE WHEN c.geom IS NULL THEN p.geom ELSE ST_Difference(p.geom,c.geom) END)/43560 ELSE 0 END,
 COALESCE(ST_Area(a.geom)/43560,0)
FROM soil_assess_parcels p
LEFT JOIN soil_assess_coverage c ON c.parcel_id=p.id
LEFT JOIN soil_assess_conflicts a ON a.parcel_id=p.id`, []any{runID}},
		{"map unit areas", `INSERT INTO soil_assessment_map_units
 (run_id,parcel_id,map_unit_key,exclusive_acres,mapped_acres,polygon_ids)
SELECT $1,k.parcel_id,k.map_unit_key,
 ST_Area(CASE WHEN a.geom IS NULL THEN k.geom ELSE ST_Difference(k.geom,a.geom) END)/43560,
 ST_Area(k.geom)/43560,k.polygon_ids
FROM soil_assess_key k LEFT JOIN soil_assess_conflicts a ON a.parcel_id=k.parcel_id`, []any{runID}},
	}
	for _, step := range steps {
		if _, err := tx.Exec(ctx, step.sql, step.args...); err != nil {
			return fmt.Errorf("soil assessment %s: %w", step.name, err)
		}
	}
	var missing int
	if err := tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM soil_assess_parcels) -
 (SELECT count(*) FROM soil_parcel_assessments WHERE run_id=$1)`, runID).Scan(&missing); err != nil {
		return err
	}
	if missing != 0 {
		return fmt.Errorf("soil assessment: %d parcels have no assessment", missing)
	}
	return nil
}
