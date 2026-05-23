package connector

import (
	"github.com/JustinMonty20/landman/internal/models"
)

// Translator converts raw records from a specific county/source into normalized parcel format
// Each county implements this to translate their specific data formats.
//
// Example: UnionCountyTranslator knows how to parse Union County's ArcGIS response
// and convert field names like "OWNER_NAME" -> Parcel owner fields (when expanded)
//
// Different counties have different field names and data structures:
// - Union County ArcGIS might use "PIN" for parcel ID
// - Mecklenburg County might use "PARCEL_NUMBER"
// - Wake County might use "REID"
//
// Each county's Translator handles these differences and produces normalized GISParcel objects.
type Translator interface {
	// Translate converts a raw record into normalized parcel
	Translate(raw RawRecord) (*models.GISParcel, error)

	// CanTranslate checks if this translator can handle the given source
	// Example: UnionCountyTranslator returns true for "union_county_gis"
	// but false for "mecklenburg_county_gis"
	CanTranslate(sourceName string) bool

	// Priority returns priority (higher = preferred) when multiple translators match
	// Useful if you have a generic translator and a specialized one
	// Example: A generic ArcGIS translator might have priority 1,
	// while Union County specific translator has priority 10
	Priority() int
}
