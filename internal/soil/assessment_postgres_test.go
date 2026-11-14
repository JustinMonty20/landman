//go:build postgres

package soil

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/JustinMonty20/landman/internal/county"
	"github.com/JustinMonty20/landman/internal/models"
	"github.com/JustinMonty20/landman/internal/parcelstore"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/ewkb"
)

type assessmentFixture struct {
	t      *testing.T
	ctx    context.Context
	db     *pgxpool.Pool
	source string
	setID  string
	server *httptest.Server
	mode   atomic.Int32
	calls  atomic.Int32
	units  []MapUnit
}

func newAssessmentFixture(t *testing.T) *assessmentFixture {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}
	db, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	f := &assessmentFixture{t: t, ctx: context.Background(), db: db, source: fmt.Sprintf("assessment-%d", time.Now().UnixNano()), units: []MapUnit{{MapUnitKey: "115328"}, {MapUnitKey: "115330"}, {MapUnitKey: "115341"}}}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.calls.Add(1)
		q := r.FormValue("query")
		file := ""
		switch {
		case strings.Contains(q, "FROM sdvattribute"):
			file = "union-rules-raw.json"
		case strings.Contains(q, "JOIN muaggatt"):
			file = "union-muaggatt-raw.json"
		case strings.Contains(q, "JOIN sacatalog"):
			file = "union-catalog-raw.json"
		case strings.Contains(q, "FROM component c WHERE"):
			file = "union-ancillary-components-raw.json"
		case strings.Contains(q, "JOIN cointerp"):
			file = "union-components-raw.json"
		case strings.Contains(q, "JOIN cosoilmoist"):
			file = "union-moisture-raw.json"
		case strings.Contains(q, "JOIN comonth"):
			file = "union-monthly-raw.json"
		case strings.Contains(q, "JOIN chorizon"):
			file = "union-horizons-raw.json"
		case strings.Contains(q, "JOIN corestrictions"):
			file = "union-restrictions-raw.json"
		default:
			fmt.Fprint(w, `{"Table":[]}`)
			return
		}
		if f.mode.Load() == 2 && file == "union-components-raw.json" {
			http.Error(w, "fixture failure", 503)
			return
		}
		raw, err := os.ReadFile("testdata/" + file)
		if err != nil {
			t.Error(err)
			http.Error(w, "fixture missing", 500)
			return
		}
		var body struct {
			Table [][]any `json:"Table"`
		}
		if err = json.Unmarshal(raw, &body); err != nil {
			t.Error(err)
			return
		}
		if file != "union-rules-raw.json" {
			filtered := [][]any{}
			for _, row := range body.Table {
				if !strings.Contains(q, "'"+row[0].(string)+"'") {
					continue
				}
				if file == "union-catalog-raw.json" {
					if f.mode.Load() == 3 {
						row[2] = "29"
					}
					if f.mode.Load() == 4 {
						row[2] = nil
					}
				}
				if f.mode.Load() == 1 && file == "union-muaggatt-raw.json" && row[0] == "115330" {
					row[1] = nil
					row[2] = "Not rated"
				}
				filtered = append(filtered, row)
			}
			body.Table = filtered
		}
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(func() {
		f.server.Close()
		for _, sql := range []string{
			`DELETE FROM soil_assessment_map_units WHERE run_id IN (SELECT id FROM soil_screening_runs WHERE source_name=$1)`,
			`DELETE FROM soil_parcel_assessments WHERE run_id IN (SELECT id FROM soil_screening_runs WHERE source_name=$1)`,
			`DELETE FROM soil_screening_members WHERE run_id IN (SELECT id FROM soil_screening_runs WHERE source_name=$1)`,
			`DELETE FROM soil_measurements WHERE run_id IN (SELECT id FROM soil_screening_runs WHERE source_name=$1)`,
			`DELETE FROM soil_component_facts WHERE run_id IN (SELECT id FROM soil_screening_runs WHERE source_name=$1)`,
			`DELETE FROM soil_screening_ratings WHERE run_id IN (SELECT id FROM soil_screening_runs WHERE source_name=$1)`,
			`DELETE FROM soil_screening_heads WHERE source_name=$1`,
			`DELETE FROM soil_screening_refresh_status WHERE source_name=$1`,
			`DELETE FROM soil_screening_runs WHERE source_name=$1`,
			`DELETE FROM soil_assessment_keys WHERE parcel_id IN (SELECT id FROM parcels WHERE source_name=$1)`,
			`DELETE FROM soil_polygon_set_heads WHERE source_name=$1`,
			`DELETE FROM soil_polygon_set_members WHERE set_id IN (SELECT id FROM soil_polygon_sets WHERE source_name=$1)`,
			`DELETE FROM soil_polygon_sets WHERE source_name=$1`,
			`DELETE FROM soil_polygon_parcels WHERE parcel_id IN (SELECT id FROM parcels WHERE source_name=$1) OR soil_polygon_id IN (SELECT id FROM soil_polygons WHERE source_name=$1)`,
			`DELETE FROM soil_polygons WHERE source_name=$1`,
			`DELETE FROM parcels WHERE source_name=$1`,
		} {
			if _, err := db.Exec(context.Background(), sql, f.source); err != nil {
				t.Errorf("cleanup: %v", err)
			}
		}
		db.Close()
	})
	return f
}

func (f *assessmentFixture) shape(wkt string) *geom.T {
	f.t.Helper()
	var raw []byte
	if err := f.db.QueryRow(f.ctx, `SELECT ST_AsEWKB(ST_Transform(ST_GeomFromText($1,2264),3857))`, wkt).Scan(&raw); err != nil {
		f.t.Fatal(err)
	}
	g, err := ewkb.Unmarshal(raw)
	if err != nil {
		f.t.Fatal(err)
	}
	return &g
}

func (f *assessmentFixture) box(x1, y1, x2, y2 float64) *geom.T {
	return f.shape(fmt.Sprintf("POLYGON((%f %f,%f %f,%f %f,%f %f,%f %f))", 1533265+x1, 462768+y1, 1533265+x2, 462768+y1, 1533265+x2, 462768+y2, 1533265+x1, 462768+y2, 1533265+x1, 462768+y1))
}

func (f *assessmentFixture) parcel(key string, shape *geom.T, vacant bool) string {
	f.t.Helper()
	tx, err := f.db.Begin(f.ctx)
	if err != nil {
		f.t.Fatal(err)
	}
	defer tx.Rollback(f.ctx)
	record := models.GISParcel{Identity: models.ParcelIdentity{CountyID: "union", CountyCode: "179", SourceName: f.source, SourceParcelID: key}, Geometry: models.GISGeometry{Shape: shape, SRID: 3857}}
	if !vacant {
		year := int64(1998)
		record.Building.YearBuilt = &year
	}
	if _, err = parcelstore.SaveParcel(f.ctx, tx, record, vacant); err != nil {
		f.t.Fatal(err)
	}
	if err = tx.Commit(f.ctx); err != nil {
		f.t.Fatal(err)
	}
	var id string
	if err = f.db.QueryRow(f.ctx, `SELECT id::text FROM parcels WHERE source_name=$1 AND source_parcel_id=$2`, f.source, key).Scan(&id); err != nil {
		f.t.Fatal(err)
	}
	return id
}

func (f *assessmentFixture) polygon(id, key string, x1, x2 float64) Polygon {
	version := 5.0
	return Polygon{County: county.Union, SourceName: f.source, ObjectID: id, MapUnitKey: key, SpatialVersion: &version, Shape: f.box(x1, 0, x2, 100)}
}

func (f *assessmentFixture) store(polygons []Polygon) {
	f.t.Helper()
	setID, err := storeWithSet(f.ctx, f.db, county.Union, f.source, polygons, f.units, f.server.Client(), f.server.URL)
	if err != nil {
		f.t.Fatal(err)
	}
	f.setID = setID
}
func (f *assessmentFixture) screen(polygons []Polygon) {
	f.t.Helper()
	if err := screen(f.ctx, f.db, f.server.Client(), f.server.URL, county.Union, f.source, polygons, f.units, ScreeningOptions{SoilSetID: f.setID}); err != nil {
		f.t.Fatal(err)
	}
}
func (f *assessmentFixture) read(id string) *ParcelAssessment {
	f.t.Helper()
	result, err := CurrentAssessment(f.ctx, f.db, county.Union, f.source, id)
	if err != nil {
		f.t.Fatal(err)
	}
	return result
}
func near(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-7 {
		t.Fatalf("got %.12f, want %.12f", got, want)
	}
}
func useResult(t *testing.T, a *ParcelAssessment, use Use) UseAssessment {
	t.Helper()
	for _, v := range a.Uses {
		if v.Use == use {
			return v
		}
	}
	t.Fatalf("missing use %s", use)
	return UseAssessment{}
}
func classArea(v UseAssessment, class string) float64 {
	for _, r := range v.Rated {
		if r.Rating == class {
			return r.Area.Acres
		}
	}
	return 0
}
func reconcile(t *testing.T, a *ParcelAssessment) {
	t.Helper()
	for _, u := range a.Uses {
		sum := u.Unrated.Acres + u.Uncovered.Acres + u.Ambiguous.Acres
		for _, r := range u.Rated {
			sum += r.Area.Acres
		}
		if a.TotalAcres != nil {
			near(t, sum, *a.TotalAcres)
		}
	}
}

func TestPostgresAssessmentSeventyThirtyAndImprovedParcels(t *testing.T) {
	f := newAssessmentFixture(t)
	vacant := f.parcel("vacant", f.box(0, 0, 100, 100), true)
	improved := f.parcel("house", f.box(0, 0, 100, 100), false)
	missing := f.parcel("missing", nil, false)
	zero := f.parcel("zero", f.shape("LINESTRING(1533265 462768,1533365 462768)"), true)
	uncovered := f.parcel("outside", f.box(-300, 0, -200, 100), true)
	polygons := []Polygon{f.polygon("a", "115341", 0, 70), f.polygon("a-duplicate", "115341", 0, 70), f.polygon("a-part", "115341", 10, 30), f.polygon("b", "115330", 70, 100), f.polygon("edge", "115328", 100, 150)}
	f.store(polygons)
	f.screen(polygons)
	total := 10000.0 / 43560
	for _, id := range []string{vacant, improved} {
		a := f.read(id)
		if !a.Available || a.Freshness.Stale || a.EvidenceLevel != "mapped-survey" {
			t.Fatalf("bad availability/freshness: %+v", a)
		}
		near(t, *a.TotalAcres, total)
		septic := useResult(t, a, ConventionalSeptic)
		near(t, classArea(septic, "Very limited"), total*.7)
		near(t, classArea(septic, "Somewhat limited"), total*.3)
		near(t, septic.Uncovered.Acres, 0)
		near(t, septic.Ambiguous.Acres, 0)
		if len(septic.MapUnits) != 2 {
			t.Fatalf("edge-only contact entered assessment: %+v", septic.MapUnits)
		}
		for _, m := range septic.MapUnits {
			if m.MapUnitKey == "115341" && len(m.Polygons) != 3 {
				t.Fatalf("lost contributing UUIDs: %+v", m)
			}
			if len(m.Interpretation.Measurements) == 0 {
				t.Fatal("normalized measurements absent from assessment")
			}
			if m.Interpretation.Revision.SurveyVersion == nil || *m.Interpretation.Revision.SurveyVersion != "28" || m.Interpretation.ID == "" {
				t.Fatalf("lost source evidence: %+v", m)
			}
		}
		dwelling := useResult(t, a, DwellingWithoutBasement)
		near(t, classArea(dwelling, "Not limited"), total*.3)
		reconcile(t, a)
	}
	for _, id := range []string{missing, zero} {
		a := f.read(id)
		if a.Available || a.TotalAcres != nil {
			t.Fatalf("missing geometry became evidence: %+v", a)
		}
		for _, u := range a.Uses {
			if u.Uncovered.Share != nil || len(u.Rated) > 0 {
				t.Fatalf("unavailable became favorable: %+v", u)
			}
		}
	}
	a := f.read(uncovered)
	near(t, useResult(t, a, ConventionalSeptic).Uncovered.Acres, total)
	var count int
	if err := f.db.QueryRow(f.ctx, `SELECT count(*) FROM soil_parcel_assessments a JOIN soil_screening_heads h ON h.run_id=a.run_id WHERE h.source_name=$1`, f.source).Scan(&count); err != nil || count != 5 {
		t.Fatalf("assessed %d parcels: %v", count, err)
	}
	old := f.read(improved)
	if _, err := f.db.Exec(f.ctx, `UPDATE parcels SET current_name_1='changed owner',situs_line_1='changed address' WHERE id=$1`, improved); err != nil {
		t.Fatal(err)
	}
	after := f.read(improved)
	if after.Freshness.Stale || after.GeometryFingerprint != old.GeometryFingerprint {
		t.Fatal("owner changed soil evidence")
	}
	f.screen(polygons)
	var componentCount int
	if err := f.db.QueryRow(f.ctx, `SELECT count(*) FROM soil_component_facts c JOIN soil_screening_heads h ON h.run_id=c.run_id WHERE h.source_name=$1`, f.source).Scan(&componentCount); err != nil || componentCount == 0 {
		t.Fatalf("component facts not persisted separately: %d %v", componentCount, err)
	}
	var measurementCount int
	if err := f.db.QueryRow(f.ctx, `SELECT count(*) FROM soil_measurements m JOIN soil_screening_heads h ON h.run_id=m.run_id WHERE h.source_name=$1`, f.source).Scan(&measurementCount); err != nil || measurementCount == 0 {
		t.Fatalf("normalized measurements absent: %d %v", measurementCount, err)
	}
	current, _, err := CurrentInterpretations(f.ctx, f.db, county.Union, f.source, "115341")
	if err != nil || len(current) != 3 || len(current[0].Measurements) == 0 {
		t.Fatalf("normalized interpretation read: %d %v", len(current), err)
	}
	refreshed := f.read(improved)
	if refreshed.ID != old.ID || refreshed.Uses[0].MapUnits[0].Interpretation.ID != old.Uses[0].MapUnits[0].Interpretation.ID {
		t.Fatal("refresh changed stable application IDs")
	}
	f.mode.Store(3)
	f.screen(polygons)
	changed := f.read(improved)
	if changed.Uses[0].MapUnits[0].Interpretation.ID == refreshed.Uses[0].MapUnits[0].Interpretation.ID {
		t.Fatal("survey revision reused the prior interpretation UUID")
	}
	if changed.Uses[0].MapUnits[0].Interpretation.Revision.SurveyVersion == nil || *changed.Uses[0].MapUnits[0].Interpretation.Revision.SurveyVersion != "29" {
		t.Fatal("survey revision missing from read surface")
	}
	f.screen(polygons)
	stable := f.read(improved)
	if stable.Uses[0].MapUnits[0].Interpretation.ID != changed.Uses[0].MapUnits[0].Interpretation.ID {
		t.Fatal("same survey revision changed interpretation UUID")
	}
	f.mode.Store(4)
	f.screen(polygons)
	unknown := f.read(improved)
	if unknown.Uses[0].MapUnits[0].Interpretation.Revision.SurveyVersion != nil || unknown.Uses[0].MapUnits[0].Interpretation.ID == stable.Uses[0].MapUnits[0].Interpretation.ID {
		t.Fatal("missing revision was fabricated or reused a known revision UUID")
	}
	f.screen(polygons)
	if f.read(improved).Uses[0].MapUnits[0].Interpretation.ID != unknown.Uses[0].MapUnits[0].Interpretation.ID {
		t.Fatal("unknown revision did not retain its stable UUID")
	}
}

func TestPostgresAssessmentCopyAndChangedInput(t *testing.T) {
	f := newAssessmentFixture(t)
	id := f.parcel("original", f.box(0, 0, 100, 100), false)
	polygons := []Polygon{f.polygon("left", "115341", 0, 50)}
	f.store(polygons)
	f.screen(polygons)
	original := f.read(id)
	total := 10000.0 / 43560
	near(t, classArea(useResult(t, original, ConventionalSeptic), "Very limited"), total*.5)

	f.screen(polygons)
	copied := f.read(id)
	if copied.ID != original.ID || copied.InterpretationSetRevision == original.InterpretationSetRevision {
		t.Fatal("unchanged assessment did not publish a new run with a stable ID")
	}
	near(t, classArea(useResult(t, copied, ConventionalSeptic), "Very limited"), total*.5)

	added := f.parcel("new", f.box(0, 0, 100, 100), true)
	f.screen(polygons)
	near(t, classArea(useResult(t, f.read(added), ConventionalSeptic), "Very limited"), total*.5)
	near(t, classArea(useResult(t, f.read(id), ConventionalSeptic), "Very limited"), total*.5)

	polygons = append(polygons, f.polygon("right", "115330", 50, 100))
	f.store(polygons)
	f.screen(polygons)
	for _, parcelID := range []string{id, added} {
		a := f.read(parcelID)
		septic := useResult(t, a, ConventionalSeptic)
		near(t, classArea(septic, "Somewhat limited"), total*.5)
		near(t, septic.Uncovered.Acres, 0)
		reconcile(t, a)
	}
}

func TestPostgresAssessmentAmbiguityUnratedRetirementAndStaleness(t *testing.T) {
	f := newAssessmentFixture(t)
	id := f.parcel("parcel", f.box(0, 0, 100, 100), false)
	polygons := []Polygon{f.polygon("a", "115341", 0, 70), f.polygon("duplicate", "115341", 0, 70), f.polygon("b", "115330", 50, 90), f.polygon("edge", "115328", 100, 150)}
	oldDate := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	polygons[0].SurveySavedOn = &oldDate
	f.mode.Store(1)
	f.store(polygons)
	f.screen(polygons)
	a := f.read(id)
	total := 10000.0 / 43560
	septic := useResult(t, a, ConventionalSeptic)
	near(t, classArea(septic, "Very limited"), total*.5)
	near(t, septic.Unrated.Acres, total*.2)
	near(t, septic.Ambiguous.Acres, total*.2)
	near(t, septic.Uncovered.Acres, total*.1)
	for _, m := range septic.MapUnits {
		if m.MapUnitKey == "115330" && (m.Interpretation.Availability != Unavailable || m.Interpretation.OriginalRating != "") {
			t.Fatal("missing rating was not unavailable")
		}
	}
	dwelling := useResult(t, a, DwellingWithoutBasement)
	for _, m := range dwelling.MapUnits {
		if m.MapUnitKey == "115330" && (m.Interpretation.Availability != NotRated || m.Interpretation.OriginalRating != "Not rated") {
			t.Fatal("not-rated phrase lost")
		}
	}
	if !strings.Contains(strings.Join(a.Warnings, ","), "dates differ") {
		t.Fatal("mismatched survey dates hidden")
	}
	reconcile(t, a)
	polygons[0].Shape = f.box(0, 0, 50, 100)
	current := polygons[:1]
	f.store(current)
	stale := f.read(id)
	if !stale.Freshness.Stale || stale.InterpretationSetRevision != a.InterpretationSetRevision {
		t.Fatal("new geometry erased prior evidence")
	}
	near(t, useResult(t, stale, ConventionalSeptic).Ambiguous.Acres, total*.2)
	f.screen(current)
	fresh := f.read(id)
	septic = useResult(t, fresh, ConventionalSeptic)
	near(t, classArea(septic, "Very limited"), total*.5)
	near(t, septic.Uncovered.Acres, total*.5)
	near(t, septic.Ambiguous.Acres, 0)
	if len(septic.MapUnits) != 1 || fresh.Freshness.Stale {
		t.Fatal("retired polygons or stale joins entered fresh assessment")
	}
	var legacy int
	if err := f.db.QueryRow(f.ctx, `SELECT count(*) FROM soil_polygon_parcels j JOIN soil_polygons p ON p.id=j.soil_polygon_id WHERE p.source_name=$1 AND p.object_id='b'`, f.source).Scan(&legacy); err != nil || legacy == 0 {
		t.Fatalf("fixture did not retain legacy joins: %d %v", legacy, err)
	}
	f.store(current)
	if f.read(id).Freshness.Stale {
		t.Fatal("unchanged geometry content marked stale")
	}
	id = f.parcel("parcel", f.box(0, 0, 100, 200), false)
	if !f.read(id).Freshness.Stale {
		t.Fatal("parcel geometry change did not stale evidence")
	}
	f.screen(current)
	fresh = f.read(id)
	near(t, *fresh.TotalAcres, total*2)
	near(t, useResult(t, fresh, ConventionalSeptic).Uncovered.Acres, total*1.5)
	reconcile(t, fresh)
	if _, err := f.db.Exec(f.ctx, `UPDATE soil_screening_runs SET fresh_until=now()-interval '1 second' WHERE id=$1`, fresh.InterpretationSetRevision); err != nil {
		t.Fatal(err)
	}
	if !f.read(id).Freshness.Stale {
		t.Fatal("expired evidence did not stale")
	}
}

func TestPostgresAssessmentChangedInputCancellationIsAtomic(t *testing.T) {
	f := newAssessmentFixture(t)
	id := f.parcel("old", f.box(0, 0, 100, 100), true)
	polygons := []Polygon{f.polygon("a", "115341", 0, 100)}
	f.store(polygons)
	f.screen(polygons)
	previous := f.read(id)
	added := f.parcel("new", f.box(0, 0, 100, 100), false)

	blocker, err := f.db.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback(f.ctx)
	if _, err = blocker.Exec(f.ctx, `LOCK TABLE soil_parcel_assessments IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(f.ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- screen(ctx, f.db, f.server.Client(), f.server.URL, county.Union, f.source, polygons, f.units, ScreeningOptions{SoilSetID: f.setID})
	}()
	deadline := time.Now().Add(15 * time.Second)
	blocked := false
	for time.Now().Before(deadline) {
		if err = f.db.QueryRow(f.ctx, `SELECT EXISTS(SELECT 1 FROM pg_locks WHERE relation='soil_parcel_assessments'::regclass AND NOT granted)`).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	screenErr := <-done
	if !blocked || screenErr == nil {
		t.Fatal("canceled changed-input assessment did not stop")
	}
	if err = blocker.Rollback(f.ctx); err != nil {
		t.Fatal(err)
	}
	if f.read(id).InterpretationSetRevision != previous.InterpretationSetRevision {
		t.Fatal("canceled changed-input assessment moved the head")
	}
	var partial int
	if err = f.db.QueryRow(f.ctx, `SELECT count(*) FROM soil_parcel_assessments WHERE parcel_id=$1`, added).Scan(&partial); err != nil || partial != 0 {
		t.Fatalf("partial changed-input assessment persisted: %d %v", partial, err)
	}
	f.screen(polygons)
	if !f.read(added).Available {
		t.Fatal("successful changed-input refresh missed the new parcel")
	}
}

func TestPostgresScreeningRejectsReplacedSoilSet(t *testing.T) {
	f := newAssessmentFixture(t)
	id := f.parcel("parcel", f.box(0, 0, 100, 100), true)
	polygons := []Polygon{f.polygon("a", "115341", 0, 100)}
	oldDate := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	polygons[0].SurveySavedOn = &oldDate
	f.store(polygons)
	f.screen(polygons)
	oldSetID := f.setID
	previous := f.read(id).InterpretationSetRevision

	current := append([]Polygon(nil), polygons...)
	newDate := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	current[0].SurveySavedOn = &newDate
	f.store(current)
	if oldSetID == f.setID {
		t.Fatal("source import reused the old soil set")
	}
	var datesDiffer bool
	if err := f.db.QueryRow(f.ctx, `SELECT old.survey_saved_on IS DISTINCT FROM current.survey_saved_on FROM soil_polygon_set_members old JOIN soil_polygon_set_members current ON old.polygon_id=current.polygon_id WHERE old.set_id=$1 AND current.set_id=$2`, oldSetID, f.setID).Scan(&datesDiffer); err != nil || !datesDiffer {
		t.Fatalf("source survey dates did not change: %v", err)
	}
	if err := screen(f.ctx, f.db, f.server.Client(), f.server.URL, county.Union, f.source, polygons, f.units, ScreeningOptions{SoilSetID: oldSetID}); err == nil || !strings.Contains(err.Error(), "newer polygon set") {
		t.Fatalf("screening accepted a replaced source import: %v", err)
	}
	if got := f.read(id).InterpretationSetRevision; got != previous {
		t.Fatal("rejected screening replaced the published run")
	}
	var count int
	if err := f.db.QueryRow(f.ctx, `SELECT count(*) FROM soil_screening_runs WHERE source_name=$1`, f.source).Scan(&count); err != nil || count != 1 {
		t.Fatalf("rejected screening persisted a run: %d %v", count, err)
	}
	f.screen(current)
	if got := f.read(id).SoilSetID; got != f.setID {
		t.Fatal("current soil set was not published")
	}
}

func TestPostgresAssessmentLocksParcelWritesUntilPublication(t *testing.T) {
	f := newAssessmentFixture(t)
	id := f.parcel("original", f.box(0, 0, 100, 100), false)
	polygons := []Polygon{f.polygon("a", "115341", 0, 100)}
	f.store(polygons)
	f.screen(polygons)
	previous := f.read(id).InterpretationSetRevision

	blocker, err := f.db.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback(f.ctx)
	if _, err = blocker.Exec(f.ctx, `LOCK TABLE soil_parcel_assessments IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(f.ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- screen(ctx, f.db, f.server.Client(), f.server.URL, county.Union, f.source, polygons, f.units, ScreeningOptions{SoilSetID: f.setID})
	}()
	deadline := time.Now().Add(15 * time.Second)
	locked := false
	for time.Now().Before(deadline) {
		if err = f.db.QueryRow(f.ctx, `SELECT EXISTS(SELECT 1 FROM pg_locks WHERE relation='parcels'::regclass AND mode='ShareLock' AND granted)`).Scan(&locked); err != nil {
			t.Fatal(err)
		}
		if locked {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !locked {
		cancel()
		<-done
		t.Fatal("screening did not lock parcel writes")
	}
	writeCtx, stopWrite := context.WithTimeout(f.ctx, 200*time.Millisecond)
	_, writeErr := f.db.Exec(writeCtx, `INSERT INTO parcels(county_id,county_code,source_name,source_parcel_id) VALUES ('union','179',$1,'concurrent')`, f.source)
	stopWrite()
	if writeErr == nil {
		t.Fatal("parcel write succeeded before screening completed")
	}
	cancel()
	if err = <-done; err == nil {
		t.Fatal("canceled screening published")
	}
	if err = blocker.Rollback(f.ctx); err != nil {
		t.Fatal(err)
	}
	if f.read(id).InterpretationSetRevision != previous {
		t.Fatal("canceled screening moved the head")
	}
	if _, err = f.db.Exec(f.ctx, `INSERT INTO parcels(county_id,county_code,source_name,source_parcel_id) VALUES ('union','179',$1,'concurrent')`, f.source); err != nil {
		t.Fatalf("parcel write stayed locked after rollback: %v", err)
	}
}

func TestPostgresLateFailureDoesNotStaleNewHead(t *testing.T) {
	f := newAssessmentFixture(t)
	id := f.parcel("parcel", f.box(0, 0, 100, 100), true)
	polygons := []Polygon{f.polygon("a", "115341", 0, 100)}
	f.store(polygons)
	f.screen(polygons)
	oldHead := f.read(id).InterpretationSetRevision
	f.screen(polygons)
	newHead := f.read(id).InterpretationSetRevision
	if oldHead == newHead {
		t.Fatal("fixture did not publish a new head")
	}
	if err := recordScreeningFailure(f.ctx, f.db, county.Union, f.source, oldHead, fmt.Errorf("late failure")); err != nil {
		t.Fatal(err)
	}
	if current := f.read(id); current.Freshness.Stale || current.InterpretationSetRevision != newHead {
		t.Fatalf("late failure marked a newer head stale: %+v", current.Freshness)
	}
	var count int
	if err := f.db.QueryRow(f.ctx, `SELECT count(*) FROM soil_screening_refresh_status WHERE county_id=$1 AND source_name=$2`, county.Union.String(), f.source).Scan(&count); err != nil || count != 0 {
		t.Fatalf("late failure created status: %d %v", count, err)
	}
}

func TestPostgresAssessmentFailedRefreshAndCancellationAreAtomic(t *testing.T) {
	f := newAssessmentFixture(t)
	id := f.parcel("parcel", f.box(0, 0, 100, 100), true)
	polygons := []Polygon{f.polygon("a", "115341", 0, 100)}
	f.store(polygons)
	f.screen(polygons)
	previous := f.read(id)
	var before int
	if err := f.db.QueryRow(f.ctx, `SELECT count(*) FROM soil_screening_runs WHERE source_name=$1`, f.source).Scan(&before); err != nil {
		t.Fatal(err)
	}
	f.mode.Store(2)
	if err := screen(f.ctx, f.db, f.server.Client(), f.server.URL, county.Union, f.source, polygons, f.units, ScreeningOptions{SoilSetID: f.setID}); err == nil {
		t.Fatal("required HTTP failure succeeded")
	}
	failed := f.read(id)
	if !failed.Freshness.Stale || failed.Freshness.RefreshFailedAt == nil || failed.InterpretationSetRevision != previous.InterpretationSetRevision {
		t.Fatal("failed refresh hid previous evidence or failure")
	}
	f.mode.Store(0)
	blocker, err := f.db.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback(f.ctx)
	if _, err = blocker.Exec(f.ctx, `LOCK TABLE soil_parcel_assessments IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(f.ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- screen(ctx, f.db, f.server.Client(), f.server.URL, county.Union, f.source, polygons, f.units, ScreeningOptions{SoilSetID: f.setID})
	}()
	deadline := time.Now().Add(15 * time.Second)
	blocked := false
	for time.Now().Before(deadline) {
		if err = f.db.QueryRow(f.ctx, `SELECT EXISTS(SELECT 1 FROM pg_locks WHERE relation='soil_parcel_assessments'::regclass AND NOT granted)`).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	if !blocked {
		t.Error("screening did not reach blocked persistence")
	}
	if err = <-done; err == nil {
		t.Fatal("canceled assessment published")
	}
	if err = blocker.Rollback(f.ctx); err != nil {
		t.Fatal(err)
	}
	after := f.read(id)
	if after.InterpretationSetRevision != previous.InterpretationSetRevision || after.Freshness.RefreshFailedAt == nil {
		t.Fatal("cancellation changed head")
	}
	var count int
	if err = f.db.QueryRow(f.ctx, `SELECT count(*) FROM soil_screening_runs WHERE source_name=$1`, f.source).Scan(&count); err != nil || count != before {
		t.Fatalf("partial generation persisted: %d vs %d: %v", count, before, err)
	}
	f.screen(polygons)
	if f.read(id).Freshness.Stale {
		t.Fatal("successful refresh did not clear failure")
	}
	duplicate := append(append([]Polygon{}, polygons...), polygons[0])
	calls := f.calls.Load()
	if err = screen(f.ctx, f.db, f.server.Client(), f.server.URL, county.Union, f.source, duplicate, f.units, ScreeningOptions{SoilSetID: f.setID}); err == nil {
		t.Fatal("duplicate membership accepted")
	}
	if f.calls.Load() != calls {
		t.Fatal("duplicate membership reached USDA")
	}
}
