package main

import (
	"fmt"
	"strings"
)

type seedSnapshot struct {
	columns  map[string][]string
	known    map[string]map[string]struct{}
	counts   map[string]int
	vacant   int
	built    int
	parcels  []seedParcel
	mapUnits map[string]seedMapUnit
}

type seedParcel struct {
	ID             string
	SourceParcelID string
	LikelyVacant   bool
	Situs          string
}

type seedMapUnit struct {
	Key      string
	Name     string
	AreaName string
	HasName  bool
	HasArea  bool
}

func parseSeed(text string) (seedSnapshot, error) {
	snap := seedSnapshot{
		columns:  map[string][]string{},
		known:    map[string]map[string]struct{}{},
		counts:   map[string]int{},
		mapUnits: map[string]seedMapUnit{},
	}
	seenParcels := map[string]struct{}{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(strings.ToUpper(line), "INSERT INTO ") {
			continue
		}
		table, cols, rest, err := parseInsertHead(line)
		if err != nil {
			return seedSnapshot{}, err
		}
		snap.addColumns(table, cols)
		snap.counts[table]++
		switch table {
		case "parcels":
			parcel, err := parcelFromValues(cols, rest)
			if err != nil {
				return seedSnapshot{}, err
			}
			if _, dup := seenParcels[parcel.SourceParcelID]; dup {
				return seedSnapshot{}, fmt.Errorf("duplicate source_parcel_id %s", parcel.SourceParcelID)
			}
			seenParcels[parcel.SourceParcelID] = struct{}{}
			snap.parcels = append(snap.parcels, parcel)
			if parcel.LikelyVacant {
				snap.vacant++
			} else {
				snap.built++
			}
		case "soil_map_units":
			unit, err := mapUnitFromValues(cols, rest)
			if err != nil {
				return seedSnapshot{}, err
			}
			snap.mapUnits[unit.Key] = unit
		}
	}
	if len(snap.parcels) == 0 {
		return seedSnapshot{}, fmt.Errorf("seed has no parcels")
	}
	return snap, nil
}

func (s *seedSnapshot) addColumns(table string, cols []string) {
	if s.known[table] == nil {
		s.known[table] = map[string]struct{}{}
	}
	for _, col := range cols {
		if _, ok := s.known[table][col]; ok {
			continue
		}
		s.known[table][col] = struct{}{}
		s.columns[table] = append(s.columns[table], col)
	}
}

func (s seedSnapshot) parcelIDs() []string {
	ids := make([]string, len(s.parcels))
	for i, parcel := range s.parcels {
		ids[i] = parcel.SourceParcelID
	}
	return ids
}

func (s seedSnapshot) parcel(id string) (seedParcel, bool) {
	for _, parcel := range s.parcels {
		if parcel.SourceParcelID == id {
			return parcel, true
		}
	}
	return seedParcel{}, false
}

func parseInsertHead(line string) (string, []string, string, error) {
	rest := strings.TrimSpace(line[len("INSERT INTO"):])
	table, rest, ok := takeIdent(rest)
	if !ok {
		return "", nil, "", fmt.Errorf("insert missing table")
	}
	rest = strings.TrimSpace(rest)
	cols, rest, err := parseColumnList(rest)
	if err != nil {
		return "", nil, "", fmt.Errorf("%s: %w", table, err)
	}
	return table, cols, strings.TrimSpace(rest), nil
}

func takeIdent(text string) (string, string, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", "", false
	}
	i := 0
	for i < len(text) {
		c := text[i]
		if c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || (i > 0 && c >= '0' && c <= '9') {
			i++
			continue
		}
		break
	}
	if i == 0 {
		return "", "", false
	}
	return text[:i], text[i:], true
}

func parseColumnList(text string) ([]string, string, error) {
	if text == "" || text[0] != '(' {
		return nil, "", fmt.Errorf("missing column list")
	}
	end := strings.IndexByte(text, ')')
	if end < 0 {
		return nil, "", fmt.Errorf("column list not closed")
	}
	var cols []string
	for _, part := range strings.Split(text[1:end], ",") {
		col := strings.TrimSpace(part)
		if col == "" {
			return nil, "", fmt.Errorf("empty column")
		}
		cols = append(cols, col)
	}
	return cols, text[end+1:], nil
}

func parcelFromValues(cols []string, rest string) (seedParcel, error) {
	vals, err := valuesOf(rest)
	if err != nil {
		return seedParcel{}, err
	}
	if len(vals) != len(cols) {
		return seedParcel{}, fmt.Errorf("parcels has %d values for %d columns", len(vals), len(cols))
	}
	id, _, err := requiredString(cols, vals, "id")
	if err != nil {
		return seedParcel{}, err
	}
	sourceID, _, err := requiredString(cols, vals, "source_parcel_id")
	if err != nil {
		return seedParcel{}, err
	}
	situs, _, err := requiredString(cols, vals, "situs_line_1")
	if err != nil {
		return seedParcel{}, err
	}
	vacant, err := requiredBool(cols, vals, "likely_vacant")
	if err != nil {
		return seedParcel{}, err
	}
	return seedParcel{ID: id, SourceParcelID: sourceID, LikelyVacant: vacant, Situs: situs}, nil
}

func mapUnitFromValues(cols []string, rest string) (seedMapUnit, error) {
	vals, err := valuesOf(rest)
	if err != nil {
		return seedMapUnit{}, err
	}
	if len(vals) != len(cols) {
		return seedMapUnit{}, fmt.Errorf("soil_map_units has %d values for %d columns", len(vals), len(cols))
	}
	key, _, err := requiredString(cols, vals, "map_unit_key")
	if err != nil {
		return seedMapUnit{}, err
	}
	name, hasName, err := optionalString(cols, vals, "map_unit_name")
	if err != nil {
		return seedMapUnit{}, err
	}
	area, hasArea, err := optionalString(cols, vals, "area_name")
	if err != nil {
		return seedMapUnit{}, err
	}
	return seedMapUnit{Key: key, Name: name, AreaName: area, HasName: hasName, HasArea: hasArea}, nil
}

func valuesOf(rest string) ([]string, error) {
	upper := strings.ToUpper(rest)
	if !strings.HasPrefix(upper, "VALUES") {
		return nil, fmt.Errorf("expected VALUES")
	}
	return splitTuple(strings.TrimSpace(rest[len("VALUES"):]))
}

func splitTuple(body string) ([]string, error) {
	startTuple := strings.IndexByte(body, '(')
	if startTuple < 0 {
		return nil, fmt.Errorf("values missing tuple")
	}
	depth := 1
	inString := false
	start := startTuple + 1
	var vals []string
	for i := startTuple + 1; i < len(body); i++ {
		c := body[i]
		if inString {
			if c != '\'' {
				continue
			}
			if i+1 < len(body) && body[i+1] == '\'' {
				i++
				continue
			}
			inString = false
			continue
		}
		switch c {
		case '\'':
			inString = true
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				vals = append(vals, strings.TrimSpace(body[start:i]))
				return vals, nil
			}
		case ',':
			if depth == 1 {
				vals = append(vals, strings.TrimSpace(body[start:i]))
				start = i + 1
			}
		}
	}
	return nil, fmt.Errorf("values tuple not closed")
}

func requiredString(cols, vals []string, name string) (string, bool, error) {
	text, ok, isNull, err := columnString(cols, vals, name)
	if err != nil {
		return "", false, err
	}
	if !ok {
		return "", false, fmt.Errorf("missing column %s", name)
	}
	if isNull || text == "" {
		return "", false, fmt.Errorf("column %s is empty", name)
	}
	return text, true, nil
}

func optionalString(cols, vals []string, name string) (string, bool, error) {
	text, ok, isNull, err := columnString(cols, vals, name)
	if err != nil || !ok || isNull {
		return "", false, err
	}
	return text, true, nil
}

func requiredBool(cols, vals []string, name string) (bool, error) {
	text, ok, isNull, err := columnString(cols, vals, name)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, fmt.Errorf("missing column %s", name)
	}
	if isNull {
		return false, fmt.Errorf("column %s is null", name)
	}
	switch strings.ToLower(text) {
	case "t", "true":
		return true, nil
	case "f", "false":
		return false, nil
	default:
		return false, fmt.Errorf("column %s value %q", name, text)
	}
}

func columnString(cols, vals []string, name string) (string, bool, bool, error) {
	idx := -1
	for i, col := range cols {
		if col == name {
			idx = i
			break
		}
	}
	if idx < 0 {
		return "", false, false, nil
	}
	if idx >= len(vals) {
		return "", false, false, fmt.Errorf("column %s has no value", name)
	}
	text, isNull, err := sqlLiteralString(vals[idx])
	if err != nil {
		return "", false, false, err
	}
	return text, true, isNull, nil
}

func sqlLiteralString(token string) (string, bool, error) {
	token = stripCast(strings.TrimSpace(token))
	if strings.EqualFold(token, "NULL") {
		return "", true, nil
	}
	if token == "" || token[0] != '\'' {
		return token, false, nil
	}
	if len(token) < 2 || token[len(token)-1] != '\'' {
		return "", false, fmt.Errorf("bad string %q", token)
	}
	return strings.ReplaceAll(token[1:len(token)-1], "''", "'"), false, nil
}

func stripCast(token string) string {
	inString := false
	castAt := -1
	for i := 0; i < len(token); i++ {
		if token[i] == '\'' {
			if inString && i+1 < len(token) && token[i+1] == '\'' {
				i++
				continue
			}
			inString = !inString
			continue
		}
		if !inString && token[i] == ':' && i+1 < len(token) && token[i+1] == ':' {
			castAt = i
			break
		}
	}
	if castAt < 0 {
		return token
	}
	return strings.TrimSpace(token[:castAt])
}
