# Land Parcel ROI Analysis Platform - Go Service Specification

## Project Overview

A standalone Go service that identifies and analyzes high ROI land investment opportunities across multiple counties in North Carolina. The service fetches data from multiple county sources (GIS, tax assessor, deeds), normalizes it, tracks changes over time, and exposes the data via REST API.

**This is the backend service only** - frontend will be a separate repository/service.

**Target User**: Real estate investor looking for undervalued land parcels based on specific criteria (absentee owners, tax delinquent properties, zoning opportunities, etc.)

**Initial Target**: Union County, NC  
**Future Scale**: Multiple NC counties (Mecklenburg, etc.)

---

## Tech Stack

- **Language**: Go (Golang)
- **Database**: PostgreSQL with PostGIS extension
- **API**: REST API (returns JSON and GeoJSON)
- **Scheduler**: cron for periodic data imports
- **Architecture Pattern**: Plugin-based connector system for county-specific data sources

---

## Core Architecture Principles

### 1. Plugin-Based County Connectors
Each county has its own self-contained package that implements standard interfaces. Adding a new county requires NO changes to core code - just implement the interfaces and register via `init()`.

### 2. Multi-Source Data Aggregation
Each county may have 2-5 different data sources (GIS API, Tax Assessor, Deeds Registry, etc.). The system:
- Fetches from all sources
- Cleans/normalizes each source payload (flattening, parsing, and removing noise)
- Translates each to normalized format
- Merges data intelligently based on source authority
- Tracks which sources contributed to each parcel

### 3. Ingestion Pipeline (Per County)
1. Fetch raw data from each source (GIS, Tax Assessor, Deeds, etc.).
2. Clean and normalize per-source (flatten/parse into typed structures).
3. Translate per-source into the common domain model.
4. Merge across sources by parcel ID to produce a single parcel view.

### 4. Change Detection & Historical Tracking
- Generate SHA-256 hash of key parcel fields
- Store hash in database
- On each import (bi-weekly), compare new hash to stored hash
- Track what changed, when, and by how much
- Maintain audit trail of significant changes

### 5. Scheduled Imports
- Run imports 2x per week (e.g., Sunday & Wednesday at 2 AM)
- Each import creates a job record with statistics
- Track: records processed, new, changed, unchanged, errors

---

## Project Directory Structure

```
project-root/
├── cmd/
│   ├── api/                    # REST API server
│   │   └── main.go
│   ├── importer/               # CLI tool for manual data imports
│   │   └── main.go
│   └── scheduler/              # Cron scheduler for periodic imports
│       └── main.go
│
├── internal/
│   ├── models/                 # Domain models
│   │   ├── parcel.go
│   │   ├── parcel_change.go
│   │   └── import_job.go
│   │
│   ├── repository/             # Data access layer
│   │   ├── parcel_repository.go
│   │   ├── parcel_change_repository.go
│   │   └── import_job_repository.go
│   │
│   ├── connector/              # Core connector interfaces and registry
│   │   ├── interface.go        # DataSource, Translator, Merger interfaces
│   │   ├── registry.go         # Global registry for connectors
│   │   │
│   │   ├── union/              # Union County connectors (example)
│   │   │   ├── exports.go       # Stable API surface for other packages
│   │   │   ├── register.go
│   │   │   ├── gis/
│   │   │   │   ├── gis_source.go
│   │   │   │   └── gis_source_test.go
│   │   │   ├── tax/
│   │   │   │   ├── tax.go
│   │   │   │   ├── tax_flatten.go
│   │   │   │   └── tax_test.go
│   │   │   ├── translator/
│   │   │   │   ├── translator.go
│   │   │   │   └── translator_test.go
│   │   │   ├── merger/
│   │   │   │   ├── merger.go
│   │   │   │   └── merger_test.go
│   │   ├── shared/
│   │   │   └── httpclient/
│   │   │       ├── http_client.go
│   │   │       └── http_client_test.go
│   │   │
│   │   └── mecklenburg/        # Future county (example structure)
│   │       ├── config.go
│   │       ├── gis_source.go
│   │       ├── tax_source.go
│   │       ├── translator.go
│   │       ├── merger.go
│   │       └── register.go
│   │
│   ├── service/                # Business logic
│   │   ├── import_service.go   # Orchestrates import process
│   │   └── parcel_service.go   # Parcel-related business logic
│   │
│   ├── handler/                # HTTP handlers
│   │   ├── parcel_handler.go
│   │   ├── change_handler.go
│   │   └── import_handler.go
│   │
│   └── middleware/             # HTTP middleware
│       ├── logging.go
│       └── cors.go
│
├── pkg/                        # Reusable packages
│   ├── hasher/                 # Hash generation for change detection
│   │   └── parcel_hasher.go
│   │
│   ├── geojson/                # GeoJSON utilities
│   │   └── converter.go
│   │
│   └── calculator/             # ROI calculations
│       └── roi.go
│
├── migrations/                 # Database migrations
│   ├── 001_create_parcels.sql
│   ├── 002_create_parcel_changes.sql
│   └── 003_create_import_jobs.sql
│
├── docs/                       # Documentation
│   └── ARCHITECTURE.md         # This file
│
├── config/                     # Configuration files
│   ├── config.yaml
│   └── config.example.yaml
│
├── .env.example
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

---

## Core Interfaces

### 1. DataSource Interface

Every county data source (GIS, Tax, Deeds) implements this interface:

```go
// internal/connector/interface.go
package connector

import (
    "context"
    "time"
)

// DataSource represents a single source of data (GIS, Tax Assessor, etc.)
type DataSource interface {
    // Name returns unique identifier for this source (e.g., "union_county_gis")
    Name() string
    
    // County returns which county this source serves
    County() string
    
    // SourceType returns the type of data (e.g., "gis", "tax", "deeds")
    SourceType() string
    
    // Fetch retrieves raw data from the source
    Fetch(ctx context.Context) ([]RawRecord, error)
    
    // SupportsIncremental indicates if source supports fetching only changed records
    SupportsIncremental() bool
    
    // FetchSince retrieves records changed since given time (if supported)
    FetchSince(ctx context.Context, since time.Time) ([]RawRecord, error)
}

// RawRecord is the standardized container for raw data from any source
type RawRecord struct {
    SourceName  string                 // Which source it came from
    ParcelID    string                 // Best effort at extracting parcel ID
    FetchedAt   time.Time             
    RawData     map[string]interface{} // The actual raw data
}
```

### 2. Translator Interface

Converts raw data from a specific county/source into normalized parcel format:

```go
// Translator converts raw records into normalized parcel data
type Translator interface {
    // Translate converts a raw record into normalized parcel
    Translate(raw RawRecord) (*models.Parcel, error)
    
    // CanTranslate checks if this translator can handle the given source
    CanTranslate(sourceName string) bool
    
    // Priority returns priority (higher = preferred) when multiple translators match
    Priority() int
}
```

### 3. Merger Interface

Combines data from multiple sources into a single, complete parcel record:

```go
// Merger combines data from multiple sources into a single parcel record
type Merger interface {
    // Merge takes multiple partial parcel records and combines them
    Merge(parcels []*models.Parcel) (*models.Parcel, error)
}
```

---

## Hash Generation for Change Detection

### Purpose
Generate a deterministic hash of key parcel fields to detect changes between imports without storing full historical records.

### Implementation

```go
// pkg/hasher/parcel_hasher.go
package hasher

import (
    "crypto/sha256"
    "encoding/json"
    "fmt"
    "math"
    "strings"
    "time"
)

// HashableParcelData - only the fields we care about for change detection
type HashableParcelData struct {
    OwnerName            string
    OwnerMailingAddress  string
    PropertyAddress      string
    AssessedValue        float64
    LotSizeAcres         float64
    Zoning               string
    TaxDelinquent        bool
    TaxAmountOwed        float64
    LastSaleDate         *time.Time
    LastSalePrice        float64
}

func GenerateParcelHash(data HashableParcelData) (string, error) {
    // Normalize data for consistent hashing
    normalized := map[string]interface{}{
        "owner_name":             normalizeString(data.OwnerName),
        "owner_mailing_address":  normalizeString(data.OwnerMailingAddress),
        "property_address":       normalizeString(data.PropertyAddress),
        "assessed_value":         roundToTwoDecimals(data.AssessedValue),
        "lot_size_acres":         roundToFourDecimals(data.LotSizeAcres),
        "zoning":                 normalizeString(data.Zoning),
        "tax_delinquent":         data.TaxDelinquent,
        "tax_amount_owed":        roundToTwoDecimals(data.TaxAmountOwed),
        "last_sale_date":         formatDate(data.LastSaleDate),
        "last_sale_price":        roundToTwoDecimals(data.LastSalePrice),
    }
    
    // Convert to JSON for consistent serialization
    jsonBytes, err := json.Marshal(normalized)
    if err != nil {
        return "", err
    }
    
    // Generate SHA-256 hash
    hash := sha256.Sum256(jsonBytes)
    return fmt.Sprintf("%x", hash), nil
}

func normalizeString(s string) string {
    return strings.TrimSpace(strings.ToLower(s))
}

func roundToTwoDecimals(f float64) float64 {
    return math.Round(f*100) / 100
}

func roundToFourDecimals(f float64) float64 {
    return math.Round(f*10000) / 10000
}

func formatDate(t *time.Time) string {
    if t == nil {
        return ""
    }
    return t.Format("2006-01-02")
}
```

### Key Fields for Hash (DO NOT include geometry in hash)
- owner_name
- owner_mailing_address
- property_address
- assessed_value
- lot_size_acres
- zoning
- tax_delinquent
- tax_amount_owed
- last_sale_date
- last_sale_price

**Important**: Geometry/coordinates are NOT included in the hash because they rarely change and GIS coordinate precision can vary between fetches.

---

## Import Service Flow

### High-Level Process

1. **Fetch Phase**
   - Get all registered DataSources for the county
   - Fetch raw data from each source
   - Group raw records by parcel_id

2. **Translation Phase**
   - For each raw record, find appropriate Translator
   - Translate raw data to normalized Parcel model
   - Collect all translated parcels for each parcel_id

3. **Merge Phase**
   - Use county-specific Merger to combine data from multiple sources
   - Apply merge priority rules (e.g., GIS authoritative for geometry, Tax authoritative for tax info)

4. **Change Detection Phase**
   - Generate hash of key fields
   - Compare to existing hash in database
   - Track what changed, update change_count

5. **Persistence Phase**
   - Upsert parcel (insert if new, update if changed)
   - Create parcel_change records for significant changes
   - Update import_job statistics


---

## API Endpoints (High-Level)

The service exposes a REST API that returns JSON and GeoJSON for consumption by any frontend.

### Parcel Queries
- `GET /api/parcels` - List parcels with filters (JSON)
  - Query params: `county`, `tax_delinquent`, `min_acres`, `max_acres`, `min_price_per_acre`, `max_price_per_acre`, `zoning`, etc.
- `GET /api/parcels/:id` - Get single parcel details (JSON)
- `GET /api/parcels/geojson` - Get parcels as GeoJSON FeatureCollection (for mapping)
  - Supports same filters as `/api/parcels`
  - Returns geometry data for rendering on maps

### Change Tracking
- `GET /api/parcels/changes` - Recent changes across all parcels
  - Query params: `since`, `change_type`, `county`
- `GET /api/parcels/:id/changes` - Change history for specific parcel

### Import Jobs
- `GET /api/imports/history` - List import job history
- `GET /api/imports/:id` - Get import job details
- `POST /api/imports/trigger` - Manually trigger import (admin only)
  - Body: `{"county": "union"}`

### Analytics
- `GET /api/analytics/roi-leaders` - Top ROI parcels
  - Query params: `county`, `limit`
- `GET /api/analytics/recent-delinquencies` - Newly delinquent parcels
  - Query params: `county`, `days`

### Health & Monitoring
- `GET /health` - Health check endpoint
- `GET /metrics` - Prometheus metrics (optional)

---

## Implementation Checklist

### Phase 1: Core Interfaces
- [ ] Define all interfaces (DataSource, Translator, Merger, Registry)
- [ ] Implement global Registry
- [ ] Define RawRecord and Parcel models
- [ ] Define HashableParcelData and hasher interface

### Phase 2: Union County Connector (First Implementation)
- [ ] Implement UnionCountyGISSource
- [ ] Implement UnionCountyTaxSource
- [ ] Implement UnionCountyTranslator
- [ ] Implement UnionCountyMerger
- [ ] Create registration mechanism

### Phase 3: Import Service
- [ ] Build ImportService that orchestrates the flow
- [ ] Implement change detection logic using hasher
- [ ] Handle errors and logging
- [ ] Create import job tracking

### Phase 4: REST API
- [ ] Build HTTP server and routing
- [ ] Implement parcel query endpoints
- [ ] Implement GeoJSON endpoint
- [ ] Implement change tracking endpoints
- [ ] Implement analytics endpoints
- [ ] Add CORS middleware for frontend consumption

### Phase 5: Scheduler
- [ ] Set up cron scheduler for bi-weekly imports
- [ ] Add monitoring and alerting
- [ ] Implement retry logic

---

## Notes for Implementation

### When Adding a New County

1. Create new package: `internal/connector/{county_name}/`
2. Implement DataSource(s) for each data source
3. Implement Translator to normalize data
4. Implement Merger to combine sources
5. Create `register.go` with `init()` function
6. Import package in main: `import _ "project/internal/connector/{county_name}"`

### Testing Strategy

- **Unit tests**: Test each connector in isolation
- **Mock sources**: Create mock DataSources for testing translation/merge logic
- **Integration tests**: Test full import flow with test database
- **Snapshot tests**: Store known-good parcel data and compare
- **API tests**: Test endpoints with various query parameters

### Performance Considerations

- Use connection pooling for database
- Consider batching database operations (bulk inserts/updates)
- Add indexes on frequently queried fields (county, tax_delinquent, roi_score)
- Implement pagination for large result sets
- Cache GeoJSON responses (consider Redis)
- Use prepared statements for repeated queries

### Database Management

- Use migration tool (golang-migrate, goose, or similar)
- Keep migrations in version control
- Support up/down migrations
- Test migrations on dev/staging before production

---

## Example Usage (High-Level)

### Registering a County Connector

```go
// internal/connector/union/register.go
package union

import "project/internal/connector"

func init() {
    config := LoadConfig()
    
    // Register sources
    connector.RegisterSource(NewUnionCountyGISSource(config))
    connector.RegisterSource(NewUnionCountyTaxSource(config))
    
    // Register translator
    connector.RegisterTranslator("union", NewUnionCountyTranslator())
    
    // Register merger
    connector.RegisterMerger("union", NewUnionCountyMerger())
}
```

### Running an Import

```go
// cmd/importer/main.go
package main

import (
    _ "project/internal/connector/union"  // Auto-registers
)

func main() {
    ctx := context.Background()
    county := "union"
    
    err := importService.ImportCounty(ctx, county)
    if err != nil {
        log.Fatal(err)
    }
}
```

### Querying Parcels

```go
// Example API usage
GET /api/parcels?county=union&tax_delinquent=true&min_acres=5&max_price_per_acre=5000

Response: {
  "parcels": [
    {
      "id": 123,
      "parcel_id": "06-123-456",
      "county": "union",
      "owner_name": "John Doe",
      "assessed_value": 50000,
      "lot_size_acres": 10.5,
      "price_per_acre": 4761.90,
      "tax_delinquent": true,
      "is_absentee_owner": true,
      "roi_score": 8.5
    }
  ]
}
```

---

## Summary

This architecture provides a clean, scalable foundation for multi-county land parcel analysis as a **standalone Go service**. The plugin-based design means you can add new counties without touching core code, and the change detection system ensures you're always tracking what matters for ROI analysis.

The service exposes a REST API that can be consumed by any frontend (React, Vue, mobile apps, etc.) built in a separate repository. By keeping the backend as a standalone service, you maintain clear separation of concerns and can iterate on frontend and backend independently.

**Focus on getting the interfaces right first, then implement one county end-to-end as a reference implementation.**

### API Response Format Examples

**JSON Response (`/api/parcels`):**
```json
{
  "parcels": [
    {
      "id": 123,
      "parcel_id": "06-123-456",
      "county": "union",
      "owner_name": "John Doe",
      "owner_mailing_address": "123 Main St, Charlotte, NC",
      "property_address": "456 Land Rd, Monroe, NC",
      "assessed_value": 50000,
      "lot_size_acres": 10.5,
      "price_per_acre": 4761.90,
      "zoning": "RA",
      "tax_delinquent": true,
      "tax_amount_owed": 2500.00,
      "is_absentee_owner": true,
      "roi_score": 8.5,
      "last_changed_at": "2024-01-15T10:30:00Z"
    }
  ],
  "total": 45,
  "page": 1,
  "per_page": 20
}
```

**GeoJSON Response (`/api/parcels/geojson`):**
```json
{
  "type": "FeatureCollection",
  "features": [
    {
      "type": "Feature",
      "geometry": {
        "type": "Polygon",
        "coordinates": [[[-80.123, 34.456], ...]]
      },
      "properties": {
        "id": 123,
        "parcel_id": "06-123-456",
        "county": "union",
        "owner_name": "John Doe",
        "assessed_value": 50000,
        "lot_size_acres": 10.5,
        "price_per_acre": 4761.90,
        "tax_delinquent": true,
        "roi_score": 8.5
      }
    }
  ]
}
```
