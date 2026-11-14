package soil

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/JustinMonty20/landman/internal/county"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const assessmentMethod = "exclusive-map-unit-union/1; EPSG:2264; acres=ftUS^2/43560"

// Area shares are fractions of geometric parcel area, never confidence estimates.
// Share is unavailable when the parcel has no positive polygonal area.
type Area struct {
	Acres float64  `json:"acres"`
	Share *float64 `json:"share"`
}

type RatingArea struct {
	Rating string `json:"rating"`
	Area   Area   `json:"area"`
}

type PolygonEvidence struct {
	ID             string     `json:"id"`
	SpatialVersion *float64   `json:"spatial_version"`
	SurveySavedOn  *time.Time `json:"survey_saved_on"`
}

type MapUnitAssessment struct {
	MapUnitKey     string            `json:"map_unit_key"`
	Area           Area              `json:"exclusive_area"`
	MappedArea     Area              `json:"mapped_area_including_ambiguity"`
	Polygons       []PolygonEvidence `json:"polygons"`
	Interpretation Interpretation    `json:"interpretation"`
}

// UseAssessment partitions parcel area for one use. Reasons belong to map-unit
// component evidence, not located component acres. Never add areas across uses.
type UseAssessment struct {
	Use       Use                 `json:"use"`
	Rated     []RatingArea        `json:"rated"`
	Unrated   Area                `json:"unrated"`
	Uncovered Area                `json:"uncovered"`
	Ambiguous Area                `json:"ambiguous"`
	MapUnits  []MapUnitAssessment `json:"map_units"`
	NextStep  string              `json:"next_step"`
}

type Freshness struct {
	Stale           bool       `json:"stale"`
	Reasons         []string   `json:"reasons"`
	FreshUntil      time.Time  `json:"fresh_until"`
	RefreshFailedAt *time.Time `json:"refresh_failed_at"`
	RefreshError    *string    `json:"refresh_error"`
}

type ParcelAssessment struct {
	ID                        string          `json:"id"`
	ParcelID                  string          `json:"parcel_id"`
	County                    county.ID       `json:"county"`
	Source                    string          `json:"source"`
	GeometryFingerprint       string          `json:"geometry_fingerprint"`
	TotalAcres                *float64        `json:"total_acres"`
	Available                 bool            `json:"available"`
	SoilSetID                 string          `json:"soil_set_id"`
	SoilSetRevision           string          `json:"soil_set_revision"`
	InterpretationSetRevision string          `json:"interpretation_set_revision"`
	MethodVersion             string          `json:"method_version"`
	CalculatedAt              time.Time       `json:"calculated_at"`
	Uses                      []UseAssessment `json:"uses"`
	Freshness                 Freshness       `json:"freshness"`
	EvidenceLevel             string          `json:"evidence_level"`
	Warnings                  []string        `json:"warnings"`
}

// Reconciliation tolerance covers overlay roundoff, not omitted slivers.
func areaTolerance(total float64) float64 { return math.Max(1e-7, total*1e-8) }

func assessParcels(ctx context.Context, tx pgx.Tx, runID, setID string, countyID county.ID, source string) error {
	// Keep parcel writes after the published head, not between the area snapshot and publication.
	if _, err := tx.Exec(ctx, `LOCK TABLE parcels IN SHARE MODE`); err != nil {
		return fmt.Errorf("soil assessment parcel lock: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO soil_assessment_keys(parcel_id) SELECT id FROM parcels WHERE county_id=$1 AND source_name=$2 ON CONFLICT DO NOTHING`, countyID.String(), source); err != nil {
		return err
	}
	copied, err := copyUnchangedAssessment(ctx, tx, runID, setID, countyID, source)
	if err != nil {
		return err
	}
	if !copied {
		if err := assessChangedParcels(ctx, tx, runID, setID, countyID, source); err != nil {
			return err
		}
	}
	var invalid int
	err = tx.QueryRow(ctx, `SELECT count(*) FROM soil_parcel_assessments a WHERE run_id=$1 AND total_acres IS NOT NULL AND abs(total_acres-uncovered_acres-ambiguous_acres-COALESCE((SELECT sum(exclusive_acres) FROM soil_assessment_map_units m WHERE m.run_id=a.run_id AND m.parcel_id=a.parcel_id),0))>greatest(1e-7,total_acres*1e-8)`, runID).Scan(&invalid)
	if err != nil {
		return err
	}
	if invalid != 0 {
		return fmt.Errorf("soil: %d parcel area partitions failed reconciliation", invalid)
	}
	err = tx.QueryRow(ctx, `SELECT count(*) FROM soil_assessment_map_units m WHERE m.run_id=$1 AND (SELECT count(*) FROM soil_screening_ratings r WHERE r.run_id=m.run_id AND r.map_unit_key=m.map_unit_key)<>3`, runID).Scan(&invalid)
	if err != nil {
		return err
	}
	if invalid != 0 {
		return errors.New("soil: incomplete interpretation coverage")
	}
	return nil
}

// Copy only when every current parcel and the complete soil geometry set are unchanged.
// New interpretations still belong to the new run; only located area is reused.
func copyUnchangedAssessment(ctx context.Context, tx pgx.Tx, runID, setID string, countyID county.ID, source string) (bool, error) {
	var previous string
	err := tx.QueryRow(ctx, `SELECT prev.id::text
 FROM soil_screening_heads h
 JOIN soil_screening_runs prev ON prev.id=h.run_id
 JOIN soil_polygon_sets old_set ON old_set.id=prev.soil_set_id
 JOIN soil_polygon_sets new_set ON new_set.id=$3
 WHERE h.county_id=$1 AND h.source_name=$2
 AND prev.assessment_method=$4 AND old_set.fingerprint=new_set.fingerprint
 AND (SELECT count(*) FROM soil_parcel_assessments a WHERE a.run_id=prev.id) =
     (SELECT count(*) FROM parcels p WHERE p.county_id=$1 AND p.source_name=$2)
 AND NOT EXISTS (
   SELECT 1 FROM parcels p LEFT JOIN soil_parcel_assessments a
   ON a.run_id=prev.id AND a.parcel_id=p.id
   WHERE p.county_id=$1 AND p.source_name=$2
   AND (a.parcel_id IS NULL OR a.geometry_fingerprint IS DISTINCT FROM COALESCE(md5(ST_AsEWKB(p.shape)),'missing'))
 )`, countyID.String(), source, setID, assessmentMethod).Scan(&previous)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO soil_parcel_assessments(run_id,parcel_id,geometry_fingerprint,total_acres,uncovered_acres,ambiguous_acres)
 SELECT $1,parcel_id,geometry_fingerprint,total_acres,uncovered_acres,ambiguous_acres
 FROM soil_parcel_assessments WHERE run_id=$2`, runID, previous); err != nil {
		return false, fmt.Errorf("copy soil assessments: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO soil_assessment_map_units(run_id,parcel_id,map_unit_key,exclusive_acres,mapped_acres,polygon_ids)
 SELECT $1,parcel_id,map_unit_key,exclusive_acres,mapped_acres,polygon_ids
 FROM soil_assessment_map_units WHERE run_id=$2`, runID, previous); err != nil {
		return false, fmt.Errorf("copy soil map-unit areas: %w", err)
	}
	return true, nil
}

func parcelArea(acres float64, total *float64) Area {
	result := Area{Acres: acres}
	if total != nil {
		share := acres / *total
		result.Share = &share
	}
	return result
}

// CurrentAssessment reads a complete published generation in a consistent snapshot.
// pgx.ErrNoRows means the parcel has never been assessed in the current generation.
// Geometry, geometry-set, method, age, and refresh failures independently mark old
// evidence stale. Reading never silently recomputes or upgrades mapped evidence.
func CurrentAssessment(ctx context.Context, pool *pgxpool.Pool, countyID county.ID, source, parcelID string) (*ParcelAssessment, error) {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(context.Background())
	result := &ParcelAssessment{ParcelID: parcelID, County: countyID, Source: source, EvidenceLevel: "mapped-survey", Uses: []UseAssessment{}, Warnings: []string{"Mapped survey evidence is not site verified", "County geometry and USDA survey revision compatibility is unverified", "Survey detail unavailable; no jurisdictional wetlands or permit determination"}}
	var uncovered, ambiguous float64
	var currentGeometry, currentSet string
	err = tx.QueryRow(ctx, `SELECT k.id::text,a.geometry_fingerprint,a.total_acres,a.uncovered_acres,a.ambiguous_acres,a.calculated_at,
 s.id::text,s.soil_set_id::text,g.fingerprint,s.assessment_method,s.fresh_until,
 COALESCE(md5(ST_AsEWKB(p.shape)),'missing'),COALESCE(cg.fingerprint,''),f.failed_at,f.error
 FROM soil_screening_heads h JOIN soil_screening_runs s ON s.id=h.run_id
 JOIN soil_parcel_assessments a ON a.run_id=s.id JOIN soil_assessment_keys k ON k.parcel_id=a.parcel_id
 JOIN parcels p ON p.id=a.parcel_id JOIN soil_polygon_sets g ON g.id=s.soil_set_id
 LEFT JOIN soil_polygon_set_heads ch ON ch.county_id=h.county_id AND ch.source_name=h.source_name
 LEFT JOIN soil_polygon_sets cg ON cg.id=ch.set_id
 LEFT JOIN soil_screening_refresh_status f ON f.county_id=h.county_id AND f.source_name=h.source_name
 WHERE h.county_id=$1 AND h.source_name=$2 AND a.parcel_id=$3`, countyID.String(), source, parcelID).Scan(&result.ID, &result.GeometryFingerprint, &result.TotalAcres, &uncovered, &ambiguous, &result.CalculatedAt, &result.InterpretationSetRevision, &result.SoilSetID, &result.SoilSetRevision, &result.MethodVersion, &result.Freshness.FreshUntil, &currentGeometry, &currentSet, &result.Freshness.RefreshFailedAt, &result.Freshness.RefreshError)
	if err != nil {
		return nil, err
	}
	result.Available = result.TotalAcres != nil
	if !result.Available {
		result.Warnings = append(result.Warnings, "Parcel geometry is missing or has zero polygonal area")
	}
	freshness := &result.Freshness
	freshness.Reasons = []string{}
	if currentGeometry != result.GeometryFingerprint {
		freshness.Reasons = append(freshness.Reasons, "parcel geometry changed")
	}
	if currentSet != result.SoilSetRevision {
		freshness.Reasons = append(freshness.Reasons, "soil geometry set changed")
	}
	if result.MethodVersion != assessmentMethod {
		freshness.Reasons = append(freshness.Reasons, "assessment method changed")
	}
	if time.Now().After(freshness.FreshUntil) {
		freshness.Reasons = append(freshness.Reasons, "evidence freshness interval expired")
	}
	if freshness.RefreshFailedAt != nil {
		freshness.Reasons = append(freshness.Reasons, "required refresh failed")
	}
	freshness.Stale = len(freshness.Reasons) > 0
	byUse := map[Use][]MapUnitAssessment{}
	rows, err := tx.Query(ctx, `SELECT m.map_unit_key,m.exclusive_acres,m.mapped_acres,r.provenance,r.interpretation_id::text,
 (SELECT COALESCE(jsonb_agg(jsonb_build_object('sequence',c.sequence,'key',c.component_key,'name',c.name,'percent',c.percent,'availability',c.availability,'original_rating',c.original_rating,'rating',c.rating,'reason_key',c.reason_key,'reason_name',c.reason_name,'reason',c.reason,'severity',c.severity) ORDER BY c.component_key,c.sequence),'[]'::jsonb) FROM soil_component_facts c WHERE c.run_id=r.run_id AND c.interpretation_id=r.interpretation_id),
 (SELECT COALESCE(jsonb_agg(x.payload ORDER BY x.component_key,x.kind,x.position),'[]'::jsonb) FROM soil_measurements x WHERE x.run_id=r.run_id AND x.interpretation_id=r.interpretation_id),
 (SELECT jsonb_agg(jsonb_build_object('id',sm.polygon_id,'spatial_version',sm.spatial_version,'survey_saved_on',sm.survey_saved_on::timestamp AT TIME ZONE 'UTC') ORDER BY sm.polygon_id) FROM soil_polygon_set_members sm WHERE sm.set_id=$3 AND sm.polygon_id=ANY(m.polygon_ids))
 FROM soil_assessment_map_units m JOIN soil_screening_ratings r ON r.run_id=m.run_id AND r.map_unit_key=m.map_unit_key
 WHERE m.run_id=$1 AND m.parcel_id=$2 ORDER BY m.map_unit_key,r.rule_key`, result.InterpretationSetRevision, parcelID, result.SoilSetID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var item MapUnitAssessment
		var exclusive, mapped float64
		var provenance, components, measurements, polygons []byte
		var interpretationID string
		if err = rows.Scan(&item.MapUnitKey, &exclusive, &mapped, &provenance, &interpretationID, &components, &measurements, &polygons); err != nil {
			rows.Close()
			return nil, err
		}
		if err = json.Unmarshal(provenance, &item.Interpretation); err != nil {
			rows.Close()
			return nil, err
		}
		item.Interpretation.ID = interpretationID
		if err = json.Unmarshal(components, &item.Interpretation.Components); err != nil {
			rows.Close()
			return nil, err
		}
		if err = json.Unmarshal(measurements, &item.Interpretation.Measurements); err != nil {
			rows.Close()
			return nil, err
		}
		if err = json.Unmarshal(polygons, &item.Polygons); err != nil {
			rows.Close()
			return nil, err
		}
		item.Area = parcelArea(exclusive, result.TotalAcres)
		item.MappedArea = parcelArea(mapped, result.TotalAcres)
		for _, p := range item.Polygons {
			survey := item.Interpretation.Revision.SurveySavedOn
			if p.SurveySavedOn == nil || survey == nil {
				result.Warnings = append(result.Warnings, "Polygon or USDA survey date unavailable")
			} else if p.SurveySavedOn.Format("2006-01-02") != survey.Format("2006-01-02") {
				result.Warnings = append(result.Warnings, "Polygon and USDA survey dates differ")
			}
		}
		byUse[item.Interpretation.Use] = append(byUse[item.Interpretation.Use], item)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, err
	}
	for _, rule := range screeningRules {
		use := UseAssessment{Use: rule.use, Unrated: parcelArea(0, result.TotalAcres), Uncovered: parcelArea(uncovered, result.TotalAcres), Ambiguous: parcelArea(ambiguous, result.TotalAcres), MapUnits: byUse[rule.use], Rated: []RatingArea{}, NextStep: "Site-specific site/geotechnical review"}
		if use.MapUnits == nil {
			use.MapUnits = []MapUnitAssessment{}
		}
		if rule.use == ConventionalSeptic {
			use.NextStep = "Site-specific soil/septic evaluation; mapped limitations do not determine approval"
		}
		classes := map[string]float64{}
		sum := uncovered + ambiguous
		for _, m := range use.MapUnits {
			sum += m.Area.Acres
			if m.Interpretation.Availability == Rated {
				classes[m.Interpretation.Rating] += m.Area.Acres
			} else {
				use.Unrated.Acres += m.Area.Acres
			}
		}
		use.Unrated = parcelArea(use.Unrated.Acres, result.TotalAcres)
		for _, class := range []string{"Not limited", "Somewhat limited", "Very limited"} {
			if acres, ok := classes[class]; ok {
				use.Rated = append(use.Rated, RatingArea{Rating: class, Area: parcelArea(acres, result.TotalAcres)})
			}
		}
		if result.TotalAcres != nil && math.Abs(sum-*result.TotalAcres) > areaTolerance(*result.TotalAcres) {
			return nil, errors.New("stored assessment partitions do not reconcile")
		}
		result.Uses = append(result.Uses, use)
	}
	sort.Strings(result.Warnings)
	result.Warnings = compactStrings(result.Warnings)
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return result, nil
}

func compactStrings(values []string) []string {
	result := values[:0]
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}
