package qualification

func roadDistanceSteps(gen string) []assessmentStep {
	return []assessmentStep{
		{"Project Road Centerlines", `CREATE TEMP TABLE qualification_projected_roads ON COMMIT DROP AS
SELECT layer_id,source_id,name,road_class,status,shape,
ST_Transform(ST_Segmentize(shape,10),2264) geom
FROM qualification_roads WHERE generation_id=$1`, []any{gen}},
		{"Index projected Road Centerlines", `CREATE INDEX ON qualification_projected_roads USING GIST(geom)`, nil},
		{"Analyze projected Road Centerlines", `ANALYZE qualification_projected_roads`, nil},
		{"Find candidate Road Centerlines", `CREATE TEMP TABLE qualification_road_candidates ON COMMIT DROP AS
SELECT p.id parcel_id,r.layer_id,r.source_id,ST_Intersects(p.shape,r.shape) intersects
FROM qualification_targets p CROSS JOIN LATERAL (
SELECT layer_id,source_id,shape FROM qualification_projected_roads
WHERE p.geom IS NOT NULL AND ST_Area(p.geom)>0
AND geom && ST_Expand(p.geom,5280)
AND ST_DWithin(p.geom,geom,5280) OFFSET 0
) r`, nil},
		{"Calculate distances to Road Centerlines", `CREATE TEMP TABLE qualification_distances ON COMMIT DROP AS
SELECT c.parcel_id,r.source_id,r.name,r.road_class,r.status,
CASE WHEN c.intersects THEN 0 ELSE ST_Distance(p.geom,r.geom) END feet
FROM qualification_road_candidates c JOIN qualification_projected_roads r USING (layer_id,source_id)
JOIN qualification_targets p ON p.id=c.parcel_id`, nil},
		{"Remove road projection workspace", `DROP TABLE qualification_road_candidates,qualification_projected_roads`, nil},
	}
}
