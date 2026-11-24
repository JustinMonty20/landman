package vacancy

// ValueRow retains the appraisal category, year and numeric building value
// separately from parcel GIS facts. Nil value is missing, not zero.
type ValueRow struct {
	Year     int
	Category string
	Building *float64
}

// SelectBuildingValue only accepts the operator-approved effective AV year.
// Identical repeated rows are one observation; competing or missing values
// never become a zero-value conclusion.
func SelectBuildingValue(rows []ValueRow, effectiveYear int) (*bool, bool) {
	if effectiveYear <= 0 {
		return nil, true
	}
	var selected *float64
	seen := false
	for _, row := range rows {
		if row.Year != effectiveYear || row.Category != "AV" {
			continue
		}
		if row.Building == nil || *row.Building < 0 {
			return nil, true
		}
		if seen && *row.Building != *selected {
			return nil, true
		}
		selected, seen = row.Building, true
	}
	if !seen {
		return nil, true
	}
	positive := *selected > 0
	return &positive, false
}
