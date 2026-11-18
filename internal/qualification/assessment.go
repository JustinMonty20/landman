package qualification

import (
	"context"
	"errors"
	"time"

	"github.com/JustinMonty20/landman/internal/connector/union"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Freshness struct {
	Stale           bool       `json:"stale"`
	Reasons         []string   `json:"reasons"`
	SourceFetchedAt *time.Time `json:"source_fetched_at"`
	RefreshFailedAt *time.Time `json:"refresh_failed_at"`
	RefreshError    *string    `json:"refresh_error"`
}
type Area struct {
	Acres float64 `json:"acres"`
	Share float64 `json:"share"`
}
type ZoneHit struct {
	LayerID       int     `json:"layer_id"`
	SourceID      int64   `json:"source_id"`
	Jurisdiction  string  `json:"jurisdiction"`
	DistrictCode  *string `json:"district_code"`
	Description   *string `json:"description"`
	CaseReference *string `json:"case_reference"`
	OfficialURL   *string `json:"official_url"`
	SourceDate    *string `json:"source_date"`
	Area          Area    `json:"area"`
}
type JurisdictionHit struct {
	LayerID    int     `json:"layer_id"`
	SourceID   int64   `json:"source_id"`
	Name       string  `json:"name"`
	SourceDate *string `json:"source_date"`
	Area       Area    `json:"area"`
}
type RoadTie struct {
	SourceID            int64 `json:"source_id"`
	Name, Class, Status *string
	SourceDate          *time.Time `json:"source_date"`
	DistanceFeet        float64    `json:"distance_feet"`
}
type ZoningAssessment struct {
	ParcelID                   string            `json:"parcel_id"`
	Available                  bool              `json:"available"`
	GenerationID               string            `json:"generation_id"`
	GeometryFingerprint        string            `json:"geometry_fingerprint"`
	MethodVersion              string            `json:"method_version"`
	CalculatedAt               *time.Time        `json:"calculated_at"`
	TotalAcres                 *float64          `json:"total_acres"`
	UnmappedAcres              float64           `json:"unmapped_acres"`
	AmbiguousAcres             float64           `json:"ambiguous_acres"`
	JurisdictionUnmappedAcres  float64           `json:"jurisdiction_unmapped_acres"`
	JurisdictionAmbiguousAcres float64           `json:"jurisdiction_ambiguous_acres"`
	Split                      bool              `json:"split"`
	Zones                      []ZoneHit         `json:"zones"`
	Jurisdictions              []JurisdictionHit `json:"jurisdictions"`
	Freshness                  Freshness         `json:"freshness"`
}

// RoadState distinguishes a published source awaiting assessment from an absent source.
type RoadState string

const (
	RoadSourceUnavailable     RoadState = "source_unavailable"
	RoadPendingAssessment     RoadState = "pending_assessment"
	RoadNearest               RoadState = "nearest"
	RoadNoneWithinRadius      RoadState = "none_within_radius"
	RoadMissingParcelGeometry RoadState = "missing_parcel_geometry"
)

type RoadAssessment struct {
	ParcelID             string     `json:"parcel_id"`
	Available            bool       `json:"available"`
	GenerationID         string     `json:"generation_id"`
	BoundaryGenerationID *string    `json:"boundary_generation_id"`
	GeometryFingerprint  string     `json:"geometry_fingerprint"`
	MethodVersion        string     `json:"method_version"`
	CalculatedAt         *time.Time `json:"calculated_at"`
	State                RoadState  `json:"state"`
	DistanceFeet         *float64   `json:"distance_feet"`
	SearchRadiusFeet     int        `json:"search_radius_feet"`
	CountyEdgeWarning    bool       `json:"county_edge_warning"`
	Ties                 []RoadTie  `json:"ties"`
	Freshness            Freshness  `json:"freshness"`
}

type currentRow struct {
	id, version, geometry string
	calculated            time.Time
	total                 *float64
	fresh                 Freshness
	boundary              *string
}

func current(ctx context.Context, tx pgx.Tx, parcelID, group, method string) (currentRow, error) {
	row := currentRow{fresh: Freshness{Reasons: []string{}}}
	var head, actual, headBoundary *string
	var fetched, boundaryFetched, boundaryFailed *time.Time
	err := tx.QueryRow(ctx, `SELECT a.generation_id::text,a.geometry_fingerprint,a.method_version,a.calculated_at,a.total_acres,
 h.generation_id::text,COALESCE(md5(ST_AsEWKB(p.shape)),'missing'),g.fetched_at,f.failed_at,f.error,
 a.boundary_generation_id::text,zh.generation_id::text,zg.fetched_at,zf.failed_at
 FROM qualification_assessments a JOIN parcels p ON p.id=a.parcel_id
 LEFT JOIN qualification_heads h ON h.source_group=a.source_group
 LEFT JOIN qualification_generations g ON g.id=a.generation_id
 LEFT JOIN qualification_refresh_failures f ON f.source_group=a.source_group
 LEFT JOIN qualification_heads zh ON zh.source_group='zoning'
 LEFT JOIN qualification_generations zg ON zg.id=a.boundary_generation_id
 LEFT JOIN qualification_refresh_failures zf ON zf.source_group='zoning'
 WHERE a.parcel_id=$1 AND a.source_group=$2`, parcelID, group).Scan(&row.id, &row.geometry, &row.version, &row.calculated, &row.total, &head, &actual, &fetched, &row.fresh.RefreshFailedAt, &row.fresh.RefreshError, &row.boundary, &headBoundary, &boundaryFetched, &boundaryFailed)
	if err != nil {
		return row, err
	}
	row.fresh.SourceFetchedAt = fetched
	add := func(reason string) { row.fresh.Stale = true; row.fresh.Reasons = append(row.fresh.Reasons, reason) }
	if head == nil || *head != row.id {
		add("source_generation_changed")
	}
	if actual == nil || row.geometry != *actual {
		add("parcel_geometry_changed")
	}
	if row.version != method {
		add("method_changed")
	}
	if fetched == nil || time.Since(*fetched) > maxSourceAge {
		add("source_age")
	}
	if row.total == nil {
		add("parcel_geometry_missing_or_invalid")
	}
	if row.fresh.RefreshFailedAt != nil {
		add("source_refresh_failed")
	}
	if group == union.QualificationRoads {
		if (row.boundary == nil) != (headBoundary == nil) || (row.boundary != nil && headBoundary != nil && *row.boundary != *headBoundary) {
			add("boundary_generation_changed")
		}
		if boundaryFetched == nil || time.Since(*boundaryFetched) > maxSourceAge {
			add("boundary_source_age_or_unavailable")
		}
		if boundaryFailed != nil {
			add("boundary_source_refresh_failed")
		}
	}
	return row, nil
}

// CurrentZoning reads persisted evidence and checks its parcel and source versions in one snapshot.
func CurrentZoning(ctx context.Context, pool *pgxpool.Pool, parcelID string) (ZoningAssessment, error) {
	result := ZoningAssessment{ParcelID: parcelID, Zones: []ZoneHit{}, Jurisdictions: []JurisdictionHit{}, Freshness: Freshness{Reasons: []string{}}}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return result, err
	}
	defer tx.Rollback(context.Background())
	base, err := current(ctx, tx, parcelID, union.QualificationZoning, ZoningMethod)
	if errors.Is(err, pgx.ErrNoRows) {
		result.Freshness, err = unavailable(ctx, tx, union.QualificationZoning)
		return result, err
	}
	if err != nil {
		return result, err
	}
	result.Available = true
	result.GenerationID = base.id
	result.GeometryFingerprint = base.geometry
	result.MethodVersion = base.version
	result.CalculatedAt = &base.calculated
	result.TotalAcres = base.total
	result.Freshness = base.fresh
	if err = tx.QueryRow(ctx, `SELECT unmapped_acres,ambiguous_acres,jurisdiction_unmapped_acres,jurisdiction_ambiguous_acres FROM qualification_assessments WHERE parcel_id=$1 AND source_group='zoning'`, parcelID).Scan(&result.UnmappedAcres, &result.AmbiguousAcres, &result.JurisdictionUnmappedAcres, &result.JurisdictionAmbiguousAcres); err != nil {
		return result, err
	}
	rows, err := tx.Query(ctx, `SELECT h.layer_id,h.source_id,h.jurisdiction,h.district_code,h.acres,h.share,z.district_description,z.case_reference,z.official_url,z.source_date FROM qualification_zone_hits h JOIN qualification_assessments a ON a.parcel_id=h.parcel_id AND a.source_group='zoning' JOIN qualification_zones z ON z.generation_id=a.generation_id AND z.layer_id=h.layer_id AND z.source_id=h.source_id WHERE h.parcel_id=$1 ORDER BY h.layer_id,h.source_id`, parcelID)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var h ZoneHit
		if err = rows.Scan(&h.LayerID, &h.SourceID, &h.Jurisdiction, &h.DistrictCode, &h.Area.Acres, &h.Area.Share, &h.Description, &h.CaseReference, &h.OfficialURL, &h.SourceDate); err != nil {
			rows.Close()
			return result, err
		}
		result.Zones = append(result.Zones, h)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	rows, err = tx.Query(ctx, `SELECT h.layer_id,h.source_id,h.name,h.acres,h.share,j.source_date FROM qualification_jurisdiction_hits h JOIN qualification_assessments a ON a.parcel_id=h.parcel_id AND a.source_group='zoning' JOIN qualification_jurisdictions j ON j.generation_id=a.generation_id AND j.layer_id=h.layer_id AND j.source_id=h.source_id WHERE h.parcel_id=$1 ORDER BY h.layer_id,h.source_id`, parcelID)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var h JurisdictionHit
		if err = rows.Scan(&h.LayerID, &h.SourceID, &h.Name, &h.Area.Acres, &h.Area.Share, &h.SourceDate); err != nil {
			rows.Close()
			return result, err
		}
		result.Jurisdictions = append(result.Jurisdictions, h)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	districts := map[string]bool{}
	for _, hit := range result.Zones {
		if hit.DistrictCode != nil {
			districts[hit.Jurisdiction+"\x00"+*hit.DistrictCode] = true
		}
	}
	result.Split = len(districts) > 1 && result.AmbiguousAcres < 1e-7
	return result, tx.Commit(ctx)
}

// CurrentRoads returns a bounded distance, ties, and coverage warning, not legal access.
func CurrentRoads(ctx context.Context, pool *pgxpool.Pool, parcelID string) (RoadAssessment, error) {
	result := RoadAssessment{ParcelID: parcelID, State: RoadSourceUnavailable, SearchRadiusFeet: 5280, Ties: []RoadTie{}, Freshness: Freshness{Reasons: []string{}}}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return result, err
	}
	defer tx.Rollback(context.Background())
	base, err := current(ctx, tx, parcelID, union.QualificationRoads, RoadsMethod)
	if errors.Is(err, pgx.ErrNoRows) {
		result.Freshness, err = unavailable(ctx, tx, union.QualificationRoads)
		if result.Freshness.SourceFetchedAt != nil {
			result.State = RoadPendingAssessment
		}
		return result, err
	}
	if err != nil {
		return result, err
	}
	result.Available = true
	result.GenerationID = base.id
	result.BoundaryGenerationID = base.boundary
	result.GeometryFingerprint = base.geometry
	result.MethodVersion = base.version
	result.CalculatedAt = &base.calculated
	result.Freshness = base.fresh
	var roadState string
	err = tx.QueryRow(ctx, `SELECT road_state,road_distance_feet,road_search_radius_feet,county_edge_warning FROM qualification_assessments WHERE parcel_id=$1 AND source_group='roads'`, parcelID).Scan(&roadState, &result.DistanceFeet, &result.SearchRadiusFeet, &result.CountyEdgeWarning)
	if err != nil {
		return result, err
	}
	result.State = RoadState(roadState)
	for _, reason := range result.Freshness.Reasons {
		if reason == "boundary_source_age_or_unavailable" || reason == "boundary_source_refresh_failed" || reason == "boundary_generation_changed" {
			result.CountyEdgeWarning = true
		}
	}
	rows, err := tx.Query(ctx, `SELECT t.source_id,t.name,t.road_class,t.status,t.distance_feet,r.source_date FROM qualification_road_ties t JOIN qualification_assessments a ON a.parcel_id=t.parcel_id AND a.source_group='roads' JOIN qualification_roads r ON r.generation_id=a.generation_id AND r.source_id=t.source_id WHERE t.parcel_id=$1 ORDER BY t.source_id`, parcelID)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var tie RoadTie
		if err = rows.Scan(&tie.SourceID, &tie.Name, &tie.Class, &tie.Status, &tie.DistanceFeet, &tie.SourceDate); err != nil {
			rows.Close()
			return result, err
		}
		result.Ties = append(result.Ties, tie)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	return result, tx.Commit(ctx)
}

func unavailable(ctx context.Context, tx pgx.Tx, group string) (Freshness, error) {
	result := Freshness{Stale: true, Reasons: []string{"not_assessed"}}
	var head *string
	err := tx.QueryRow(ctx, `SELECT h.generation_id::text,g.fetched_at,f.failed_at,f.error FROM (SELECT $1::text source_group) input LEFT JOIN qualification_heads h ON h.source_group=input.source_group LEFT JOIN qualification_generations g ON g.id=h.generation_id LEFT JOIN qualification_refresh_failures f ON f.source_group=input.source_group`, group).Scan(&head, &result.SourceFetchedAt, &result.RefreshFailedAt, &result.RefreshError)
	if err != nil {
		return result, err
	}
	if head == nil {
		result.Reasons = []string{"source_unavailable"}
	}
	if result.RefreshFailedAt != nil {
		result.Reasons = append(result.Reasons, "source_refresh_failed")
	}
	return result, nil
}
