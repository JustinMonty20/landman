// Package vacancy derives parcel-keyed building evidence assessments from
// separately admitted live source generations.
package vacancy

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

const RuleVersion = 1

type Tier string

const (
	StrongCandidate Tier = "strong_candidate"
	Review          Tier = "review"
	Conflict        Tier = "conflict"
	Unknown         Tier = "unknown"
)

// Manifest describes a bounded operator-supplied snapshot. Coverage is never
// inferred from a file's row count or a missing feature.
type Manifest struct {
	StructureURL   string     `json:"structure_url"`
	AssessmentURL  string     `json:"assessment_url"`
	RetrievedAt    time.Time  `json:"retrieved_at"`
	EffectiveAt    *time.Time `json:"effective_at"`
	ExpectedCount  int        `json:"expected_count"`
	GeometrySRID   int        `json:"geometry_srid"`
	ValidationNote string     `json:"validation_note"`
	Provenance     string     `json:"provenance"`
}
type Structure struct {
	ID       string          `json:"id"`
	PIN      string          `json:"pin"`
	Status   string          `json:"status"`
	Geometry json.RawMessage `json:"geometry"`
}
type Assessment struct {
	ID            string   `json:"id"`
	ParcelNumber  string   `json:"parcel_number"`
	Year          int      `json:"year"`
	Category      string   `json:"category"`
	BuildingValue *float64 `json:"building_value"`
}
type Snapshot struct {
	Manifest    Manifest     `json:"manifest"`
	Structures  []Structure  `json:"structures"`
	Assessments []Assessment `json:"assessments"`
}

// ReadSnapshot limits input bytes and rejects trailing data. The snapshot is
// evidence, not an assertion of countywide completeness or assessment currency.
func ReadSnapshot(r io.Reader) (Snapshot, string, error) {
	var s Snapshot
	b, err := io.ReadAll(io.LimitReader(r, 32<<20+1))
	if err != nil {
		return s, "", err
	}
	if len(b) > 32<<20 {
		return s, "", errors.New("snapshot exceeds 32 MiB")
	}
	d := json.NewDecoder(strings.NewReader(string(b)))
	d.DisallowUnknownFields()
	if err = d.Decode(&s); err != nil {
		return s, "", err
	}
	var extra any
	if err = d.Decode(&extra); err != io.EOF {
		return s, "", errors.New("snapshot has trailing data")
	}
	if s.Manifest.StructureURL == "" || s.Manifest.AssessmentURL == "" || s.Manifest.RetrievedAt.IsZero() || s.Manifest.ValidationNote == "" || s.Manifest.Provenance != "unverified_fixture" || s.Manifest.ExpectedCount < 0 || s.Manifest.GeometrySRID != 4326 {
		return s, "", errors.New("snapshot requires provenance, declared WGS84 geometry and expected count")
	}
	if s.Manifest.ExpectedCount != len(s.Structures)+len(s.Assessments) {
		return s, "", fmt.Errorf("snapshot count mismatch: expected %d got %d", s.Manifest.ExpectedCount, len(s.Structures)+len(s.Assessments))
	}
	seen := map[string]bool{}
	for _, v := range s.Structures {
		var geom struct {
			Type string `json:"type"`
		}
		if v.ID == "" || seen["s:"+v.ID] || json.Unmarshal(v.Geometry, &geom) != nil || geom.Type != "Polygon" && geom.Type != "MultiPolygon" {
			return s, "", errors.New("invalid or duplicate structure identity/geometry")
		}
		seen["s:"+v.ID] = true
	}
	for _, v := range s.Assessments {
		if v.ID == "" || seen["a:"+v.ID] || v.ParcelNumber == "" || v.BuildingValue != nil && (*v.BuildingValue < 0 || *v.BuildingValue > 1e15) {
			return s, "", errors.New("invalid or duplicate assessment")
		}
		seen["a:"+v.ID] = true
	}
	hash := sha256.Sum256(b)
	return s, hex.EncodeToString(hash[:]), nil
}

// Facts contains independently determined identity, topology and source state.
// Nil means missing evidence, never a negative observation.
type Facts struct {
	ParcelGeneration, StructureGeneration, AssessmentGeneration string `json:"-"`
	ParcelCoverage                                              bool   `json:"parcel_coverage_verified"`
	StructureCoverage                                           bool   `json:"structure_coverage_verified"`
	AssessmentCoverage                                          bool   `json:"assessment_coverage_verified"`
	IdentitySafe                                                bool   `json:"identity_safe"`
	ParcelReportsStructure                                      *bool  `json:"parcel_reports_structure"`
	OverlappingFootprint                                        *bool  `json:"overlapping_footprint"`
	PositiveBuildingValue                                       *bool  `json:"positive_building_value"`
	AssessmentAmbiguous                                         bool   `json:"assessment_ambiguous"`
	FootprintConflict                                           bool   `json:"footprint_conflict"`
}
type Result struct {
	Tier        Tier   `json:"tier"`
	Reason      string `json:"reason"`
	RuleVersion int    `json:"rule_version"`
}

// Derive does not turn zero value, an absent PIN, or a boundary touch into
// proof that a building is absent.
func Derive(f Facts) Result {
	result := func(t Tier, why string) Result { return Result{t, why, RuleVersion} }
	if f.ParcelGeneration == "" || f.StructureGeneration == "" || f.AssessmentGeneration == "" || !f.ParcelCoverage || !f.StructureCoverage || !f.AssessmentCoverage {
		return result(Unknown, "source_generation_or_coverage_unverified")
	}
	if !f.IdentitySafe || f.AssessmentAmbiguous || f.FootprintConflict {
		return result(Unknown, "identity_or_assessment_ambiguous")
	}
	if f.ParcelReportsStructure == nil || f.OverlappingFootprint == nil || f.PositiveBuildingValue == nil {
		return result(Unknown, "necessary_evidence_missing")
	}
	footprint, parcel, value := *f.OverlappingFootprint, *f.ParcelReportsStructure, *f.PositiveBuildingValue
	if footprint != parcel || footprint != value {
		return result(Conflict, "independent_building_evidence_disagrees")
	}
	if !footprint && !parcel && !value {
		return result(StrongCandidate, "no_reported_structure_or_positive_building_value_with_verified_coverage")
	}
	return result(Review, "building_evidence_requires_review")
}
