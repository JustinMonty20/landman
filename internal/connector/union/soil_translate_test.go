package union

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/JustinMonty20/landman/internal/county"
	"github.com/twpayne/go-geom"
)

func TestParseSoilFeature(t *testing.T) {
	data, err := os.ReadFile("testdata/soil/layer197-feature.json")
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		Features []map[string]interface{} `json:"features"`
	}
	if err := json.Unmarshal(data, &response); err != nil || len(response.Features) != 1 {
		t.Fatalf("ChA fixture: %v, features %d", err, len(response.Features))
	}
	parse := func(feature map[string]interface{}) error {
		t.Helper()
		encoded, err := json.Marshal(feature)
		if err != nil {
			t.Fatal(err)
		}
		_, _, err = ParseSoilFeature(encoded)
		return err
	}
	first := response.Features[0]
	polygons, units, err := ParseSoilFeatures(data)
	if err != nil || len(polygons) != 1 || len(units) != 1 {
		t.Fatalf("parse live ChA: %v, polygons %d, units %d", err, len(polygons), len(units))
	}
	p, u := polygons[0], units[0]
	if p.County != county.Union || p.SourceName != "union_county_gis" || p.ObjectID != "1" || p.MapUnitKey != "115341" || p.SpatialVersion == nil || *p.SpatialVersion != 5 || p.SurveySavedOn != nil {
		t.Fatalf("unexpected polygon: %+v", p)
	}
	if u.MapUnitKey != "115341" || u.MapUnitSymbol != "ChA" || u.AreaSymbol != "NC179" || u.MapUnitName != nil || u.AreaName != nil {
		t.Fatalf("unexpected map unit: %+v", u)
	}
	polygon, ok := (*p.Shape).(*geom.Polygon)
	if !ok || polygon.SRID() != 3857 {
		t.Fatalf("expected polygon SRID 3857, got %v", p.Shape)
	}
	fixturePoint := first["geometry"].(map[string]interface{})["rings"].([]interface{})[0].([]interface{})[0].([]interface{})
	if polygon.FlatCoords()[0] != fixturePoint[0] || polygon.FlatCoords()[1] != fixturePoint[1] {
		t.Fatalf("first coordinate differs from ChA fixture: %v", polygon.FlatCoords()[:2])
	}
	if _, hasHydric := reflect.TypeOf(p).FieldByName("HydrcRatng"); hasHydric {
		t.Fatal("hydric rating must not be stored")
	}
	attrs := first["attributes"].(map[string]interface{})
	original := attrs["MUKEY"]
	delete(attrs, "MUKEY")
	if err := parse(first); err == nil {
		t.Fatal("missing MUKEY accepted")
	}
	attrs["MUKEY"] = original
	original = attrs["MUSYM"]
	delete(attrs, "MUSYM")
	if err := parse(first); err == nil {
		t.Fatal("missing MUSYM accepted")
	}
	attrs["MUSYM"] = original
	attrs["OBJECTID"] = nil
	attrs["unexpected"] = "ignored"
	if err := parse(first); err != nil {
		t.Fatalf("null OBJECTID or extra field rejected: %v", err)
	}
	geometry := first["geometry"]
	first["geometry"] = map[string]interface{}{"rings": []interface{}{}}
	if err := parse(first); err == nil {
		t.Fatal("empty rings accepted")
	}
	first["geometry"] = geometry
	for i := 0; i < 2; i++ {
		again, againUnit, err := ParseSoilFeature(mustJSON(t, first))
		if err != nil || !reflect.DeepEqual(again, p) || !reflect.DeepEqual(againUnit, u) || again.SurveySavedOn != nil {
			t.Fatalf("repeat parse %d: polygon %+v, unit %+v, error %v", i, again, againUnit, err)
		}
	}
	if empty, emptyUnits, err := ParseSoilFeatures([]byte(`{"features":[]}`)); err != nil || len(empty) != 0 || len(emptyUnits) != 0 {
		t.Fatalf("empty list: %v, %v, %v", empty, emptyUnits, err)
	}
	if _, _, err := ParseSoilFeatures([]byte(`{"error":{"code":400,"message":"bad query"}}`)); err == nil {
		t.Fatal("ArcGIS error accepted")
	}
	if _, _, err := ParseSoilFeature([]byte(`{"error":{"code":400}}`)); err == nil {
		t.Fatal("ArcGIS feature error accepted")
	}
}

func mustJSON(t *testing.T, value interface{}) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
