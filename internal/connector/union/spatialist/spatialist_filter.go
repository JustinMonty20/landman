package spatialist

// KeepWhenYearBuiltIsNil keeps a Spatialist parcel only when
// parcel.sections[2][0][0].YEARBLT is present and nil.
//
// If the expected path is missing or malformed, this returns true so the
// record is not dropped due to upstream payload drift.
func KeepWhenYearBuiltIsNil(raw map[string]interface{}, _ *SpatialistFlatEnvelope) bool {
	if raw == nil {
		return true
	}

	parcel, ok := raw["parcel"].(map[string]interface{})
	if !ok {
		return true
	}

	sections, ok := parcel["sections"].([]interface{})
	if !ok || len(sections) <= 2 {
		return true
	}

	section2, ok := sections[2].([]interface{})
	if !ok || len(section2) == 0 {
		return true
	}

	row0, ok := section2[0].([]interface{})
	if !ok || len(row0) == 0 {
		return true
	}

	entry, ok := row0[0].(map[string]interface{})
	if !ok {
		return true
	}

	yearBuilt, exists := entry["YEARBLT"]
	if !exists {
		return true
	}

	return yearBuilt == nil
}
