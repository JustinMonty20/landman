package parcelapi

import (
	"encoding/base64"
	"math"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestFilters(t *testing.T) {
	for _, tc := range []string{
		"unknown=x", "zoom=14", "vacancy_tier=unknown&vacancy_tier=review", "min_acres=NaN", "max_acres=+Inf", "min_acres=-1", "min_acres=3&max_acres=2", "bbox=1,2,1,3", "bbox=181,0,182,1", "bbox=-180,-90,180,90", "bbox=0,85.051129,1,86", "bbox=0,-86,1,-85.051129", "bbox=0,0,1", "bbox=0,0,1,NaN", "viewport_bbox=0,0,1,NaN", "viewport_bbox=1,2,1,3", "page_size=101", "page_size=0", "vacancy_tier=occupied", "likely_vacant=true", "cursor=" + strings.Repeat("x", 2049),
	} {
		q, e := url.ParseQuery(tc)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = parseFilters(q); e == nil {
			t.Errorf("accepted %s", tc)
		}
	}
	f, e := parseFilters(url.Values{"bbox": {"-81,34,-80,35"}, "min_acres": {"1.5"}, "vacancy_tier": {"unknown"}, "delinquent": {"true"}})
	if e != nil || f.Bounds.West != -81 || *f.AcresMin != 1.5 || *f.VacancyTier != "unknown" || !*f.Delinquent {
		t.Fatalf("filter %+v: %v", f, e)
	}
	if f.PageSize != 50 || !f.active() {
		t.Fatal("wrong defaults")
	}
	for _, tier := range []string{"strong_candidate", "review", "conflict", "unknown"} {
		parsed, err := parseFilters(url.Values{"vacancy_tier": {tier}})
		if err != nil || parsed.VacancyTier == nil || *parsed.VacancyTier != tier || !parsed.active() {
			t.Fatalf("tier %s unavailable: %+v %v", tier, parsed, err)
		}
	}
	if _, e = finite("NaN", 0, math.MaxFloat64); e == nil {
		t.Fatal("NaN accepted")
	}
}
func TestMalformedQuery(t *testing.T) {
	w := httptest.NewRecorder()
	New(nil).ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/parcels?min_acres=1;max_acres=2", nil))
	if w.Code != 400 {
		t.Fatalf("malformed query returned %d", w.Code)
	}
}
func TestCursor(t *testing.T) {
	c := cursor{After: "00000000-0000-0000-0000-000000000001", Filter: strings.Repeat("a", 64), Stamp: strings.Repeat("b", 64)}
	got, e := decodeCursor(encodeCursor(c))
	if e != nil || got != c {
		t.Fatalf("cursor: %+v %v", got, e)
	}
	for _, v := range []string{"%%%", base64.RawURLEncoding.EncodeToString([]byte(`{"after":"not-a-uuid","filter":"a"}`))} {
		if _, e = decodeCursor(v); e == nil {
			t.Fatal("accepted invalid cursor")
		}
	}
	f, _ := parseFilters(url.Values{"min_acres": {"1"}})
	g, _ := parseFilters(url.Values{"min_acres": {"2"}})
	if f.digest() == g.digest() {
		t.Fatal("filter digest collision")
	}
	view, err := parseFilters(url.Values{"discovery": {"browse"}, "viewport_bbox": {"-81,34,-80,35"}})
	if err != nil || view.ViewportBounds == nil || view.Bounds != nil {
		t.Fatalf("viewport hint changed list filter: %+v %v", view, err)
	}
	moved, err := parseFilters(url.Values{"discovery": {"browse"}, "viewport_bbox": {"-80,34,-79,35"}})
	if err != nil || view.digest() == moved.digest() {
		t.Fatal("pan retained cursor identity")
	}
	if _, err := parseFilterValues(url.Values{"bbox": {"-81,34,-80,35"}, "viewport_bbox": {"-81,34,-80,35"}}, true); err == nil {
		t.Fatal("map accepted list priority")
	}
}

func TestDiscoveryFiltersAndCursorIdentity(t *testing.T) {
	legacy, err := parseFilters(url.Values{})
	if err != nil || legacy.active() {
		t.Fatalf("legacy empty list: %+v %v", legacy, err)
	}
	browse, err := parseFilters(url.Values{"discovery": {"browse"}})
	if err != nil || !browse.active() || browse.digest() == legacy.digest() {
		t.Fatalf("browse mode: %+v %v", browse, err)
	}
	combined, err := parseFilters(url.Values{"discovery": {"browse"}, "foreclosure": {"true"}, "soil_type": {"unit-1"}, "soil_use": {"conventional-septic"}, "soil_rating": {"Not limited"}})
	if err != nil || !combined.active() || len(combined.args()) != 20 || *combined.SoilUse != "208" {
		t.Fatalf("combined: %+v %v", combined, err)
	}
	for _, query := range []string{"discovery=all", "foreclosure=maybe", "soil_type=", "soil_use=conventional-septic", "soil_rating=Not+limited", "soil_use=other&soil_rating=Not+limited", "soil_use=conventional-septic&soil_rating=Not+rated"} {
		q, _ := url.ParseQuery(query)
		if _, err := parseFilters(q); err == nil {
			t.Errorf("accepted %s", query)
		}
	}
	mapFilter, err := parseFilterValues(url.Values{"bbox": {"-81,34,-80,35"}, "zoom": {"8"}, "discovery": {"browse"}, "soil_type": {"unit-1"}}, true)
	if err != nil || *mapFilter.SoilType != "unit-1" {
		t.Fatalf("map filters: %+v %v", mapFilter, err)
	}
	if mapFilter.digest() == combined.digest() {
		t.Fatal("semantic filter change retained cursor identity")
	}
}
