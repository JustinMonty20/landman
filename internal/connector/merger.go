package connector

import (
	"github.com/JustinMonty20/landman/internal/models"
)

// Merger combines data from multiple sources into a single, complete parcel record
// Each county implements merge logic based on which sources are authoritative for which fields.
//
// Example: Union County might use this strategy:
// - GIS data is authoritative for geometry and parcel boundaries (most accurate spatial data)
// - Tax Assessor data is authoritative for assessed values and tax status (most current financial data)
// - Deeds data is authoritative for ownership and sale history (legal record of title)
//
// Mecklenburg County might have different priorities:
// - They might trust their Tax data more for property addresses
// - Their GIS might be updated less frequently than Union County
//
// The merger decides which source "wins" when data conflicts occur.
// This is county-specific because each county has different data quality and update frequencies.
type Merger interface {
	// Merge takes multiple partial parcel records and combines them into one
	// The records slice contains parcels from different sources for the SAME parcel ID
	//
	// Example input for parcel "06-123-456":
	// - parcels[0]: from GIS (has geometry, basic info)
	// - parcels[1]: from Tax Assessor (has owner, tax info, assessed value)
	// - parcels[2]: from Deeds (has sale history, legal owner name)
	//
	// Output: Single merged parcel with best data from each source
	Merge(parcels []*models.GISParcel) (*models.GISParcel, error)
}
