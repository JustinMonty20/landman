//go:build postgres

package soil

import (
	"fmt"
	"strings"
	"testing"

	"github.com/JustinMonty20/landman/internal/county"
	"github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/ewkb"
)

func (f *assessmentFixture) storedShape(wkt string) *geom.T {
	f.t.Helper()
	var raw []byte
	if err := f.db.QueryRow(f.ctx, `SELECT ST_AsEWKB(ST_Translate(ST_GeomFromText($1,3857),-9000000,4170000))`, wkt).Scan(&raw); err != nil {
		f.t.Fatal(err)
	}
	g, err := ewkb.Unmarshal(raw)
	if err != nil {
		f.t.Fatal(err)
	}
	return &g
}

func (f *assessmentFixture) storedPolygon(id, key, wkt string) Polygon {
	return Polygon{County: county.Union, SourceName: f.source, ObjectID: id, MapUnitKey: key, Shape: f.storedShape(wkt)}
}

const assessmentSquare = "POLYGON((0 0,1000 0,1000 1000,0 1000,0 0))"

func TestPostgresAssessmentStoredTopology(t *testing.T) {
	for _, tc := range []struct {
		name          string
		unitA, unitB  string
		sameUnit      bool
		gap, conflict bool
		mapUnits      int
	}{
		{"touch", assessmentSquare, "POLYGON((1000 0,1100 0,1100 1000,1000 1000,1000 0))", false, false, false, 1},
		{"exact diagonal cover", "POLYGON((0 0,1000 0,1000 1000,0 0))", "POLYGON((0 0,500 500,1000 1000,0 1000,0 0))", false, false, false, 2},
		{"duplicate pieces", assessmentSquare, assessmentSquare, true, false, false, 1},
		{"different-unit conflict", "POLYGON((0 0,550 0,550 1000,0 1000,0 0))", "POLYGON((450 0,1000 0,1000 1000,450 1000,450 0))", false, false, true, 2},
		{"hole", "POLYGON((0 0,1000 0,1000 1000,0 1000,0 0),(250 250,250 750,750 750,750 250,250 250))", "", false, true, false, 1},
		{"two millimeter gap", "POLYGON((0 0,500 0,500 1000,0 1000,0 0))", "POLYGON((500.002 0,1000 0,1000 1000,500.002 1000,500.002 0))", false, true, false, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newAssessmentFixture(t)
			id := f.parcel("parcel", f.storedShape(assessmentSquare), false)
			polygons := []Polygon{f.storedPolygon("a", "115328", tc.unitA)}
			if tc.unitB != "" {
				key := "115330"
				if tc.sameUnit {
					key = "115328"
				}
				polygons = append(polygons, f.storedPolygon("b", key, tc.unitB))
			}
			f.store(polygons)
			f.screen(polygons)
			result := f.read(id)
			if !result.Available || result.Freshness.Stale {
				t.Fatalf("expected current soil assessment: %+v", result.Freshness)
			}
			for _, use := range result.Uses {
				if (use.Uncovered.Acres > 0) != tc.gap || (use.Ambiguous.Acres > 0) != tc.conflict || len(use.MapUnits) != tc.mapUnits {
					t.Fatalf("%s: uncovered=%g, ambiguous=%g, map units=%d", use.Use, use.Uncovered.Acres, use.Ambiguous.Acres, len(use.MapUnits))
				}
				if tc.sameUnit && len(use.MapUnits[0].Polygons) != 2 {
					t.Fatal("duplicate pieces lost their source provenance")
				}
				for _, unit := range use.MapUnits {
					if tc.conflict && unit.Area.Acres >= unit.MappedArea.Acres {
						t.Fatal("overlap counted in exclusive rating area")
					}
				}
			}
			reconcile(t, result)
		})
	}
}

func TestPostgresAssessmentRepairAndRefusal(t *testing.T) {
	f := newAssessmentFixture(t)
	// One ring traces two squares that touch at a point. Repair must retain both.
	join := f.storedPolygon("point-touch", "115328", "POLYGON((0 0,500 0,500 500,0 500,0 0,-500 0,-500 -500,0 -500,0 0))")
	f.store([]Polygon{join})
	id := f.parcel("parcel", f.storedShape(assessmentSquare), false)
	f.screen([]Polygon{join})
	current := f.read(id)
	use := useResult(t, current, ConventionalSeptic)
	if len(use.MapUnits) != 1 || use.MapUnits[0].Area.Acres <= 0 || use.Uncovered.Acres <= 0 {
		t.Fatalf("repaired soil polygon did not contribute to screening: %+v", use)
	}
	var rawValid bool
	if err := f.db.QueryRow(f.ctx, `SELECT ST_IsValid(shape) FROM soil_polygon_set_members WHERE set_id=$1`, f.setID).Scan(&rawValid); err != nil || rawValid {
		t.Fatalf("raw geometry changed: valid=%t err=%v", rawValid, err)
	}
	bad := f.storedPolygon("exterior-spike", "115330", "POLYGON((3000 0,4000 0,4000 1000,3500 1000,3500 1100,3500 1000,3000 1000,3000 0))")
	f.store([]Polygon{bad})
	err := screen(f.ctx, f.db, f.server.Client(), f.server.URL, county.Union, f.source, []Polygon{bad}, f.units, ScreeningOptions{SoilSetID: f.setID})
	if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("OBJECTID %s", bad.ObjectID)) {
		t.Fatalf("screening did not identify the refused soil geometry: %v", err)
	}
	if after := f.read(id); after.InterpretationSetRevision != current.InterpretationSetRevision || !after.Freshness.Stale {
		t.Fatal("refused geometry changed the active screening head or hid the failure")
	}
}
