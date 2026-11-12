# Soil polygon–parcel join

## Scope

**S1.** After one layer-197 fetch on the existing Union GIS client, import parcel pages through the existing writer and preserve its `[]string` ID hook. A failed soil fetch or parcel import stops before the soil join.

**S2.** After parcel persistence succeeds, insert distinct map-unit stubs and enrich missing names with one batched Soil Data Access request independent of the GIS limiter. Preserve stubs and prior survey dates when optional enrichment fails; retry null names on the next import. Use the NC179 survey calendar date when returned.

**S3.** In a single soil transaction upsert map units and polygons on their natural keys, omitting UUIDs, then execute one set-based PostGIS intersection over all stored parcel shapes of the county and source. Measure positive intersection area in EPSG:2264 square US survey feet divided by 43,560; exclude edge-only contact but retain slivers. Never filter on vacancy or delete retired records. Roll back on error or cancellation.

## Acceptance and verification

**A1.** `go test -race ./internal/soil -count=1 -run 'TestSoilJoin|TestSoilNames'`, `go test -race ./... -count=1`, `go vet ./...`, and `go build ./...` pass. The SQL contract tests assert one `INSERT ... SELECT`, state-plane transform, and no ID-list/vacancy filter.

**A2.** On migrated disposable PostGIS, insert a 200-foot square, a west half, a 1-foot sliver, and an edge touch. Check acres near 0.459137 and 0.004591, two stored parcels including a house, stable UUIDs after repeat, and no removal of retired rows. Empty parcel tables yield zero joins. Cancellation and failing-name service do not publish partial soil changes.

**A3.** Record fixture statement elapsed time and execution count; the half-square join must finish under 30 seconds and execute once. For a county-sized run, record request count (at most seven soil GIS requests for 11,250 polygons at 2,000/page), join row count, and join elapsed time against the 490-second budget; do not add concurrency without that measurement.

## Exclusions

**E1.** No parcel writer, Union connector, scraper, migration, map UI, suitability screening, filtering by vacancy, polygon deletion, or worker pool changes. Soil use-specific screening remains the separate soil-5 milestone; geometry alone does not complete the soil-layer release.
