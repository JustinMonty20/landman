package soil

// MapUnit identifies a national soil map unit and its optional descriptive names.
type MapUnit struct {
	MapUnitKey    string
	MapUnitSymbol string
	AreaSymbol    string
	MapUnitName   *string
	AreaName      *string
}
