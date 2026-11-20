package qualification

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
)

type assessmentStep struct {
	name string
	sql  string
	args []any
}

func runAssessmentSteps(ctx context.Context, tx pgx.Tx, gen string, steps []assessmentStep) error {
	for i, step := range steps {
		log.Printf("[union.qualification.assess] generation=%s step=%d/%d: %s", gen, i+1, len(steps), step.name)
		start := time.Now()
		if _, err := tx.Exec(ctx, step.sql, step.args...); err != nil {
			return fmt.Errorf("step %d/%d %s after %s: %w", i+1, len(steps), step.name, time.Since(start).Round(time.Millisecond), err)
		}
		log.Printf("[union.qualification.assess] generation=%s step=%d/%d: %s complete elapsed=%s", gen, i+1, len(steps), step.name, time.Since(start).Round(time.Millisecond))
	}
	return nil
}

func (s Store) reconcileZoning(ctx context.Context, tx pgx.Tx, gen, method string) error {
	initial := []assessmentStep{
		// Reduce precision once per zone. Use the original shape for candidate matching and raw area checks.
		{"Find zoning overlaps", `CREATE TEMP TABLE qualification_zone_sources ON COMMIT DROP AS
 WITH zones AS MATERIALIZED (
 SELECT layer_id,source_id,jurisdiction,district_code,shape,ST_ReducePrecision(shape,0.000001) geom
 FROM qualification_zones WHERE generation_id=$1 AND shape IS NOT NULL
 )
 SELECT p.id parcel_id,z.layer_id,z.source_id,z.jurisdiction,z.district_code,z.geom,
 ST_Area(ST_CollectionExtract(ST_Intersection(p.shape,z.shape),3)) original_area
 FROM zones z CROSS JOIN LATERAL
 (SELECT id,shape FROM qualification_targets WHERE shape && z.shape AND geom IS NOT NULL AND ST_Intersects(shape,z.shape) OFFSET 0) p`, []any{gen}},
		{"Clip zones to parcels", `CREATE TEMP TABLE qualification_zone_shapes ON COMMIT DROP AS
 SELECT s.parcel_id,s.layer_id,s.source_id,s.jurisdiction,s.district_code,
 ST_CollectionExtract(ST_Intersection(p.analysis_geom,s.geom,p.precision_grid),3) geom
 FROM qualification_zone_sources s JOIN qualification_targets p ON p.id=s.parcel_id`, nil},
		{"Find jurisdiction overlaps", `CREATE TEMP TABLE qualification_jurisdiction_sources ON COMMIT DROP AS
WITH jurisdictions AS MATERIALIZED (
 SELECT layer_id,source_id,name,shape,ST_ReducePrecision(shape,0.000001) geom
 FROM qualification_jurisdictions WHERE generation_id=$1 AND shape IS NOT NULL
)
SELECT p.id parcel_id,j.layer_id,j.source_id,j.name,j.geom,
ST_Area(ST_CollectionExtract(ST_Intersection(p.shape,j.shape),3)) original_area
FROM jurisdictions j CROSS JOIN LATERAL
(SELECT id,shape FROM qualification_targets WHERE shape && j.shape AND geom IS NOT NULL AND ST_Intersects(shape,j.shape) OFFSET 0) p`, []any{gen}},
		{"Clip jurisdictions to parcels", `CREATE TEMP TABLE qualification_jurisdiction_shapes ON COMMIT DROP AS
SELECT s.parcel_id,s.layer_id,s.source_id,s.name,
ST_CollectionExtract(ST_Intersection(p.analysis_geom,s.geom,p.precision_grid),3) geom
FROM qualification_jurisdiction_sources s JOIN qualification_targets p ON p.id=s.parcel_id`, nil},
		{"Register precision retry evidence", `CREATE TEMP TABLE qualification_precision_retry ON COMMIT DROP AS
SELECT p.id parcel_id,'parcel'::text source_group,NULL::integer layer_id,NULL::bigint source_id,ST_Area(p.shape) original_area,'source'::text failure
FROM qualification_targets p WHERE p.shape IS NOT NULL AND ST_Area(p.shape)>0 AND ST_Area(p.analysis_geom)=0
UNION ALL
SELECT s.parcel_id,'zoning',s.layer_id,s.source_id,s.original_area,
CASE WHEN ST_Area(s.geom)=0 THEN 'source' ELSE 'clip' END
FROM qualification_zone_sources s LEFT JOIN qualification_zone_shapes h
ON h.parcel_id=s.parcel_id AND h.layer_id=s.layer_id AND h.source_id=s.source_id
WHERE s.original_area>0 AND (ST_Area(s.geom)=0 OR h.geom IS NULL OR ST_Area(h.geom)=0)
UNION ALL
SELECT s.parcel_id,'jurisdiction',s.layer_id,s.source_id,s.original_area,
CASE WHEN ST_Area(s.geom)=0 THEN 'source' ELSE 'clip' END
FROM qualification_jurisdiction_sources s LEFT JOIN qualification_jurisdiction_shapes h
ON h.parcel_id=s.parcel_id AND h.layer_id=s.layer_id AND h.source_id=s.source_id
WHERE s.original_area>0 AND (ST_Area(s.geom)=0 OR h.geom IS NULL OR ST_Area(h.geom)=0)`, nil},
		{"Partition retry parcels", `CREATE TEMP TABLE qualification_retry_targets ON COMMIT DROP AS
SELECT DISTINCT parcel_id FROM qualification_precision_retry`, nil},
		{"Index retry parcels", `CREATE UNIQUE INDEX ON qualification_retry_targets(parcel_id)`, nil},
	}
	if err := runAssessmentSteps(ctx, tx, gen, initial); err != nil {
		return err
	}
	var retryCount int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM qualification_retry_targets`).Scan(&retryCount); err != nil {
		return err
	}
	if retryCount != 0 {
		log.Printf("[union.qualification.assess] generation=%s precision_retry_parcels=%d", gen, retryCount)
		retry := []assessmentStep{
			{"Reduce retry parcels at fine precision", `UPDATE qualification_targets p
SET analysis_geom=ST_ReducePrecision(p.shape,0.0000001),precision_grid=0.0000001
FROM qualification_retry_targets r WHERE p.id=r.parcel_id`, nil},
			{"Reduce retry zones once per source", `CREATE TEMP TABLE qualification_fine_zones ON COMMIT DROP AS
WITH affected AS MATERIALIZED (
SELECT DISTINCT s.layer_id,s.source_id FROM qualification_zone_sources s
JOIN qualification_retry_targets r ON r.parcel_id=s.parcel_id
)
SELECT a.layer_id,a.source_id,z.geom FROM affected a CROSS JOIN LATERAL (
SELECT ST_ReducePrecision(shape,0.0000001) geom FROM qualification_zones
WHERE generation_id=$1 AND layer_id=a.layer_id AND source_id=a.source_id OFFSET 0
) z`, []any{gen}},
			{"Replace retry zone sources", `UPDATE qualification_zone_sources s SET geom=z.geom
FROM qualification_fine_zones z,qualification_retry_targets r
WHERE s.parcel_id=r.parcel_id AND s.layer_id=z.layer_id AND s.source_id=z.source_id`, nil},
			{"Drop fine zone workspace", `DROP TABLE qualification_fine_zones`, nil},
			{"Reduce retry jurisdictions once per source", `CREATE TEMP TABLE qualification_fine_jurisdictions ON COMMIT DROP AS
WITH affected AS MATERIALIZED (
SELECT DISTINCT s.layer_id,s.source_id FROM qualification_jurisdiction_sources s
JOIN qualification_retry_targets r ON r.parcel_id=s.parcel_id
)
SELECT a.layer_id,a.source_id,j.geom FROM affected a CROSS JOIN LATERAL (
SELECT ST_ReducePrecision(shape,0.0000001) geom FROM qualification_jurisdictions
WHERE generation_id=$1 AND layer_id=a.layer_id AND source_id=a.source_id OFFSET 0
) j`, []any{gen}},
			{"Replace retry jurisdiction sources", `UPDATE qualification_jurisdiction_sources s SET geom=j.geom
FROM qualification_fine_jurisdictions j,qualification_retry_targets r
WHERE s.parcel_id=r.parcel_id AND s.layer_id=j.layer_id AND s.source_id=j.source_id`, nil},
			{"Drop fine jurisdiction workspace", `DROP TABLE qualification_fine_jurisdictions`, nil},
			{"Replace retry zone clips", `DELETE FROM qualification_zone_shapes h USING qualification_retry_targets r WHERE h.parcel_id=r.parcel_id;
INSERT INTO qualification_zone_shapes(parcel_id,layer_id,source_id,jurisdiction,district_code,geom)
SELECT s.parcel_id,s.layer_id,s.source_id,s.jurisdiction,s.district_code,
ST_CollectionExtract(ST_Intersection(p.analysis_geom,s.geom,p.precision_grid),3)
FROM qualification_zone_sources s JOIN qualification_retry_targets r ON r.parcel_id=s.parcel_id
JOIN qualification_targets p ON p.id=s.parcel_id`, nil},
			{"Replace retry jurisdiction clips", `DELETE FROM qualification_jurisdiction_shapes h USING qualification_retry_targets r WHERE h.parcel_id=r.parcel_id;
INSERT INTO qualification_jurisdiction_shapes(parcel_id,layer_id,source_id,name,geom)
SELECT s.parcel_id,s.layer_id,s.source_id,s.name,
ST_CollectionExtract(ST_Intersection(p.analysis_geom,s.geom,p.precision_grid),3)
FROM qualification_jurisdiction_sources s JOIN qualification_retry_targets r ON r.parcel_id=s.parcel_id
JOIN qualification_targets p ON p.id=s.parcel_id`, nil},
		}
		if err := runAssessmentSteps(ctx, tx, gen, retry); err != nil {
			return err
		}
		if err := retryWithoutGrid(ctx, tx, gen); err != nil {
			return err
		}
	}
	statements := []assessmentStep{
		{"Check clipped positive overlaps", precisionCheckSQL("qualification_zone_shapes", "qualification_jurisdiction_shapes", "h.geom IS NULL OR ST_Area(h.geom)<=0", "clipped"), nil},
		{"Remove zero-area zone overlaps", `DELETE FROM qualification_zone_shapes WHERE ST_Area(geom)=0`, nil},
		{"Index zone overlaps", `CREATE INDEX ON qualification_zone_shapes(parcel_id)`, nil},
		{"Analyze zone overlaps", `ANALYZE qualification_zone_shapes`, nil},
		{"Calculate zoning coverage", `CREATE TEMP TABLE qualification_coverage ON COMMIT DROP AS
 SELECT s.parcel_id,ST_CollectionExtract(ST_Intersection(p.analysis_geom,ST_UnaryUnion(ST_Collect(s.geom),p.precision_grid),p.precision_grid),3) geom
 FROM qualification_zone_sources s JOIN qualification_targets p ON p.id=s.parcel_id GROUP BY s.parcel_id,p.analysis_geom,p.precision_grid`, nil},
		{"Calculate zoning conflicts", `CREATE TEMP TABLE qualification_ambiguity ON COMMIT DROP AS
 SELECT conflicts.parcel_id,ST_UnaryUnion(ST_Collect(conflicts.geom),p.precision_grid) geom FROM (
 SELECT a.parcel_id,ST_CollectionExtract(ST_Intersection(p.analysis_geom,ST_CollectionExtract(ST_Intersection(a.geom,b.geom,p.precision_grid),3),p.precision_grid),3) geom
 FROM qualification_zone_sources a JOIN qualification_zone_sources b ON a.parcel_id=b.parcel_id
 JOIN qualification_targets p ON p.id=a.parcel_id AND
 (a.layer_id,a.source_id)<(b.layer_id,b.source_id)
 WHERE (a.jurisdiction,a.district_code) IS DISTINCT FROM (b.jurisdiction,b.district_code)
 AND a.geom && b.geom AND ST_Intersects(a.geom,b.geom)
 UNION ALL SELECT parcel_id,geom FROM qualification_zone_shapes WHERE district_code IS NULL
 ) conflicts JOIN qualification_targets p ON p.id=conflicts.parcel_id WHERE ST_Area(conflicts.geom)>0 GROUP BY conflicts.parcel_id,p.precision_grid`, nil},
		{"Remove zero-area jurisdiction overlaps", `DELETE FROM qualification_jurisdiction_shapes WHERE ST_Area(geom)=0`, nil},
		{"Index jurisdiction overlaps", `CREATE INDEX ON qualification_jurisdiction_shapes(parcel_id)`, nil},
		{"Analyze jurisdiction overlaps", `ANALYZE qualification_jurisdiction_shapes`, nil},
		{"Calculate jurisdiction coverage", `CREATE TEMP TABLE qualification_jurisdiction_coverage ON COMMIT DROP AS
SELECT s.parcel_id,ST_CollectionExtract(ST_Intersection(p.analysis_geom,ST_UnaryUnion(ST_Collect(s.geom),p.precision_grid),p.precision_grid),3) geom
 FROM qualification_jurisdiction_sources s JOIN qualification_targets p ON p.id=s.parcel_id GROUP BY s.parcel_id,p.analysis_geom,p.precision_grid`, nil},
		{"Calculate jurisdiction conflicts", `CREATE TEMP TABLE qualification_jurisdiction_conflicts ON COMMIT DROP AS
SELECT pairs.parcel_id,ST_UnaryUnion(ST_Collect(pairs.geom),p.precision_grid) geom FROM (
SELECT a.parcel_id,ST_CollectionExtract(ST_Intersection(p.analysis_geom,ST_CollectionExtract(ST_Intersection(a.geom,b.geom,p.precision_grid),3),p.precision_grid),3) geom
FROM qualification_jurisdiction_sources a JOIN qualification_jurisdiction_sources b ON a.parcel_id=b.parcel_id
JOIN qualification_targets p ON p.id=a.parcel_id AND
(a.layer_id,a.source_id)<(b.layer_id,b.source_id) AND (a.layer_id,a.name) IS DISTINCT FROM (b.layer_id,b.name)
WHERE a.geom && b.geom AND ST_Intersects(a.geom,b.geom)
UNION ALL SELECT parcel_id,geom FROM qualification_jurisdiction_shapes WHERE name IS NULL OR name=''
) pairs JOIN qualification_targets p ON p.id=pairs.parcel_id WHERE ST_Area(pairs.geom)>0 GROUP BY pairs.parcel_id,p.precision_grid`, nil},
		{"Write zoning assessments", `INSERT INTO qualification_assessments(parcel_id,source_group,generation_id,geometry_fingerprint,method_version,total_acres,unmapped_acres,ambiguous_acres,jurisdiction_unmapped_acres,jurisdiction_ambiguous_acres,has_zone_conflict)
 SELECT p.id,'zoning',$1,p.fingerprint,$2,NULLIF(ST_Area(p.geom)/43560,0),
 CASE WHEN p.geom IS NULL THEN 0 ELSE ST_Area(ST_Transform(ST_Segmentize(ST_Simplify(CASE WHEN c.geom IS NULL THEN p.analysis_geom ELSE ST_Difference(p.analysis_geom,c.geom,p.precision_grid) END,0),10),2264))/43560 END,
 COALESCE(ST_Area(ST_Transform(ST_Segmentize(ST_Simplify(a.geom,0),10),2264))/43560,0),
 CASE WHEN p.geom IS NULL THEN 0 ELSE ST_Area(ST_Transform(ST_Segmentize(ST_Simplify(CASE WHEN jc.geom IS NULL THEN p.analysis_geom ELSE ST_Difference(p.analysis_geom,jc.geom,p.precision_grid) END,0),10),2264))/43560 END,
 COALESCE(ST_Area(ST_Transform(ST_Segmentize(ST_Simplify(ja.geom,0),10),2264))/43560,0),a.geom IS NOT NULL
 FROM qualification_targets p LEFT JOIN qualification_coverage c ON c.parcel_id=p.id
 LEFT JOIN qualification_ambiguity a ON a.parcel_id=p.id
 LEFT JOIN qualification_jurisdiction_coverage jc ON jc.parcel_id=p.id
 LEFT JOIN qualification_jurisdiction_conflicts ja ON ja.parcel_id=p.id
 ON CONFLICT(parcel_id,source_group) DO UPDATE SET generation_id=excluded.generation_id,
 geometry_fingerprint=excluded.geometry_fingerprint,method_version=excluded.method_version,
 calculated_at=now(),total_acres=excluded.total_acres,unmapped_acres=excluded.unmapped_acres,
 ambiguous_acres=excluded.ambiguous_acres,jurisdiction_unmapped_acres=excluded.jurisdiction_unmapped_acres,
 jurisdiction_ambiguous_acres=excluded.jurisdiction_ambiguous_acres,has_zone_conflict=excluded.has_zone_conflict`, []any{gen, method}},
		{"Remove previous zoning matches", `DELETE FROM qualification_zone_hits WHERE parcel_id IN (SELECT id FROM qualification_targets)`, nil},
		{"Remove previous jurisdiction matches", `DELETE FROM qualification_jurisdiction_hits WHERE parcel_id IN (SELECT id FROM qualification_targets)`, nil},
		{"Write zoning matches", `INSERT INTO qualification_zone_hits(parcel_id,layer_id,source_id,jurisdiction,district_code,acres,share)
 SELECT h.parcel_id,h.layer_id,h.source_id,h.jurisdiction,h.district_code,
 m.area_ft2/43560,m.area_ft2/ST_Area(p.geom)
 FROM qualification_zone_shapes h JOIN qualification_targets p ON p.id=h.parcel_id
 CROSS JOIN LATERAL (SELECT CASE WHEN ST_Area(h.geom)*2<=ST_Area(p.analysis_geom)
 THEN ST_Area(ST_Transform(ST_Segmentize(ST_Simplify(h.geom,0),10),2264))
 ELSE ST_Area(p.geom)-ST_Area(ST_Transform(ST_Segmentize(ST_Simplify(ST_Difference(p.analysis_geom,h.geom,p.precision_grid),0),10),2264)) END area_ft2) m
 WHERE ST_Area(p.geom)>0`, nil},
		{"Write jurisdiction matches", `INSERT INTO qualification_jurisdiction_hits(parcel_id,layer_id,source_id,name,acres,share)
SELECT h.parcel_id,h.layer_id,h.source_id,h.name,m.area_ft2/43560,m.area_ft2/ST_Area(p.geom)
FROM qualification_jurisdiction_shapes h JOIN qualification_targets p ON p.id=h.parcel_id
CROSS JOIN LATERAL (SELECT CASE WHEN ST_Area(h.geom)*2<=ST_Area(p.analysis_geom)
THEN ST_Area(ST_Transform(ST_Segmentize(ST_Simplify(h.geom,0),10),2264))
ELSE ST_Area(p.geom)-ST_Area(ST_Transform(ST_Segmentize(ST_Simplify(ST_Difference(p.analysis_geom,h.geom,p.precision_grid),0),10),2264)) END area_ft2) m
WHERE ST_Area(p.geom)>0`, nil},
		{"Check projected positive overlaps", precisionCheckSQL("qualification_zone_hits", "qualification_jurisdiction_hits", "h.acres IS NULL OR h.acres<=0 OR h.share<=0", "projected"), nil},
	}
	return runAssessmentSteps(ctx, tx, gen, statements)
}

// PostGIS uses floating-point overlay when gridSize is -1. This preserves
// the stored coordinates without snap rounding; it is not exact arithmetic.
func retryWithoutGrid(ctx context.Context, tx pgx.Tx, gen string) error {
	steps := []assessmentStep{
		{"Find overlaps still collapsed at fine precision", `CREATE TEMP TABLE qualification_no_grid_evidence ON COMMIT DROP AS
SELECT p.id parcel_id,'parcel'::text kind,NULL::integer layer_id,NULL::bigint source_id,ST_Area(p.shape) original_overlap_m2
FROM qualification_retry_targets r JOIN qualification_targets p ON p.id=r.parcel_id
WHERE p.shape IS NOT NULL AND ST_Area(p.shape)>0 AND (p.analysis_geom IS NULL OR ST_Area(p.analysis_geom)=0)
UNION ALL
SELECT s.parcel_id,'zone',s.layer_id,s.source_id,s.original_area
FROM qualification_retry_targets r JOIN qualification_zone_sources s USING (parcel_id)
LEFT JOIN qualification_zone_shapes h USING (parcel_id,layer_id,source_id)
WHERE s.original_area>0 AND (h.geom IS NULL OR ST_Area(h.geom)=0)
UNION ALL
SELECT s.parcel_id,'jurisdiction',s.layer_id,s.source_id,s.original_area
FROM qualification_retry_targets r JOIN qualification_jurisdiction_sources s USING (parcel_id)
LEFT JOIN qualification_jurisdiction_shapes h USING (parcel_id,layer_id,source_id)
WHERE s.original_area>0 AND (h.geom IS NULL OR ST_Area(h.geom)=0)`, nil},
	}
	if err := runAssessmentSteps(ctx, tx, gen, steps); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT p.source_parcel_id,e.parcel_id::text,e.kind,COALESCE(e.layer_id::text,'-'),COALESCE(e.source_id::text,'-'),e.original_overlap_m2
FROM qualification_no_grid_evidence e JOIN parcels p ON p.id=e.parcel_id`)
	if err != nil {
		return fmt.Errorf("read no-grid retry evidence: %w", err)
	}
	var count int
	for rows.Next() {
		var number, id, kind, layer, source string
		var area float64
		if err := rows.Scan(&number, &id, &kind, &layer, &source, &area); err != nil {
			rows.Close()
			return fmt.Errorf("decode no-grid retry evidence: %w", err)
		}
		count++
		log.Printf("[union.qualification.assess] generation=%s retry=no-grid kind=%s parcel_number=%q parcel_id=%s layer_id=%s source_id=%s original_overlap_m2=%g", gen, kind, number, id, layer, source, area)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read no-grid retry evidence: %w", err)
	}
	if count == 0 {
		return nil
	}

	return runAssessmentSteps(ctx, tx, gen, []assessmentStep{
		{"Restore stored parcel geometry", `UPDATE qualification_targets p
SET analysis_geom=p.shape,precision_grid=-1
FROM (SELECT DISTINCT parcel_id FROM qualification_no_grid_evidence) r WHERE p.id=r.parcel_id`, nil},
		{"Restore stored zone geometry", `UPDATE qualification_zone_sources s SET geom=z.shape
FROM qualification_targets p,qualification_zones z
WHERE p.id=s.parcel_id AND p.precision_grid=-1
AND z.generation_id=$1 AND z.layer_id=s.layer_id AND z.source_id=s.source_id`, []any{gen}},
		{"Restore stored jurisdiction geometry", `UPDATE qualification_jurisdiction_sources s SET geom=j.shape
FROM qualification_targets p,qualification_jurisdictions j
WHERE p.id=s.parcel_id AND p.precision_grid=-1
AND j.generation_id=$1 AND j.layer_id=s.layer_id AND j.source_id=s.source_id`, []any{gen}},
		{"Clip stored zones without a grid", `UPDATE qualification_zone_shapes h
SET geom=ST_CollectionExtract(ST_Intersection(p.analysis_geom,s.geom,p.precision_grid),3)
FROM qualification_zone_sources s JOIN qualification_targets p ON p.id=s.parcel_id
WHERE p.precision_grid=-1 AND (h.parcel_id,h.layer_id,h.source_id)=(s.parcel_id,s.layer_id,s.source_id)`, nil},
		{"Clip stored jurisdictions without a grid", `UPDATE qualification_jurisdiction_shapes h
SET geom=ST_CollectionExtract(ST_Intersection(p.analysis_geom,s.geom,p.precision_grid),3)
FROM qualification_jurisdiction_sources s JOIN qualification_targets p ON p.id=s.parcel_id
WHERE p.precision_grid=-1 AND (h.parcel_id,h.layer_id,h.source_id)=(s.parcel_id,s.layer_id,s.source_id)`, nil},
	})
}

// Validate one required hit per failure. All interpolated SQL is internal.
func precisionCheckSQL(zones, jurisdictions, missing, stage string) string {
	return fmt.Sprintf(`DO $$ DECLARE hit record; BEGIN
SELECT failures.*,p.source_parcel_id INTO hit FROM (
SELECT 'zone' kind,s.parcel_id,s.layer_id,s.source_id,s.original_area
FROM qualification_zone_sources s LEFT JOIN %s h USING (parcel_id,layer_id,source_id)
WHERE s.original_area>0 AND (%s)
UNION ALL
SELECT 'jurisdiction',s.parcel_id,s.layer_id,s.source_id,s.original_area
FROM qualification_jurisdiction_sources s LEFT JOIN %s h USING (parcel_id,layer_id,source_id)
WHERE s.original_area>0 AND (%s)
) failures JOIN parcels p ON p.id=failures.parcel_id LIMIT 1;
IF FOUND THEN RAISE EXCEPTION 'assessment pending precision repair: %s %% hit has no positive area: parcel_number=%% parcel_id=%% layer_id=%% source_id=%% original_overlap_m2=%%',
hit.kind,quote_nullable(hit.source_parcel_id),hit.parcel_id,hit.layer_id,hit.source_id,hit.original_area; END IF;
END $$`, zones, missing, jurisdictions, missing, stage)
}

func (s Store) reconcileRoads(ctx context.Context, tx pgx.Tx, gen, method string) error {
	statements := []assessmentStep{
		{"Calculate distances to Road Centerlines", `CREATE TEMP TABLE qualification_distances ON COMMIT DROP AS
 SELECT p.id parcel_id,r.source_id,r.name,r.road_class,r.status,
 CASE WHEN ST_Intersects(p.shape,r.shape) THEN 0 ELSE ST_Distance(p.geom,ST_Transform(ST_Segmentize(r.shape,10),2264)) END feet
 FROM qualification_targets p CROSS JOIN LATERAL (
 SELECT source_id,name,road_class,status,shape FROM qualification_roads
 WHERE generation_id=$1 AND p.geom IS NOT NULL AND ST_Area(p.geom)>0
 AND shape && ST_Expand(p.shape,8000)
 AND ST_DWithin(p.shape,shape,8000) OFFSET 0
 ) r`, []any{gen}},
		{"Remove roads outside the search radius", `DELETE FROM qualification_distances WHERE feet>5280`, nil},
		{"Index road distances", `CREATE INDEX ON qualification_distances(parcel_id)`, nil},
		{"Analyze road distances", `ANALYZE qualification_distances`, nil},
		{"Write road assessments", `INSERT INTO qualification_assessments(parcel_id,source_group,generation_id,boundary_generation_id,geometry_fingerprint,method_version,total_acres,road_state,road_distance_feet,road_search_radius_feet,county_edge_warning)
 SELECT p.id,'roads',$1,(SELECT generation_id FROM qualification_heads WHERE source_group='zoning'),p.fingerprint,$2,
 NULLIF(ST_Area(p.geom)/43560,0),
 CASE WHEN p.geom IS NULL OR ST_Area(p.geom)=0 THEN 'missing_parcel_geometry'
 WHEN d.feet IS NULL THEN 'none_within_radius' ELSE 'nearest' END,d.feet,5280,
 EXISTS (SELECT 1 FROM (SELECT ST_UnaryUnion(ST_Collect(b.shape)) shape FROM qualification_boundaries b
 JOIN qualification_heads h ON h.source_group='zoning' AND h.generation_id=b.generation_id) b
 WHERE p.geom IS NOT NULL AND (b.shape IS NULL OR NOT ST_Covers(b.shape,p.shape)
 OR ST_Distance(p.geom,ST_Transform(ST_Segmentize(ST_Boundary(b.shape),10),2264))<=5280))
 FROM qualification_targets p LEFT JOIN (SELECT parcel_id,min(feet) feet FROM qualification_distances GROUP BY parcel_id) d ON d.parcel_id=p.id
 ON CONFLICT(parcel_id,source_group) DO UPDATE SET generation_id=excluded.generation_id,boundary_generation_id=excluded.boundary_generation_id,
 geometry_fingerprint=excluded.geometry_fingerprint,method_version=excluded.method_version,calculated_at=now(),
 total_acres=excluded.total_acres,road_state=excluded.road_state,road_distance_feet=excluded.road_distance_feet,
 road_search_radius_feet=excluded.road_search_radius_feet,county_edge_warning=excluded.county_edge_warning`, []any{gen, method}},
		{"Remove previous road matches", `DELETE FROM qualification_road_ties WHERE parcel_id IN (SELECT id FROM qualification_targets)`, nil},
		{"Write nearest road matches", `INSERT INTO qualification_road_ties(parcel_id,source_id,name,road_class,status,distance_feet)
 SELECT d.parcel_id,d.source_id,d.name,d.road_class,d.status,d.feet
 FROM qualification_distances d JOIN qualification_assessments a ON a.parcel_id=d.parcel_id AND a.source_group='roads'
 WHERE d.feet <= a.road_distance_feet+0.01`, nil},
	}
	return runAssessmentSteps(ctx, tx, gen, statements)
}
