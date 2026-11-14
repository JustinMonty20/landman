package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestClassifyCountySlowWithoutCommitDoesNotNeedSales(t *testing.T) {
	out := classify(classifyInput{
		stderr: slowDriveLog(30*time.Second) + "context deadline exceeded\n",
		probe:  fastMetadataProbe(0),
	})
	if out.Verdict != verdictCountySlow {
		t.Fatalf("verdict %q, want county-source-slow", out.Verdict)
	}
	if out.code() != 2 {
		t.Fatalf("exit %d, want 2", out.code())
	}
	if out.SeededSales != tierNotRun || out.FullImport != tierMissed {
		t.Fatalf("tiers sales %q import %q", out.SeededSales, out.FullImport)
	}
	if out.Sales != nil {
		t.Fatalf("sales %#v, want nil", out.Sales)
	}
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte(`"sales":null`)) {
		t.Fatalf("sales JSON %s", raw)
	}
}

func TestClassifyFailWhenGapsAndPageAreFast(t *testing.T) {
	out := classify(classifyInput{
		stderr: slowDriveLog(2*time.Second) + "context deadline exceeded\n",
		probe:  fastMetadataProbe(2 * time.Second),
	})
	if out.Verdict != verdictFail {
		t.Fatalf("verdict %q, want fail", out.Verdict)
	}
	if out.Sales != nil {
		t.Fatalf("sales %#v, want nil", out.Sales)
	}
}

func TestClassifyRejectsSlowVerdictWithoutFastMetadataOrSlowPage(t *testing.T) {
	out := classify(classifyInput{
		stderr: slowDriveLog(2*time.Second) + "rate: Wait(n=1) would exceed context deadline\n",
		probe: pageTimings{
			ParcelMetadata: timedCall{HTTPStatus: 200, DurationMS: 10_000},
			SoilMetadata:   timedCall{HTTPStatus: 500, DurationMS: 100},
			ParcelPage:     timedCall{HTTPStatus: 200, DurationMS: 2_000},
			SoilPage:       timedCall{HTTPStatus: 200, DurationMS: 2_000},
		},
	})
	if out.Verdict == verdictCountySlow {
		t.Fatal("county-source-slow without a fast metadata sample and a slow page or batch gap")
	}
	if out.Verdict != verdictFail {
		t.Fatalf("verdict %q, want fail", out.Verdict)
	}
}

func TestClassifyCountsGapThatEndsAtBatchOne(t *testing.T) {
	start := time.Date(2026, 11, 14, 17, 0, 0, 0, time.UTC)
	logText := fmt.Sprintf("%s [union_county_gis] Total parcels to fetch: 4000\n%s [union_county_gis] Batch 1/2 complete: fetched 2000 records (total so far: 2000)\n%s [union_county_gis] Batch 2/2 complete: fetched 2000 records (total so far: 4000)\ncontext deadline exceeded\n",
		start.Format("2006/01/02 15:04:05"),
		start.Add(40*time.Second).Format("2006/01/02 15:04:05"),
		start.Add(42*time.Second).Format("2006/01/02 15:04:05"))
	out := classify(classifyInput{stderr: logText, probe: fastMetadataProbe(2 * time.Second)})
	if out.Verdict != verdictCountySlow {
		t.Fatalf("verdict %q, want county-source-slow", out.Verdict)
	}
}

func TestClassifyPassWhenZeroAmountDeedMatches(t *testing.T) {
	attrs := map[string]any{
		"s2_SALESAMT":  0,
		"s2_SALEDATE":  1790726400000,
		"s2_DEED_BOOK": "9472",
		"s2_DEED_PAGE": "0680",
	}
	expected := expectedSaleSlots("07102010", attrs)
	if len(expected) != 1 {
		t.Fatalf("expected slots %#v", expected)
	}
	actual := []saleSlot{{
		SourceParcelID: "07102010",
		Sequence:       2,
		SaleDate:       "2026-09-30",
		Amount:         "0.00",
		DeedBook:       "9472",
		DeedPage:       "0680",
	}}
	out := classify(classifyInput{
		stdout:      "Imported parcels, 4 likely-vacant feature IDs\n",
		processExit: 0,
		salesRead:   true,
		expected:    expected,
		actual:      actual,
		post:        matchingPost(),
	})
	if out.Verdict != verdictPass || out.code() != 0 {
		t.Fatalf("verdict %q exit %d problems %v", out.Verdict, out.code(), out.Problems)
	}
	if out.SeededSales != tierMet || out.FullImport != tierMet {
		t.Fatalf("tiers sales %q import %q", out.SeededSales, out.FullImport)
	}
	if len(out.Sales) != 1 || out.Sales[0].Amount != "0.00" {
		t.Fatalf("sales %#v", out.Sales)
	}
}

func TestClassifyFailWhenEmptySlotHasDatabaseRow(t *testing.T) {
	expected := expectedSaleSlots("07102010", map[string]any{
		"s1_SALESAMT":  0,
		"s1_DEED_BOOK": "  ",
		"s1_DEED_PAGE": "",
	})
	if len(expected) != 0 {
		t.Fatalf("empty slot kept %#v", expected)
	}
	out := classify(classifyInput{
		stdout:      "Imported parcels, 1 likely-vacant feature IDs\n",
		processExit: 0,
		salesRead:   true,
		expected:    expected,
		actual: []saleSlot{{
			SourceParcelID: "07102010",
			Sequence:       1,
			Amount:         "0.00",
		}},
		post: matchingPost(),
	})
	if out.Verdict != verdictFail || out.SeededSales != tierMissed {
		t.Fatalf("verdict %q sales tier %q", out.Verdict, out.SeededSales)
	}
	if !strings.Contains(strings.Join(out.Problems, "\n"), "presence 07102010 sequence 1") {
		t.Fatalf("problems %v, want presence", out.Problems)
	}
}

func TestClassifyCountySlowAfterCommitWhenPageIsSlow(t *testing.T) {
	expected := expectedSaleSlots("07102010", map[string]any{"s1_DEED_BOOK": "9472"})
	actual := append([]saleSlot(nil), expected...)
	actual[0].Amount = ""
	out := classify(classifyInput{
		stdout:      "Imported parcels, 2 likely-vacant feature IDs\n",
		stderr:      "context deadline exceeded\n",
		processExit: 1,
		probe:       fastMetadataProbe(30 * time.Second),
		salesRead:   true,
		expected:    expected,
		actual:      actual,
	})
	if out.Verdict != verdictCountySlow || out.code() != 2 {
		t.Fatalf("verdict %q exit %d problems %v", out.Verdict, out.code(), out.Problems)
	}
	if out.SeededSales != tierMet || out.FullImport != tierMissed {
		t.Fatalf("tiers sales %q import %q", out.SeededSales, out.FullImport)
	}
	if out.Sales == nil {
		t.Fatal("sales nil after commit")
	}
}

func TestExpectedSaleSlotsKeepsSequenceThreeDeedOnly(t *testing.T) {
	slots := expectedSaleSlots("07102067", map[string]any{
		"s1_SALESAMT":  0,
		"s1_SALEDATE":  0,
		"s1_DEED_BOOK": "",
		"s2_SALESAMT":  0.0,
		"s2_grantor":   "   ",
		"s3_DEED_BOOK": "6316",
	})
	if len(slots) != 1 {
		t.Fatalf("slots %#v, want only sequence 3", slots)
	}
	if slots[0].Sequence != 3 || slots[0].DeedBook != "6316" || slots[0].Amount != "" || slots[0].SaleDate != "" {
		t.Fatalf("slot %#v", slots[0])
	}
}

func TestDriveOutcomeJSONHasNoBoolean(t *testing.T) {
	out := classify(classifyInput{
		stdout:      "Imported parcels, 1 likely-vacant feature IDs\n",
		processExit: 0,
		salesRead:   true,
		expected:    expectedSaleSlots("1", map[string]any{"s1_DEED_PAGE": "0680", "s1_SALESAMT": 0}),
		actual: []saleSlot{{
			SourceParcelID: "1",
			Sequence:       1,
			Amount:         "0.00",
			DeedPage:       "0680",
		}},
		post: matchingPost(),
	})
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	if jsonHasBool(payload) {
		t.Fatalf("boolean in %s", raw)
	}
}

func TestOracleSourceDoesNotCallBuildSales(t *testing.T) {
	src, err := os.ReadFile("slots.go")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(src, []byte("buildSales")) {
		t.Fatal("expectedSaleSlots calls buildSales")
	}
}

func slowDriveLog(gap time.Duration) string {
	start := time.Date(2026, 11, 14, 17, 0, 0, 0, time.UTC)
	return fmt.Sprintf("%s [union_county_gis] Batch 1/2 complete: fetched 2000 records (total so far: 2000)\n%s [union_county_gis] Batch 2/2 complete: fetched 2000 records (total so far: 4000)\n",
		start.Format("2006/01/02 15:04:05"),
		start.Add(gap).Format("2006/01/02 15:04:05"))
}

func fastMetadataProbe(page time.Duration) pageTimings {
	meta := timedCall{HTTPStatus: 200, DurationMS: 400}
	sample := timedCall{HTTPStatus: 200, DurationMS: page.Milliseconds()}
	return pageTimings{ParcelMetadata: meta, SoilMetadata: meta, ParcelPage: sample, SoilPage: sample}
}

func matchingPost() *postImport {
	return &postImport{
		CountyParcels:         100,
		SeedParcels:           36,
		SoilPolygons:          20,
		SeedSoilPolygons:      7,
		Overlaps:              80,
		SeedOverlaps:          71,
		ParcelJobSuccesses:    1,
		ScreeningJobSuccesses: 1,
		ScreeningHeads:        1,
		Assessments:           100,
	}
}

func jsonHasBool(value any) bool {
	switch typed := value.(type) {
	case bool:
		return true
	case map[string]any:
		for _, child := range typed {
			if jsonHasBool(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if jsonHasBool(child) {
				return true
			}
		}
	}
	return false
}
