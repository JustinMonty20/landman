package vacancy

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const AssessmentJobID = "union.gis.vacancy_assessments"

// Publish derives parcel-keyed assessments only from the current two admitted
// live captures. It deliberately cannot publish fixture generations. All writes
// and validation rechecks happen on one transaction; a failed refresh leaves
// the prior rows, which readers must independently check for freshness.
func Publish(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var structures, values string
	var year int
	err = tx.QueryRow(ctx, `SELECT s.generation_id::text,a.generation_id::text,a.effective_av_year
 FROM vacancy_live_validation s JOIN vacancy_live_generations sg ON sg.id=s.generation_id
 JOIN vacancy_latest_observations sh ON sh.source_name=sg.source_name AND sh.capture_id=sg.capture_id
 JOIN vacancy_live_validation a ON a.source_name='union_assessed_value_0'
 JOIN vacancy_live_generations ag ON ag.id=a.generation_id
 JOIN vacancy_latest_observations ah ON ah.source_name=ag.source_name AND ah.capture_id=ag.capture_id
 WHERE s.source_name='union_structures_171' AND s.capture_contract_version=1 AND a.capture_contract_version=1
 AND s.normalized_sha256=sg.normalized_sha256 AND a.normalized_sha256=ag.normalized_sha256
 AND sg.normalizer_version='vacancy-rows-v1' AND ag.normalizer_version='vacancy-rows-v1'
 ORDER BY sg.retrieved_at DESC,ag.retrieved_at DESC LIMIT 1`).Scan(&structures, &values, &year)
	if errors.Is(err, pgx.ErrNoRows) {
		return errors.New("both current live sources require reviewed validation and an approved AV year")
	}
	if err != nil {
		return err
	}
	var retained, observed int
	if err = tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM vacancy_live_shapes WHERE generation_id=$1::uuid),
 (SELECT observed_count FROM vacancy_live_generations WHERE id=$1::uuid)`, structures).Scan(&retained, &observed); err != nil {
		return err
	}
	if retained != 0 && retained != observed {
		return errors.New("incomplete retained vacancy shapes; refuse publication")
	}
	if retained == 0 {
		// Multi-ring ArcGIS outlines have ambiguous shell/hole identity. Keep the
		// raw rings but do not turn them into evidence of absence.
		_, err = tx.Exec(ctx, `INSERT INTO vacancy_live_shapes(generation_id,ordinal,shape)
 SELECT generation_id,ordinal,
 CASE WHEN source_crs=3857 AND jsonb_typeof(geometry->'rings')='array'
 AND jsonb_array_length(geometry->'rings')=1 THEN
 ST_SetSRID(ST_GeomFromGeoJSON(jsonb_build_object('type','Polygon','coordinates',geometry->'rings')::text),3857)
 END FROM vacancy_live_structures WHERE generation_id=$1::uuid`, structures)
		if err != nil {
			return err
		}
		// Invalid footprints cannot be treated as absent from any parcel.
		_, err = tx.Exec(ctx, `UPDATE vacancy_live_shapes SET shape=NULL WHERE generation_id=$1::uuid AND shape IS NOT NULL
 AND (NOT ST_IsValid(shape) OR ST_IsEmpty(shape) OR ST_Area(shape)=0)`, structures)
		if err != nil {
			return err
		}
	}
	_, err = tx.Exec(ctx, publicationSQL, structures, values, year, RuleVersion)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

const publicationSQL = `WITH source_state AS MATERIALIZED (
 SELECT EXISTS(SELECT 1 FROM vacancy_live_shapes WHERE generation_id=$1::uuid AND shape IS NULL) AS source_shape_incomplete
), evidence AS (
 SELECT p.id,x.source_shape_incomplete,p.source_parcel_id,p.shape,p.year_built,p.sqft,p.structure,p.structure_style,
 c.observation_id::text || ':' || c.normalized_hash || ':' || c.geometry_hash AS parcel_generation,
 EXISTS (SELECT 1 FROM parcels other WHERE other.id<>p.id AND other.county_id=p.county_id
 AND other.source_name=p.source_name AND other.source_parcel_id=p.source_parcel_id) AS duplicate_pid,
 (SELECT count(DISTINCT (r.market_building::text)) FROM vacancy_live_assessments r
 WHERE r.generation_id=$2::uuid AND r.parcel_number=p.source_parcel_id
 AND r.parcel_year=$3 AND r.assess_category='AV') AS value_count,
 EXISTS (SELECT 1 FROM vacancy_live_assessments r WHERE r.generation_id=$2::uuid
 AND r.parcel_number=p.source_parcel_id AND r.parcel_year=$3 AND r.assess_category='AV'
 AND r.market_building IS NULL) AS missing_building_value,
 (SELECT min(r.market_building) FROM vacancy_live_assessments r
 WHERE r.generation_id=$2::uuid AND r.parcel_number=p.source_parcel_id
 AND r.parcel_year=$3 AND r.assess_category='AV') AS building_value,
 EXISTS (SELECT 1 FROM vacancy_live_structures r JOIN vacancy_live_shapes sh
 ON sh.generation_id=r.generation_id AND sh.ordinal=r.ordinal
 WHERE r.generation_id=$1::uuid AND sh.shape IS NOT NULL AND p.shape && sh.shape
 AND ST_Relate(p.shape,sh.shape,'2********')) AS footprint,
 (EXISTS (SELECT 1 FROM vacancy_live_structures r JOIN vacancy_live_shapes sh
 ON sh.generation_id=r.generation_id AND sh.ordinal=r.ordinal
 WHERE r.generation_id=$1::uuid AND r.pin=p.source_parcel_id
 AND (sh.shape IS NULL OR NOT (p.shape && sh.shape AND ST_Relate(p.shape,sh.shape,'2********'))
 OR r.status IS NULL OR r.status <> ALL(((SELECT current_statuses FROM vacancy_live_validation WHERE generation_id=$1::uuid))::text[])
 OR EXISTS (SELECT 1 FROM parcels other WHERE other.id<>p.id AND other.county_id='union' AND other.source_name='union_county_gis'
 AND sh.shape IS NOT NULL AND other.shape && sh.shape AND parcel_boundary_usable(other.shape)
 AND ST_Relate(other.shape,sh.shape,'2********'))))
 OR EXISTS (SELECT 1 FROM vacancy_live_shapes sh JOIN vacancy_live_structures r
 ON r.generation_id=sh.generation_id AND r.ordinal=sh.ordinal
 WHERE sh.generation_id=$1::uuid AND sh.shape && p.shape AND ST_Relate(p.shape,sh.shape,'2********')
 AND (r.pin IS DISTINCT FROM p.source_parcel_id OR r.status IS NULL
 OR r.status <> ALL(((SELECT current_statuses FROM vacancy_live_validation WHERE generation_id=$1::uuid))::text[])
 OR EXISTS (SELECT 1 FROM parcels other WHERE other.id<>p.id AND other.county_id='union' AND other.source_name='union_county_gis'
 AND other.shape && sh.shape AND parcel_boundary_usable(other.shape)
 AND ST_Relate(other.shape,sh.shape,'2********'))))) AS uncertain_footprint
 FROM parcels p CROSS JOIN source_state x LEFT JOIN parcel_evidence_current c ON c.county_id=p.county_id AND c.source_name=p.source_name AND c.source_parcel_id=p.source_parcel_id
 WHERE p.county_id='union' AND p.source_name='union_county_gis'
), selected AS (
 SELECT *, (coalesce(year_built,0)>0 OR coalesce(sqft,0)>0 OR coalesce(structure,'')<>'' OR coalesce(structure_style,'')<>'') AS parcel_reports_structure,
 (building_value>0) AS positive_building_value FROM evidence
), classified AS (
 SELECT *, CASE
 WHEN parcel_generation IS NULL OR shape IS NULL OR NOT parcel_boundary_usable(shape) OR duplicate_pid OR uncertain_footprint OR source_shape_incomplete
 OR year_built IS NULL OR sqft IS NULL OR value_count<>1 OR missing_building_value OR building_value IS NULL THEN 'unknown'
 WHEN footprint <> parcel_reports_structure OR footprint <> positive_building_value THEN 'conflict'
 WHEN NOT footprint THEN 'strong_candidate'
 ELSE 'review' END AS tier FROM selected
)
 INSERT INTO vacancy_assessments AS current(parcel_id,tier,rule_version,parcel_generation,structure_generation,assessment_generation,facts,reason,assessed_at)
 SELECT id,tier,$4,coalesce(parcel_generation,'missing'),$1::uuid,$2::uuid,
 jsonb_build_object('year_built',year_built,'sqft',sqft,'building_value',building_value,'effective_av_year',$3,
 'overlapping_footprint',footprint,'uncertain_footprint',uncertain_footprint,'source_shape_incomplete',source_shape_incomplete,'parcel_reports_structure',parcel_reports_structure),
 CASE tier WHEN 'unknown' THEN 'necessary_evidence_missing_or_identity_ambiguous'
 WHEN 'conflict' THEN 'independent_building_evidence_disagrees'
 WHEN 'review' THEN 'building_evidence_requires_review'
 ELSE 'no_reported_structure_or_positive_building_value_with_verified_coverage' END,now()
 FROM classified
 ON CONFLICT(parcel_id) DO UPDATE SET tier=EXCLUDED.tier,rule_version=EXCLUDED.rule_version,
 parcel_generation=EXCLUDED.parcel_generation,structure_generation=EXCLUDED.structure_generation,
 assessment_generation=EXCLUDED.assessment_generation,facts=EXCLUDED.facts,reason=EXCLUDED.reason,assessed_at=EXCLUDED.assessed_at
 WHERE (current.tier,current.rule_version,current.parcel_generation,current.structure_generation,current.assessment_generation,current.facts,current.reason)
 IS DISTINCT FROM (EXCLUDED.tier,EXCLUDED.rule_version,EXCLUDED.parcel_generation,EXCLUDED.structure_generation,EXCLUDED.assessment_generation,EXCLUDED.facts,EXCLUDED.reason)`
