//go:build postgres

package main

import (
	"testing"
)

func TestParcelFieldHistoryAndExactSourceNumerics(t *testing.T) {
	ctx, dsn, pool := parcelFixtureDB(t)
	before := feature(1, "A", "before", true, 1)
	before["attributes"].(map[string]interface{})["mapped_acres"] = 0.1234567
	before["attributes"].(map[string]interface{})["TOTVAL"] = 12.345
	fixture := &parcelHTTPFixture{features: []map[string]interface{}{before}}
	source := fixture.source(t, 1)
	if _, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err != nil {
		t.Fatal(err)
	}
	after := feature(1, "A", "after", true, 1)
	after["attributes"].(map[string]interface{})["mapped_acres"] = 1.2345674
	after["attributes"].(map[string]interface{})["TOTVAL"] = 21.565
	after["attributes"].(map[string]interface{})["LAND_CODE"] = "new-use"
	fixture.features[0] = after
	if _, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err != nil {
		t.Fatal(err)
	}
	var oldName, newName, oldArea, newArea, oldValue, newValue, oldUse, newUse, rawArea, oldSource, newSource, oldGeometry, newGeometry string
	err := pool.QueryRow(ctx, `SELECT h.old_fields->>'current_name_1',h.new_fields->>'current_name_1',h.old_fields->>'mapped_acres',h.new_fields->>'mapped_acres',h.old_fields->>'valuation_assessed',h.new_fields->>'valuation_assessed',h.old_fields->>'land_code',h.new_fields->>'land_code',c.source_features #>> '{0,attributes,mapped_acres}',h.old_source_hash,h.new_source_hash,h.old_geometry_hash,h.new_geometry_hash FROM parcel_evidence_history h JOIN parcel_evidence_current c ON c.source_parcel_id=h.source_parcel_id WHERE h.old_observation_id IS NOT NULL ORDER BY h.id DESC LIMIT 1`).Scan(&oldName, &newName, &oldArea, &newArea, &oldValue, &newValue, &oldUse, &newUse, &rawArea, &oldSource, &newSource, &oldGeometry, &newGeometry)
	if err != nil {
		t.Fatal(err)
	}
	if oldName != "before" || newName != "after" || oldArea != "0.123457" || newArea != "1.234567" || rawArea != "1.2345674" || oldValue != "12.35" || newValue != "21.57" || oldUse != "01" || newUse != "new-use" || oldSource == newSource || oldGeometry != newGeometry {
		t.Fatalf("fields old=%q/%q acres=%q/%q raw=%q value=%q/%q use=%q/%q source_changed=%v geom_same=%v", oldName, newName, oldArea, newArea, rawArea, oldValue, newValue, oldUse, newUse, oldSource != newSource, oldGeometry == newGeometry)
	}
}
