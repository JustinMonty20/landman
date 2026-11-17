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
		"unknown=x", "likely_vacant=true&likely_vacant=false", "min_acres=NaN", "max_acres=+Inf", "min_acres=-1", "min_acres=3&max_acres=2", "bbox=1,2,1,3", "bbox=181,0,182,1", "bbox=0,0,1", "bbox=0,0,1,NaN", "page_size=101", "page_size=0", "likely_vacant=1", "cursor=" + strings.Repeat("x", 2049),
	} {
		q, e := url.ParseQuery(tc)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = parseFilters(q); e == nil {
			t.Errorf("accepted %s", tc)
		}
	}
	f, e := parseFilters(url.Values{"bbox": {"-81,34,-80,35"}, "min_acres": {"1.5"}, "likely_vacant": {"false"}, "delinquent": {"true"}})
	if e != nil || f.Bounds.West != -81 || *f.AcresMin != 1.5 || *f.LikelyVacant || !*f.Delinquent {
		t.Fatalf("filter %+v: %v", f, e)
	}
	if f.PageSize != 50 || !f.active() {
		t.Fatal("wrong defaults")
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
	c := cursor{"00000000-0000-0000-0000-000000000001", strings.Repeat("a", 64), strings.Repeat("b", 64)}
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
}
