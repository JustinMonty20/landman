package qualification

import "strings"

// Coverage reuses unions only when the complete source set and precision match.
func coverageSQL(workspace, result string) string {
	return strings.NewReplacer("@workspace", workspace, "@result", result).Replace(`CREATE TEMP TABLE @result ON COMMIT DROP AS
WITH covered AS MATERIALIZED (
SELECT DISTINCT h.parcel_id FROM @workspace_shapes h JOIN qualification_targets p ON p.id=h.parcel_id
JOIN @workspace_geometry s ON (s.layer_id,s.source_id,s.precision_grid)=(h.layer_id,h.source_id,p.precision_grid)
WHERE CASE WHEN p.precision_grid>0 AND ST_Equals(h.geom,p.analysis_geom) THEN ST_Covers(s.geom,p.analysis_geom) ELSE false END
), candidates AS MATERIALIZED (
SELECT s.parcel_id,p.precision_grid,array_agg(ARRAY[s.layer_id::bigint,s.source_id] ORDER BY s.layer_id,s.source_id) source_keys
FROM @workspace_candidates s JOIN qualification_targets p ON p.id=s.parcel_id
WHERE NOT EXISTS (SELECT 1 FROM covered d WHERE d.parcel_id=s.parcel_id)
GROUP BY s.parcel_id,p.precision_grid
), representatives AS (
SELECT DISTINCT ON (precision_grid,source_keys) precision_grid,source_keys,parcel_id
FROM candidates ORDER BY precision_grid,source_keys,parcel_id
), merged AS MATERIALIZED (
SELECT r.precision_grid,r.source_keys,u.geom FROM representatives r CROSS JOIN LATERAL (
SELECT ST_UnaryUnion(ST_Collect(g.geom ORDER BY g.layer_id,g.source_id),r.precision_grid) geom
FROM @workspace_candidates s JOIN @workspace_geometry g
ON (g.layer_id,g.source_id,g.precision_grid)=(s.layer_id,s.source_id,r.precision_grid)
WHERE s.parcel_id=r.parcel_id
) u
)
SELECT c.parcel_id,ST_CollectionExtract(ST_Intersection(p.analysis_geom,m.geom,c.precision_grid),3) geom
FROM candidates c JOIN merged m USING (precision_grid,source_keys) JOIN qualification_targets p ON p.id=c.parcel_id
UNION ALL SELECT d.parcel_id,p.analysis_geom FROM covered d JOIN qualification_targets p ON p.id=d.parcel_id`)
}

// Within a generation, an ordered source pair and precision identify one intersection.
func conflictSteps(label, workspace, result, distinct, unknown string) []assessmentStep {
	sql := strings.NewReplacer("@workspace", workspace, "@result", result, "@distinct", distinct, "@unknown", unknown).Replace
	return []assessmentStep{
		{"Find " + label + " conflict parcel pairs", sql(`CREATE TEMP TABLE @workspace_pair_parcels ON COMMIT DROP AS
SELECT a.parcel_id,a.layer_id a_layer,a.source_id a_source,b.layer_id b_layer,b.source_id b_source,p.precision_grid
FROM @workspace_candidates a JOIN @workspace_candidates b ON a.parcel_id=b.parcel_id
JOIN qualification_targets p ON p.id=a.parcel_id
WHERE (a.layer_id,a.source_id)<(b.layer_id,b.source_id) AND @distinct`), nil},
		{"Cache " + label + " conflict source pairs", sql(`CREATE TEMP TABLE @workspace_pair_overlaps ON COMMIT DROP AS
WITH representatives AS MATERIALIZED (
SELECT DISTINCT a_layer,a_source,b_layer,b_source,precision_grid
FROM @workspace_pair_parcels
)
SELECT r.a_layer,r.a_source,r.b_layer,r.b_source,r.precision_grid,
ST_CollectionExtract(ST_Intersection(a.geom,b.geom,r.precision_grid),3) geom
FROM representatives r
JOIN @workspace_geometry a ON (a.layer_id,a.source_id,a.precision_grid)=(r.a_layer,r.a_source,r.precision_grid)
JOIN @workspace_geometry b ON (b.layer_id,b.source_id,b.precision_grid)=(r.b_layer,r.b_source,r.precision_grid)
WHERE a.geom && b.geom AND ST_Intersects(a.geom,b.geom)`), nil},
		{"Index cached " + label + " conflicts", sql(`CREATE INDEX ON @workspace_pair_overlaps(a_layer,a_source,b_layer,b_source,precision_grid)`), nil},
		{"Analyze cached " + label + " conflicts", sql(`ANALYZE @workspace_pair_overlaps`), nil},
		{"Clip " + label + " conflicts to parcels", sql(`CREATE TEMP TABLE @workspace_pair_clips ON COMMIT DROP AS
SELECT k.parcel_id,ST_CollectionExtract(ST_Intersection(p.analysis_geom,o.geom,p.precision_grid),3) geom
FROM @workspace_pair_parcels k
JOIN @workspace_pair_overlaps o ON (o.a_layer,o.a_source,o.b_layer,o.b_source,o.precision_grid)=
(k.a_layer,k.a_source,k.b_layer,k.b_source,k.precision_grid)
JOIN qualification_targets p ON p.id=k.parcel_id
WHERE NOT ST_IsEmpty(o.geom)`), nil},
		{"Calculate " + label + " conflicts", sql(`CREATE TEMP TABLE @result ON COMMIT DROP AS
SELECT conflicts.parcel_id,ST_UnaryUnion(ST_Collect(conflicts.geom),p.precision_grid) geom FROM (
SELECT parcel_id,geom FROM @workspace_pair_clips
UNION ALL SELECT parcel_id,geom FROM @workspace_shapes WHERE @unknown
) conflicts JOIN qualification_targets p ON p.id=conflicts.parcel_id WHERE ST_Area(conflicts.geom)>0 GROUP BY conflicts.parcel_id,p.precision_grid`), nil},
	}
}
