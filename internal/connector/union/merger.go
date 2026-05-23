package union

import (
	"fmt"

	"github.com/JustinMonty20/landman/internal/models"
)

// ParcelMerger combines data from multiple Union County sources into a single GISParcel.
//
// Merge strategy for Union County (when multiple sources are available):
// 1. GIS data is authoritative for geometry and parcel boundaries
// 2. Tax Assessor data is authoritative for assessed values and tax status
// 3. Deeds Registry data is authoritative for ownership and sale history
type ParcelMerger struct{}

// NewParcelMerger creates a new Union County merger.
func NewParcelMerger() *ParcelMerger {
	return &ParcelMerger{}
}

// Merge combines multiple partial parcel records into one complete record.
func (m *ParcelMerger) Merge(parcels []*models.GISParcel) (*models.GISParcel, error) {
	if len(parcels) == 0 {
		return nil, fmt.Errorf("no parcels to merge")
	}
	if len(parcels) == 1 {
		return parcels[0], nil
	}
	// When multiple sources are added, implement source-authority merge logic here.
	return parcels[0], nil
}
