package spatialist

import (
	"encoding/json"
	"testing"
)

func TestFlattenSpatialistRecord(t *testing.T) {
	const payload = `{
		"id": "01033003B",
		"nbhd": 4102001,
		"ctx": "-80.30896607484091",
		"cty": "35.11742129828186",
		"active": true,
		"imageid": "01033003B",
		"sketchid": "01033003B",
		"parcel": {
			"header": {
				"PID": "01033003B",
				"PHYSSTRADD": "4614 NAPIER STEGALL RD"
			},
			"bounds": "MULTIPOLYGON(((-80.30919850193993 35.116825966309385)))",
			"mapicon": null,
			"mapiconselected": null,
			"sections": [
				[
					[
						{"PID": "01033003B", "PropClassDesc": "RESIDENTIAL"}
					],
					[
						{"FMV_LAND": "$44,200", "FMV_TOTAL": "$787,500"}
					]
				],
				[],
				[],
				[
					[
						{
							"row": {"document_number": 150273, "gross_selling_price": "$16,000"},
							"subdata": {"grantor": "STEGALL BILLY WARD ET AL", "grantee": "THOMAS CAMERON BRUCE"}
						}
					]
				],
				[
					[
						{"assess_date": "02/27/2025", "TotalAssessed": "$787,500"},
						{"assess_date": "02/15/2024", "TotalAssessed": "$671,000"}
					]
				],
				[
					[
						{
							"row": {"PermitNumber": 201706136, "IssuedDate": "11/06/2017"},
							"subdata": {"PermitDescription": "new single family"}
						}
					]
				]
			]
		}
	}`

	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(payload), &raw); err != nil {
		t.Fatalf("failed to unmarshal payload: %v", err)
	}

	flat, err := FlattenSpatialistRecord(raw)
	if err != nil {
		t.Fatalf("FlattenSpatialistRecord error: %v", err)
	}

	if flat.UCRealPropertySearch == nil {
		t.Fatal("expected UCRealPropertySearch to be populated")
	}

	record := flat.UCRealPropertySearch

	if record.ParcelID != "01033003B" {
		t.Fatalf("ParcelID = %q, want %q", record.ParcelID, "01033003B")
	}

	if record.Valuation.OGFMVTotal != "$787,500" {
		t.Fatalf("OGFMVTotal = %v, want %v", record.Valuation.OGFMVTotal, "$787,500")
	}
	if record.Valuation.FMVTotal != int64(787500) {
		t.Fatalf("FMVTotal = %v, want %v", record.Valuation.FMVTotal, int64(787500))
	}

	if len(record.Sales) != 1 {
		t.Fatalf("Sales len = %d, want 1", len(record.Sales))
	}
	if record.Sales[0].Grantor != "STEGALL BILLY WARD ET AL" {
		t.Fatalf("Grantor = %v, want %v", record.Sales[0].Grantor, "STEGALL BILLY WARD ET AL")
	}
	if record.Sales[0].OGGrossSellingPrice != "$16,000" {
		t.Fatalf("OGGrossSellingPrice = %v, want %v", record.Sales[0].OGGrossSellingPrice, "$16,000")
	}
	if record.Sales[0].GrossSellingPrice != int64(16000) {
		t.Fatalf("GrossSellingPrice = %v, want %v", record.Sales[0].GrossSellingPrice, int64(16000))
	}

	if len(record.Assessments) != 2 {
		t.Fatalf("Assessments len = %d, want 2", len(record.Assessments))
	}
	if record.Assessments[1].OGTotalAssessed != "$671,000" {
		t.Fatalf("OGTotalAssessed = %v, want %v", record.Assessments[1].OGTotalAssessed, "$671,000")
	}
	if record.Assessments[1].TotalAssessed != int64(671000) {
		t.Fatalf("TotalAssessed = %v, want %v", record.Assessments[1].TotalAssessed, int64(671000))
	}

	if len(record.Permits) != 1 {
		t.Fatalf("Permits len = %d, want 1", len(record.Permits))
	}
	if record.Permits[0].PermitDescription != "new single family" {
		t.Fatalf("PermitDescription = %v, want %v", record.Permits[0].PermitDescription, "new single family")
	}
}
