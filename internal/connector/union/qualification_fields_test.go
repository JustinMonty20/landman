package union

import (
	"encoding/json"
	"testing"
)

func TestCapturedMunicipalDistrictCode(t *testing.T) {
	for _, id := range []int{176, 183} {
		var capture struct {
			Features []struct {
				Attributes map[string]interface{} `json:"attributes"`
			} `json:"features"`
			Provenance string `json:"provenance"`
		}
		if e := json.Unmarshal(fixture(t, id, "municipal-fields"), &capture); e != nil {
			t.Fatal(e)
		}
		if len(capture.Features) != 2 || capture.Provenance == "" {
			t.Fatalf("layer %d lacks captured field provenance", id)
		}
		for _, item := range capture.Features {
			a := item.Attributes
			if str(a, "Zoning") == "" || str(a, "Zoning") == str(a, "ZoneDesc") || str(a, "Zoning") == str(a, "CorpId") {
				t.Fatalf("layer %d municipal code mapping %v", id, a)
			}
		}
	}
}
