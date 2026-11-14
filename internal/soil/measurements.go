package soil

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// fetchMeasurements keeps every observation attached to a rated component and
// never collapses a month or horizon into a map-unit claim.
func fetchMeasurements(keys []string, components map[string]map[string]Fact, query func(string, int) ([][]any, error)) (map[string][]Measurement, error) {
	quoted := quotedKeys(keys)
	specs := []struct {
		kind, sql string
		columns   int
	}{
		{"month", fmt.Sprintf(`SELECT c.mukey,c.cokey,m.comonthkey,m.month,m.flodfreqcl,m.pondfreqcl FROM component c JOIN comonth m ON m.cokey=c.cokey WHERE c.mukey IN (%s) ORDER BY c.mukey,c.cokey,m.comonthkey`, quoted), 6},
		{"water_table_depth", fmt.Sprintf(`SELECT c.mukey,c.cokey,m.comonthkey,s.cosoilmoistkey,s.soimoistdept_r FROM component c JOIN comonth m ON m.cokey=c.cokey JOIN cosoilmoist s ON s.comonthkey=m.comonthkey WHERE c.mukey IN (%s) ORDER BY c.mukey,c.cokey,m.comonthkey,s.cosoilmoistkey`, quoted), 5},
		{"linear_extensibility", fmt.Sprintf(`SELECT c.mukey,c.cokey,h.hzname,h.hzdept_r,h.hzdepb_r,h.lep_r FROM component c JOIN chorizon h ON h.cokey=c.cokey WHERE c.mukey IN (%s) ORDER BY c.mukey,c.cokey,h.hzdept_r,h.hzdepb_r,h.hzname`, quoted), 6},
		{"restriction_depth", fmt.Sprintf(`SELECT c.mukey,c.cokey,r.resdept_r,r.reskind FROM component c JOIN corestrictions r ON r.cokey=c.cokey WHERE c.mukey IN (%s) ORDER BY c.mukey,c.cokey,r.resdept_r,r.reskind`, quoted), 4},
	}
	expected := map[string]bool{}
	for _, key := range keys {
		expected[key] = true
	}
	result := map[string][]Measurement{}
	months := map[string]string{}
	monthCount := map[string]int{}
	calendarMonths := map[string]bool{}
	seen := map[string]bool{}
	add := func(row []any, spec struct {
		kind, sql string
		columns   int
	}, source, kind, raw, unit, month string, top, bottom *float64) error {
		key, _ := field(row, 0)
		component, _ := field(row, 1)
		if !expected[key] || component == "" || source == "" {
			return fmt.Errorf("unexpected USDA %s identity %s/%s/%s", kind, key, component, source)
		}
		identity := key + ":" + component + ":" + kind + ":" + source
		if seen[identity] {
			return fmt.Errorf("duplicate USDA %s %s", kind, identity)
		}
		seen[identity] = true
		value := Fact{Availability: Unavailable, MissingReason: "USDA returned no value", Unit: unit}
		if raw != "" {
			value = Fact{RawValue: &raw, Value: &raw, Unit: unit, Availability: Rated}
		}
		position := len(result[key])
		result[key] = append(result[key], Measurement{ComponentKey: component, Kind: kind, SourceKey: source, Month: month, Position: position, TopCM: top, BottomCM: bottom, Value: value, SourceQuery: spec.sql, FetchedAt: time.Now().UTC()})
		return nil
	}
	for _, spec := range specs {
		rows, err := query(spec.sql, spec.columns)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			key, _ := field(row, 0)
			component, _ := field(row, 1)
			if !expected[key] || component == "" {
				return nil, fmt.Errorf("unexpected USDA %s component %s/%s", spec.kind, key, component)
			}
			if _, ok := components[key+":"+component]; !ok {
				return nil, fmt.Errorf("USDA %s references unknown component %s/%s", spec.kind, key, component)
			}
			switch spec.kind {
			case "month":
				id, _ := field(row, 2)
				month, _ := field(row, 3)
				if !validMonth(month) || id == "" {
					return nil, fmt.Errorf("unsupported USDA month %q", month)
				}
				mk := key + ":" + component + ":" + id
				if _, ok := months[mk]; ok {
					return nil, errors.New("duplicate USDA month key")
				}
				calendarID := key + ":" + component + ":" + month
				if calendarMonths[calendarID] {
					return nil, fmt.Errorf("duplicate USDA calendar month %s", calendarID)
				}
				calendarMonths[calendarID] = true
				months[mk] = month
				monthCount[key+":"+component]++
				for i, kind := range []string{"flood_frequency", "ponding_frequency"} {
					raw, _ := field(row, 4+i)
					if raw != "" && !validFrequency(raw) {
						return nil, fmt.Errorf("unsupported USDA %s %q", kind, raw)
					}
					if err = add(row, spec, id, kind, raw, "", month, nil, nil); err != nil {
						return nil, err
					}
				}
			case "water_table_depth":
				monthKey, _ := field(row, 2)
				id, _ := field(row, 3)
				raw, _ := field(row, 4)
				month, ok := months[key+":"+component+":"+monthKey]
				if !ok {
					return nil, errors.New("USDA moisture has no matching month")
				}
				if err = validNonnegative(raw); err != nil {
					return nil, err
				}
				if err = add(row, spec, id, "water_table_depth", raw, "cm", month, nil, nil); err != nil {
					return nil, err
				}
			case "linear_extensibility":
				horizon, _ := field(row, 2)
				topRaw, _ := field(row, 3)
				bottomRaw, _ := field(row, 4)
				raw, _ := field(row, 5)
				top, e := depth(topRaw)
				if e != nil {
					return nil, e
				}
				bottom, e := depth(bottomRaw)
				if e != nil {
					return nil, e
				}
				if top == nil || bottom == nil || *bottom <= *top || horizon == "" {
					return nil, errors.New("invalid USDA horizon depth or identity")
				}
				if err = validNonnegative(raw); err != nil {
					return nil, err
				}
				if err = add(row, spec, horizon+":"+topRaw+":"+bottomRaw, spec.kind, raw, "percent", "", top, bottom); err != nil {
					return nil, err
				}
			case "restriction_depth":
				raw, _ := field(row, 2)
				kind, _ := field(row, 3)
				if kind == "" || raw == "" {
					return nil, errors.New("invalid USDA restriction identity")
				}
				if err = validNonnegative(raw); err != nil {
					return nil, err
				}
				if err = add(row, spec, kind+":"+raw, spec.kind, raw, "cm", "", nil, nil); err != nil {
					return nil, err
				}
			}
		}
	}
	for component, count := range monthCount {
		if count != 12 {
			return nil, fmt.Errorf("incomplete USDA monthly observations for %s: %d", component, count)
		}
	}
	for identity := range components {
		parts := strings.SplitN(identity, ":", 2)
		key, component := parts[0], parts[1]
		for _, missing := range []struct {
			kind, unit string
			spec       int
		}{
			{"flood_frequency", "", 0}, {"ponding_frequency", "", 0},
			{"water_table_depth", "cm", 1}, {"linear_extensibility", "percent", 2},
			{"restriction_depth", "cm", 3},
		} {
			found := false
			for _, m := range result[key] {
				if m.ComponentKey == component && m.Kind == missing.kind {
					found = true
					break
				}
			}
			if !found {
				result[key] = append(result[key], Measurement{ComponentKey: component, Kind: missing.kind, SourceKey: "unavailable", Position: len(result[key]), Value: Fact{Availability: Unavailable, Unit: missing.unit, MissingReason: "USDA returned no rows for component"}, SourceQuery: specs[missing.spec].sql, FetchedAt: time.Now().UTC()})
			}
		}
	}
	return result, nil
}
func depth(s string) (*float64, error) {
	if s == "" {
		return nil, nil
	}
	if err := validNonnegative(s); err != nil {
		return nil, err
	}
	v, _ := strconv.ParseFloat(s, 64)
	return &v, nil
}
func validNonnegative(s string) error {
	if s == "" {
		return nil
	}
	v, e := strconv.ParseFloat(s, 64)
	if e != nil || v < 0 || v > 100000 || strings.ContainsAny(s, "nNiIfF") {
		return fmt.Errorf("unsupported USDA measurement %q", s)
	}
	return nil
}
func validMonth(s string) bool {
	for _, m := range []string{"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"} {
		if m == s {
			return true
		}
	}
	return false
}
func validFrequency(s string) bool {
	switch s {
	case "None", "Very rare", "Rare", "Occasional", "Frequent", "Very frequent":
		return true
	}
	return false
}
