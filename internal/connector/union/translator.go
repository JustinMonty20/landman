package union

import (
	"fmt"

	"github.com/JustinMonty20/landman/internal/connector"
	"github.com/JustinMonty20/landman/internal/models"
)

// UnionCountyTranslator converts Union County's raw GIS data into normalized Parcel objects
//
// This translator knows Union County's specific field names and data structures:
// - Union County uses "PIN" for parcel ID
// - Field names in their ArcGIS: "OWNER_NAME", "OWNER_ADDR", etc.
//
// Other counties would have different translators:
// - Mecklenburg might use "PARCEL_NUMBER" instead of "PIN"
// - Wake County might use "REID" for parcel ID
// - Each county has different field names for owner, address, tax info, etc.
type UnionCountyTranslator struct {
	priority int
}

// NewUnionCountyTranslator creates a new Union County translator
func NewUnionCountyTranslator() *UnionCountyTranslator {
	return &UnionCountyTranslator{
		priority: 10, // High priority since it's county-specific
	}
}

// CanTranslate checks if this translator can handle the given source
func (t *UnionCountyTranslator) CanTranslate(sourceName string) bool {
	// This translator only handles Union County GIS data
	// If we add UnionCountyTaxSource later, we might accept that too
	return sourceName == "union_county_gis"
}

// Priority returns the priority of this translator
func (t *UnionCountyTranslator) Priority() int {
	return t.priority
}

// Translate converts a raw record from Union County into a normalized Parcel
//
// NOTE: The current Parcel model is minimal (ParcelId, CountyId, changelog).
// When the Parcel model is expanded with fields like:
// - OwnerName
// - PropertyAddress
// - AssessedValue
// - LotSizeAcres
// - Zoning
// - TaxDelinquent
// - etc.
//
// This translator will extract those fields from Union County's specific field names:
// - raw["attributes"]["OWNER_NAME"] -> Parcel.OwnerName
// - raw["attributes"]["SITE_ADDR"] -> Parcel.PropertyAddress
// - raw["attributes"]["TOTAL_VALUE"] -> Parcel.AssessedValue
// - raw["attributes"]["CALC_ACRES"] -> Parcel.LotSizeAcres
// - raw["attributes"]["ZONING"] -> Parcel.Zoning
// - etc.
func (t *UnionCountyTranslator) Translate(raw connector.RawRecord) (*models.Parcel, error) {
	// Validate source
	if !t.CanTranslate(raw.SourceName) {
		return nil, fmt.Errorf("cannot translate source: %s", raw.SourceName)
	}

	// Extract parcel ID
	// Union County uses "PIN" field in the attributes
	parcelID := raw.ParcelID
	if parcelID == "" {
		return nil, fmt.Errorf("missing parcel ID in raw record")
	}

	// Create normalized parcel
	// For now, just the basic fields. When Parcel model expands, add more field extraction here.
	parcel := &models.Parcel{
		ParcelId: parcelID,
		CountyId: "union",
		// When we add more fields to Parcel model, extract them here:
		// OwnerName: extractString(raw.RawData, "attributes.OWNER_NAME"),
		// PropertyAddress: extractString(raw.RawData, "attributes.SITE_ADDR"),
		// AssessedValue: extractFloat(raw.RawData, "attributes.TOTAL_VALUE"),
		// etc.
	}

	return parcel, nil
}

// Helper functions for extracting fields from raw data
// These will be useful when we expand the Parcel model:

// extractString safely extracts a string field from nested map
// Example: extractString(raw, "attributes.OWNER_NAME")
// func extractString(data map[string]interface{}, path string) string {
// 	// Implementation using reflection or gjson
// 	return ""
// }

// extractFloat safely extracts a float field
// func extractFloat(data map[string]interface{}, path string) float64 {
// 	// Implementation
// 	return 0.0
// }

// extractInt safely extracts an int field
// func extractInt(data map[string]interface{}, path string) int64 {
// 	// Implementation
// 	return 0
// }

// extractBool safely extracts a bool field
// func extractBool(data map[string]interface{}, path string) bool {
// 	// Implementation
// 	return false
// }

// extractDate safely extracts and parses a date field
// func extractDate(data map[string]interface{}, path string) *time.Time {
// 	// Implementation
// 	return nil
// }
