package vacancy

import (
	"strings"
	"testing"
)

func ptr(v bool) *bool { return &v }
func TestDerive(t *testing.T) {
	base := Facts{ParcelGeneration: "p", StructureGeneration: "s", AssessmentGeneration: "a", ParcelCoverage: true, StructureCoverage: true, AssessmentCoverage: true, IdentitySafe: true, ParcelReportsStructure: ptr(false), OverlappingFootprint: ptr(false), PositiveBuildingValue: ptr(false)}
	for _, tt := range []struct {
		name   string
		change func(*Facts)
		want   Tier
	}{
		{"no footprint", func(*Facts) {}, StrongCandidate},
		{"coverage missing", func(f *Facts) { f.StructureCoverage = false }, Unknown},
		{"stale parcel generation", func(f *Facts) { f.ParcelGeneration = "" }, Unknown},
		{"split identity", func(f *Facts) { f.IdentitySafe = false }, Unknown},
		{"cross boundary", func(f *Facts) { f.FootprintConflict = true }, Unknown},
		{"boundary touch only", func(*Facts) {}, StrongCandidate},
		{"accessory footprint", func(f *Facts) { f.OverlappingFootprint = ptr(true) }, Conflict},
		{"positive assessment", func(f *Facts) { f.PositiveBuildingValue = ptr(true) }, Conflict},
		{"competing rows", func(f *Facts) { f.AssessmentAmbiguous = true }, Unknown},
		{"future year", func(f *Facts) { f.AssessmentAmbiguous = true }, Unknown},
		{"zero value improved", func(f *Facts) { f.ParcelReportsStructure = ptr(true) }, Conflict},
		{"building added", func(f *Facts) {
			f.OverlappingFootprint = ptr(true)
			f.ParcelReportsStructure = ptr(true)
			f.PositiveBuildingValue = ptr(true)
		}, Review},
		{"value changed", func(f *Facts) { f.PositiveBuildingValue = ptr(true) }, Conflict},
		{"missing parcel fact", func(f *Facts) { f.ParcelReportsStructure = nil }, Unknown},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := base
			tt.change(&f)
			got := Derive(f)
			if got.Tier != tt.want || got.RuleVersion != RuleVersion {
				t.Fatalf("got %+v want %s", got, tt.want)
			}
		})
	}
}
func TestSnapshotRejectsPartialAndDuplicates(t *testing.T) {
	fixture := `{"manifest":{"structure_url":"https://example.test/171","assessment_url":"https://example.test/0","retrieved_at":"2026-11-15T00:00:00Z","geometry_srid":4326,"expected_count":1,"validation_note":"unverified fixture"},"structures":[{"id":"1","pin":"001","status":"existing","geometry":{"type":"Polygon","coordinates":[[[0,0],[1,0],[1,1],[0,0]]]}}],"assessments":[]}`
	if _, hash, err := ReadSnapshot(strings.NewReader(fixture)); err != nil || len(hash) != 64 {
		t.Fatalf("valid snapshot: %v %q", err, hash)
	}
	for _, bad := range []string{strings.Replace(fixture, `"expected_count":1`, `"expected_count":2`, 1), fixture + `{}`, strings.Replace(fixture, `"structures":[`, `"structures":[{"id":"1","geometry":{"type":"Polygon"}},`, 1)} {
		if _, _, err := ReadSnapshot(strings.NewReader(bad)); err == nil {
			t.Fatalf("accepted invalid snapshot %s", bad)
		}
	}
}
