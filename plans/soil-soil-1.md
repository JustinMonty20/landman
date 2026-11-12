# Shared soil types and Union layer 197 translation

## Why
Store soil polygons independently of parcels so later ingestion can retain polygon identity and calculate parcel overlaps. This unit defines the shared shapes and translates one Union County feature; it does not fetch pages or write tables.

## Files
Create `internal/soil/polygon.go`, `internal/soil/mapunit.go`, `internal/soil/overlap.go`, `internal/connector/union/soil_translate.go`, `internal/connector/union/soil_translate_test.go`, and `internal/connector/union/testdata/soil/layer197-feature.json`. No other application files change.

## Contract
- `Polygon` uses `county.ID`, source name, county object ID as text, map-unit key, optional spatial version and survey date, and geometry at SRID 3857. Database UUID belongs to storage, not this struct.
- `MapUnit` carries key, symbol, area symbol, and optional names. `Overlap` carries polygon UUID, parcel UUID, and acres only.
- `ParseSoilFeature` maps `OBJECTID_1`, `MUKEY`, `MUSYM`, `AREASYMBOL`, and `SPATIALVER` from the live layer 197 response. It sets county to `county.Union` and source to `union_county_gis`, leaves survey date unset, and ignores `HydrcRatng`. Missing required keys or rings fail. An empty feature collection is valid and yields empty results.
- No parcel insertion, HTTP client, GIS fetch loop, migration, join, screening, or map UI belongs to this unit.

## Verification
- Capture the ChA `OBJECTID_1=1` response directly from live MapServer layer 197 if reachable. Confirm key `115341`, symbol `ChA`, area `NC179`, version 5, and polygon rings.
- Run `go test -race ./internal/soil/ ./internal/connector/union/ -count=1 -run TestParseSoilFeature` and `go test -race ./... -count=1`. Cover the live ChA fixture, absent `MUKEY` and `MUSYM`, empty features, empty rings, ArcGIS error, null `OBJECTID` alongside `OBJECTID_1`, unknown attributes, and repeat parsing.
- Compare trunk and head package test wall time with trunk, head, trunk runs. Reject head duration at or above 60 seconds or any test-time dial to `atlas.unioncountync.gov`; fixture reads must work offline.
- Review the exact file boundary and leave independent live and performance review to the program verifier before merge. This PR remains unmerged.
