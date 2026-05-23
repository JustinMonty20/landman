package connector

import (
	"context"
	"time"
)

// RawRecord is the standardized container for raw data from any source
// Each county's DataSource returns data in this format
type RawRecord struct {
	SourceName string                 // Which source it came from (e.g., "union_county_gis")
	ParcelID   string                 // Best effort at extracting parcel ID
	FetchedAt  time.Time              // When this record was fetched
	RawData    map[string]interface{} // The actual raw data from the source
}

// DataSource represents a single source of data (GIS, Tax Assessor, Deeds, etc.)
// Each county implements this interface for each of their data sources.
//
// Example: Union County might have:
// - UnionCountyGISSource (fetches from ArcGIS REST API)
// - UnionCountyTaxSource (fetches from tax assessor database)
// - UnionCountyDeedsSource (fetches from deeds registry)
//
// Other counties would implement their own versions with county-specific logic:
// - Mecklenburg County would have MecklenburgCountyGISSource with their API endpoints
// - Wake County would have WakeCountyGISSource with their specific parameters
type DataSource interface {
	// Name returns unique identifier for this source (e.g., "union_county_gis")
	Name() string

	// County returns which county this source serves (e.g., "union")
	County() string

	// SourceType returns the type of data (e.g., "gis", "tax", "deeds")
	SourceType() string

	// Fetch retrieves raw data from the source
	// Returns slice of RawRecord containing the raw data
	Fetch(ctx context.Context) ([]RawRecord, error)

	// SupportsIncremental indicates if source supports fetching only changed records
	// Most APIs don't support this initially, so return false
	SupportsIncremental() bool

	// FetchSince retrieves records changed since given time (if supported)
	// Only called if SupportsIncremental() returns true
	FetchSince(ctx context.Context, since time.Time) ([]RawRecord, error)
}
