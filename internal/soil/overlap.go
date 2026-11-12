package soil

// Overlap links stored polygon and parcel UUIDs by their shared area in acres.
type Overlap struct {
	SoilPolygonID string
	ParcelID      string
	OverlapAcres  float64
}
