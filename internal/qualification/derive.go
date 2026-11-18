package qualification

import (
	"context"
	"github.com/jackc/pgx/v5"
)

func (s Store) reconcileZoning(ctx context.Context, tx pgx.Tx, gen, method string) error {
	statements := []struct {
		sql  string
		args []any
	}{
		{`CREATE TEMP TABLE qualification_zone_shapes ON COMMIT DROP AS
 SELECT p.id parcel_id,z.layer_id,z.source_id,z.jurisdiction,z.district_code,
 ST_CollectionExtract(ST_Transform(ST_CollectionExtract(ST_Intersection(p.shape,z.shape),3),2264),3) geom
 FROM qualification_zones z CROSS JOIN LATERAL
 (SELECT id,shape,geom FROM qualification_targets WHERE shape && z.shape AND geom IS NOT NULL AND ST_Intersects(shape,z.shape) OFFSET 0) p
 WHERE z.generation_id=$1`, []any{gen}},
		{`DELETE FROM qualification_zone_shapes WHERE ST_Area(geom)=0`, nil},
		{`CREATE INDEX ON qualification_zone_shapes(parcel_id)`, nil},
		{`ANALYZE qualification_zone_shapes`, nil},
		{`CREATE TEMP TABLE qualification_coverage ON COMMIT DROP AS
 SELECT parcel_id,ST_UnaryUnion(ST_Collect(geom)) geom FROM qualification_zone_shapes GROUP BY parcel_id`, nil},
		{`CREATE TEMP TABLE qualification_ambiguity ON COMMIT DROP AS
 SELECT parcel_id,ST_UnaryUnion(ST_Collect(geom)) geom FROM (
 SELECT a.parcel_id,ST_CollectionExtract(ST_Intersection(a.geom,b.geom),3) geom
 FROM qualification_zone_shapes a JOIN qualification_zone_shapes b ON a.parcel_id=b.parcel_id AND
 (a.layer_id,a.source_id)<(b.layer_id,b.source_id)
 WHERE (a.jurisdiction,a.district_code) IS DISTINCT FROM (b.jurisdiction,b.district_code)
 AND a.geom && b.geom AND ST_Intersects(a.geom,b.geom)
 UNION ALL SELECT parcel_id,geom FROM qualification_zone_shapes WHERE district_code IS NULL
 ) conflicts WHERE ST_Area(geom)>0 GROUP BY parcel_id`, nil},
		{`CREATE TEMP TABLE qualification_jurisdiction_shapes ON COMMIT DROP AS
SELECT p.id parcel_id,j.layer_id,j.source_id,j.name,
ST_CollectionExtract(ST_Transform(ST_CollectionExtract(ST_Intersection(p.shape,j.shape),3),2264),3) geom
FROM qualification_jurisdictions j CROSS JOIN LATERAL
(SELECT id,shape,geom FROM qualification_targets WHERE shape && j.shape AND geom IS NOT NULL AND ST_Intersects(shape,j.shape) OFFSET 0) p
WHERE j.generation_id=$1`, []any{gen}},
		{`DELETE FROM qualification_jurisdiction_shapes WHERE ST_Area(geom)=0`, nil},
		{`CREATE INDEX ON qualification_jurisdiction_shapes(parcel_id)`, nil},
		{`ANALYZE qualification_jurisdiction_shapes`, nil},
		{`CREATE TEMP TABLE qualification_jurisdiction_coverage ON COMMIT DROP AS
SELECT parcel_id,ST_UnaryUnion(ST_Collect(geom)) geom FROM qualification_jurisdiction_shapes GROUP BY parcel_id`, nil},
		{`CREATE TEMP TABLE qualification_jurisdiction_conflicts ON COMMIT DROP AS
SELECT parcel_id,ST_UnaryUnion(ST_Collect(geom)) geom FROM (
SELECT a.parcel_id,ST_CollectionExtract(ST_Intersection(a.geom,b.geom),3) geom
FROM qualification_jurisdiction_shapes a JOIN qualification_jurisdiction_shapes b ON a.parcel_id=b.parcel_id AND
(a.layer_id,a.source_id)<(b.layer_id,b.source_id) AND (a.layer_id,a.name) IS DISTINCT FROM (b.layer_id,b.name)
WHERE a.geom && b.geom AND ST_Intersects(a.geom,b.geom)
) pairs WHERE ST_Area(geom)>0 GROUP BY parcel_id`, nil},
		{`INSERT INTO qualification_assessments(parcel_id,source_group,generation_id,geometry_fingerprint,method_version,total_acres,unmapped_acres,ambiguous_acres,jurisdiction_unmapped_acres,jurisdiction_ambiguous_acres)
 SELECT p.id,'zoning',$1,p.fingerprint,$2,NULLIF(ST_Area(p.geom)/43560,0),
 CASE WHEN p.geom IS NULL THEN 0 ELSE ST_Area(CASE WHEN c.geom IS NULL THEN p.geom ELSE ST_Difference(p.geom,c.geom) END)/43560 END,
 COALESCE(ST_Area(a.geom)/43560,0),
 CASE WHEN p.geom IS NULL THEN 0 ELSE ST_Area(CASE WHEN jc.geom IS NULL THEN p.geom ELSE ST_Difference(p.geom,jc.geom) END)/43560 END,
 COALESCE(ST_Area(ja.geom)/43560,0)
 FROM qualification_targets p LEFT JOIN qualification_coverage c ON c.parcel_id=p.id
 LEFT JOIN qualification_ambiguity a ON a.parcel_id=p.id
 LEFT JOIN qualification_jurisdiction_coverage jc ON jc.parcel_id=p.id
 LEFT JOIN qualification_jurisdiction_conflicts ja ON ja.parcel_id=p.id
 ON CONFLICT(parcel_id,source_group) DO UPDATE SET generation_id=excluded.generation_id,
 geometry_fingerprint=excluded.geometry_fingerprint,method_version=excluded.method_version,
 calculated_at=now(),total_acres=excluded.total_acres,unmapped_acres=excluded.unmapped_acres,
 ambiguous_acres=excluded.ambiguous_acres,jurisdiction_unmapped_acres=excluded.jurisdiction_unmapped_acres,
 jurisdiction_ambiguous_acres=excluded.jurisdiction_ambiguous_acres`, []any{gen, method}},
		{`DELETE FROM qualification_zone_hits WHERE parcel_id IN (SELECT id FROM qualification_targets)`, nil},
		{`DELETE FROM qualification_jurisdiction_hits WHERE parcel_id IN (SELECT id FROM qualification_targets)`, nil},
		{`INSERT INTO qualification_zone_hits(parcel_id,layer_id,source_id,jurisdiction,district_code,acres,share)
 SELECT h.parcel_id,h.layer_id,h.source_id,h.jurisdiction,h.district_code,
 ST_Area(h.geom)/43560,ST_Area(h.geom)/ST_Area(p.geom)
 FROM qualification_zone_shapes h JOIN qualification_targets p ON p.id=h.parcel_id WHERE ST_Area(p.geom)>0`, nil},
		{`INSERT INTO qualification_jurisdiction_hits(parcel_id,layer_id,source_id,name,acres,share)
SELECT h.parcel_id,h.layer_id,h.source_id,h.name,ST_Area(h.geom)/43560,
ST_Area(h.geom)/ST_Area(p.geom)
FROM qualification_jurisdiction_shapes h JOIN qualification_targets p ON p.id=h.parcel_id WHERE ST_Area(p.geom)>0`, nil},
	}
	for _, step := range statements {
		if _, err := tx.Exec(ctx, step.sql, step.args...); err != nil {
			return err
		}
	}
	return nil
}

func (s Store) reconcileRoads(ctx context.Context, tx pgx.Tx, gen, method string) error {
	statements := []struct {
		sql  string
		args []any
	}{
		{`CREATE TEMP TABLE qualification_distances ON COMMIT DROP AS
 SELECT p.id parcel_id,r.source_id,r.name,r.road_class,r.status,
 CASE WHEN ST_Intersects(p.shape,r.shape) THEN 0 ELSE ST_Distance(p.geom,ST_Transform(r.shape,2264)) END feet
 FROM qualification_targets p CROSS JOIN LATERAL (
 SELECT source_id,name,road_class,status,shape FROM qualification_roads
 WHERE generation_id=$1 AND p.geom IS NOT NULL AND ST_Area(p.geom)>0
 AND shape && ST_Expand(p.shape,8000)
 AND ST_DWithin(p.geom,ST_Transform(shape,2264),5280) OFFSET 0
 ) r`, []any{gen}},
		{`CREATE INDEX ON qualification_distances(parcel_id)`, nil},
		{`ANALYZE qualification_distances`, nil},
		{`INSERT INTO qualification_assessments(parcel_id,source_group,generation_id,boundary_generation_id,geometry_fingerprint,method_version,total_acres,road_state,road_distance_feet,road_search_radius_feet,county_edge_warning)
 SELECT p.id,'roads',$1,(SELECT generation_id FROM qualification_heads WHERE source_group='zoning'),p.fingerprint,$2,
 NULLIF(ST_Area(p.geom)/43560,0),
 CASE WHEN p.geom IS NULL OR ST_Area(p.geom)=0 THEN 'missing_parcel_geometry'
 WHEN d.feet IS NULL THEN 'none_within_radius' ELSE 'nearest' END,d.feet,5280,
 EXISTS (SELECT 1 FROM (SELECT ST_UnaryUnion(ST_Collect(b.shape)) shape FROM qualification_boundaries b
 JOIN qualification_heads h ON h.source_group='zoning' AND h.generation_id=b.generation_id) b
 WHERE p.geom IS NOT NULL AND (b.shape IS NULL OR NOT ST_Covers(b.shape,p.shape)
 OR ST_DWithin(p.geom,ST_Boundary(ST_Transform(b.shape,2264)),5280)))
 FROM qualification_targets p LEFT JOIN (SELECT parcel_id,min(feet) feet FROM qualification_distances GROUP BY parcel_id) d ON d.parcel_id=p.id
 ON CONFLICT(parcel_id,source_group) DO UPDATE SET generation_id=excluded.generation_id,boundary_generation_id=excluded.boundary_generation_id,
 geometry_fingerprint=excluded.geometry_fingerprint,method_version=excluded.method_version,calculated_at=now(),
 total_acres=excluded.total_acres,road_state=excluded.road_state,road_distance_feet=excluded.road_distance_feet,
 road_search_radius_feet=excluded.road_search_radius_feet,county_edge_warning=excluded.county_edge_warning`, []any{gen, method}},
		{`DELETE FROM qualification_road_ties WHERE parcel_id IN (SELECT id FROM qualification_targets)`, nil},
		{`INSERT INTO qualification_road_ties(parcel_id,source_id,name,road_class,status,distance_feet)
 SELECT d.parcel_id,d.source_id,d.name,d.road_class,d.status,d.feet
 FROM qualification_distances d JOIN qualification_assessments a ON a.parcel_id=d.parcel_id AND a.source_group='roads'
 WHERE d.feet <= a.road_distance_feet+0.01`, nil},
	}
	for _, step := range statements {
		if _, err := tx.Exec(ctx, step.sql, step.args...); err != nil {
			return err
		}
	}
	return nil
}
