package qualification

func roadDistanceSteps(gen string) []assessmentStep {
	return []assessmentStep{
		{"Find candidate Road Centerlines", `CREATE TEMP TABLE qualification_road_candidates ON COMMIT DROP AS
SELECT p.id parcel_id,r.layer_id,r.source_id,ST_Intersects(p.shape,r.shape) intersects
FROM qualification_targets p CROSS JOIN LATERAL (
SELECT layer_id,source_id,shape FROM qualification_roads
WHERE generation_id=$1 AND p.geom IS NOT NULL AND ST_Area(p.geom)>0
AND shape && ST_Expand(p.shape,8000)
AND ST_DWithin(p.shape,shape,8000) OFFSET 0
) r`, []any{gen}},
		{"Project candidate Road Centerlines", `CREATE TEMP TABLE qualification_projected_roads ON COMMIT DROP AS
SELECT r.layer_id,r.source_id,r.name,r.road_class,r.status,
CASE WHEN d.needs_projection THEN ST_Transform(ST_Segmentize(r.shape,10),2264) END geom
FROM (SELECT layer_id,source_id,bool_or(NOT intersects) needs_projection
FROM qualification_road_candidates GROUP BY layer_id,source_id) d
JOIN qualification_roads r USING (layer_id,source_id)
WHERE r.generation_id=$1`, []any{gen}},
		{"Index projected Road Centerlines", `CREATE UNIQUE INDEX ON qualification_projected_roads(layer_id,source_id)`, nil},
		{"Analyze projected Road Centerlines", `ANALYZE qualification_projected_roads`, nil},
		{"Calculate distances to Road Centerlines", `CREATE TEMP TABLE qualification_distances ON COMMIT DROP AS
SELECT c.parcel_id,r.source_id,r.name,r.road_class,r.status,
CASE WHEN c.intersects THEN 0 ELSE ST_Distance(p.geom,r.geom) END feet
FROM qualification_road_candidates c JOIN qualification_projected_roads r USING (layer_id,source_id)
JOIN qualification_targets p ON p.id=c.parcel_id`, nil},
		{"Remove road projection workspace", `DROP TABLE qualification_road_candidates,qualification_projected_roads`, nil},
	}
}
