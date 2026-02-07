package merger

import (
	"fmt"

	"github.com/JustinMonty20/landman/internal/models"
)

// UnionCountyMerger combines data from multiple Union County sources into a single Parcel
//
// Current sources:
// - GIS (geometry, basic parcel info)
//
// Future sources to add:
// - Tax Assessor (owner info, tax status, assessed values)
// - Deeds Registry (sale history, legal ownership)
//
// Merge strategy for Union County:
// When we have multiple sources, the merger will use this authority hierarchy:
// 1. GIS data is authoritative for:
//    - Geometry/boundaries (most accurate spatial data)
//    - ParcelId (official PIN)
//    - Lot size/acreage
//
// 2. Tax Assessor data is authoritative for:
//    - Owner name and mailing address (most current)
//    - Assessed values (property tax records)
//    - Tax delinquent status
//    - Tax amount owed
//
// 3. Deeds Registry data is authoritative for:
//    - Legal owner name (official record)
//    - Last sale date and price
//    - Property transfer history
//
// Other counties might have different priorities:
// - Mecklenburg might trust their Tax data more for addresses
// - Wake County might have more frequently updated GIS data
type UnionCountyMerger struct{}

// NewUnionCountyMerger creates a new merger for Union County
func NewUnionCountyMerger() *UnionCountyMerger {
	return &UnionCountyMerger{}
}

// Merge combines multiple parcel records from different sources
//
// Current implementation: Since we only have GIS data, just return the single parcel.
//
// Future implementation: When we add Tax and Deeds sources, this will:
// 1. Start with GIS parcel as base (has geometry)
// 2. Overlay Tax data (owner, values, tax status)
// 3. Overlay Deeds data (sale history)
// 4. Resolve conflicts using authority hierarchy above
func (m *UnionCountyMerger) Merge(parcels []*models.Parcel) (*models.Parcel, error) {
	if len(parcels) == 0 {
		return nil, fmt.Errorf("no parcels to merge")
	}

	// Currently we only have one source (GIS), so just return it
	if len(parcels) == 1 {
		return parcels[0], nil
	}

	// When we have multiple sources, implement merge logic here:
	//
	// merged := &models.Parcel{}
	//
	// // Find the GIS parcel (use as base)
	// var gisParcel *models.Parcel
	// for _, p := range parcels {
	// 	if p.SourceType == "gis" {
	// 		gisParcel = p
	// 		break
	// 	}
	// }
	//
	// if gisParcel == nil {
	// 	return nil, fmt.Errorf("no GIS parcel found for merge")
	// }
	//
	// // Start with GIS data
	// merged.ParcelId = gisParcel.ParcelId
	// merged.CountyId = gisParcel.CountyId
	// merged.Geometry = gisParcel.Geometry
	// merged.LotSizeAcres = gisParcel.LotSizeAcres
	//
	// // Overlay Tax data (if present)
	// for _, p := range parcels {
	// 	if p.SourceType == "tax" {
	// 		merged.OwnerName = p.OwnerName  // Tax is authoritative for owner
	// 		merged.OwnerMailingAddress = p.OwnerMailingAddress
	// 		merged.AssessedValue = p.AssessedValue
	// 		merged.TaxDelinquent = p.TaxDelinquent
	// 		merged.TaxAmountOwed = p.TaxAmountOwed
	// 	}
	// }
	//
	// // Overlay Deeds data (if present)
	// for _, p := range parcels {
	// 	if p.SourceType == "deeds" {
	// 		merged.LastSaleDate = p.LastSaleDate
	// 		merged.LastSalePrice = p.LastSalePrice
	// 	}
	// }
	//
	// return merged, nil

	// For now, just return the first parcel
	return parcels[0], nil
}
