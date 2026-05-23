# Small normalization layer for county GIS parcels

## Context
- Goal: keep this scope very small by normalizing Union County GIS into the existing `models.GISParcel` standard format.
- Current code already has a `connector.Translator` interface and a Union County `GISTranslator`, but it only fills parcel identity fields.
- Existing standard target model is `internal/models/gis_parcel.go`.
- User confirmed the first pass should normalize everything available from Union County GIS, not just a subset.

## Approach
- Expand the existing Union County GIS translator rather than adding a new abstraction.
- Extract data from the existing `RawRecord.RawData["attributes"]` map produced by `GISSource.fetchBatch`; fall back to top-level raw data only for test ergonomics/backward compatibility.
- Map all Union County GIS fields that have a clear home into `models.GISParcel`: identity, owner, mailing/current address, situs address, legal, land, building, valuation, sales, geometry metadata, and source extras.
- Preserve modeled Union-specific leftovers in `SourceExtras.Union`; do not expand the standard parcel model unless a field is already in Union GIS but absent from both standard fields and extras.
- Keep this small by limiting work to Union County GIS translation and tests; do not add persistence, fetching, or cross-county abstractions.
- For geometry, parse ArcGIS polygon `geometry.rings` into a `go-geom` polygon where possible, set `Geometry.SRID`/`SourceSRID` from the ArcGIS service SRID (`3857` today), and retain source area/length from `SHAPE.STArea()` and `SHAPE.STLength()`.
- Add unit tests around the field mappings and error behavior.

## Field mapping
- Identity: `PID -> SourceParcelID/ParcelNumber`, `ACCTNO -> AccountNumber`, `parcel_year -> ParcelYear`, constants `county_id/county_code = union`, `source_name = union_county_gis`.
- Owner: `CURR_NAME1/2 -> CurrentName1/2`, `JAN1_NAME1/2 -> TaxName1/2`, `JAN1_OWNERID/2 -> TaxOwnerID1/2`.
- Mailing/current address: `CURR_ADDR1/2`, `CURR_CITY`, `CURR_STATE`, `CURR_ZIPCODE`.
- Situs address: `PHYSSTRADD` as `Line1`.
- Legal: `LEGDESC_1`, `PLAT_BOOK`, `PLAT_PAGE`, and first sale deed book/page as current deed reference if present.
- Land: `LAND_CODE`, `LAND_TYPE`, `property_use`, `subdivision`, `mapped_acres`, `gross_acres`, `VAL_AC`, `VAL_SQFT`.
- Building: `YEARBLT`, `SQFT`, `BASEMENT`, `BLDQUAL_CODE`, `STRUCTTYPE`, `STRUCTSTYLE`.
- Valuation: `FMV_LAND -> Land`, `FMV_IMPRV -> Improvement`, `FMV_TOTAL -> Total`, `TOTVAL -> Assessed`.
- Sales: `s1_`, `s2_`, and `s3_` fields to three `ParcelSale` entries, preserving sale date millis as `OGSaleDate` and parsing to `SaleDate` when present.
- Source extras: `OBJECTID`, `OBJECTID_1`, district/township/description/neighborhood/multi-land fields, `idcol`, and `DATE_CRT`/original milliseconds.

## Files to modify
- `internal/connector/union/translator.go`
- `internal/connector/union/translator_test.go`

## Reuse
- `connector.Translator` in `internal/connector/translator.go`.
- `connector.RawRecord` in `internal/connector/datasource.go`.
- `GISSource.fetchBatch` in `internal/connector/union/gis.go`, which already stores ArcGIS `attributes` and `geometry` in `RawData`.
- `models.GISParcel` and nested parcel structs in `internal/models/gis_parcel.go`.
- `models.UnionGISParcel` as a field inventory/reference in `internal/models/union_gis_parcel.go`.
- `github.com/twpayne/go-geom` already exists in `go.mod` and can represent parsed parcel polygons.

## Steps
- [x] Confirm the first county/source: Union County GIS.
- [x] Confirm the first-pass field set: normalize everything available from Union County GIS.
- [x] Add small typed extraction helpers inside `internal/connector/union/translator.go` for strings, ints, floats, ArcGIS millisecond dates, attributes map access, and geometry rings.
- [x] Populate every `GISParcel` section listed in the field mapping.
- [x] Preserve Union-specific leftover fields in `SourceExtras.Union`.
- [x] Add table-driven translator tests for populated fields, sales/date parsing, geometry metadata/parsing, missing parcel ID, and wrong source.

## Verification
- Run `go test ./internal/connector/union`.
- Run `go test ./...` if the small change remains quick.
