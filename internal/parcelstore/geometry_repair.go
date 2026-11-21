package parcelstore

// repairParcelGeometrySQL returns NULL for a rejected parcel and the ordinals of
// the features that required repair. It leaves valid features untouched.
const repairParcelGeometrySQL = `
WITH parts AS MATERIALIZED (
 SELECT ordinal, ST_GeomFromEWKB(shape) AS g FROM parcel_import_parts WHERE source_parcel_id=$1
), classified AS MATERIALIZED (
 SELECT ordinal, g, ST_IsValidDetail(g) AS validity FROM parts
), candidates AS MATERIALIZED (
 SELECT ordinal, g, ST_MakeValid(g,'method=linework') AS l,
 ST_MakeValid(g,'method=structure keepcollapsed=true') AS s
 FROM classified WHERE (validity).reason='Ring Self-intersection'
), comparable AS MATERIALIZED (
 SELECT ordinal,g,l,s FROM candidates
 WHERE ST_SRID(l)=3857 AND ST_SRID(s)=3857 AND ST_NDims(l)=2 AND ST_NDims(s)=2
 AND ST_GeometryType(l) IN ('ST_Polygon','ST_MultiPolygon')
 AND ST_GeometryType(s) IN ('ST_Polygon','ST_MultiPolygon')
 AND NOT ST_IsEmpty(l) AND NOT ST_IsEmpty(s) AND ST_IsValid(l) AND ST_IsValid(s)
), repaired AS MATERIALIZED (
 SELECT ordinal,l AS g FROM comparable WHERE ST_Equals(l,s) AND NOT EXISTS (
  SELECT 1 FROM (VALUES(l),(s)) AS v(candidate)
  WHERE (SELECT array_agg(ST_AsBinary(d.geom,'XDR') ORDER BY ST_AsBinary(d.geom,'XDR'))
         FROM ST_DumpSegments(ST_ForcePolygonCW(g)) d) IS DISTINCT FROM
        (SELECT array_agg(ST_AsBinary(d.geom,'XDR') ORDER BY ST_AsBinary(d.geom,'XDR'))
         FROM ST_DumpSegments(ST_ForcePolygonCW(v.candidate)) d)
 )
), accepted AS MATERIALIZED (
 SELECT ordinal,g FROM classified WHERE (validity).valid AND NOT ST_IsEmpty(g)
 AND ST_SRID(g)=3857 AND ST_NDims(g)=2 AND ST_GeometryType(g) IN ('ST_Polygon','ST_MultiPolygon')
 UNION ALL SELECT ordinal,g FROM repaired
), combined AS MATERIALIZED (
 SELECT CASE WHEN count(*)=(SELECT count(*) FROM parts)
 THEN ST_Normalize(ST_Multi(ST_Union(g))) END AS g FROM accepted
)
SELECT CASE WHEN NOT ST_IsEmpty(g) AND (ST_IsValidDetail(g)).valid
 AND ST_SRID(g)=3857 AND ST_NDims(g)=2 AND ST_GeometryType(g)='ST_MultiPolygon'
 THEN ST_AsEWKB(g) END,
 ARRAY(SELECT ordinal FROM repaired ORDER BY ordinal) FROM combined`
