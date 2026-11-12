# Soil-3: shared soil tables

## Scope and source
Implement the shared storage schema from [soil-layer.md](soil-layer.md#add-the-shared-soil-tables-soil-3) in `db/migrations/000009_soil.sql`. This migration follows the existing parcel schema and county restriction; it adds national map units, county source polygons, and polygon-to-parcel overlaps without changing parcel rows.

## Acceptance checks
- Goose up creates all three tables, the three specified indexes, UUID defaults, unique source and pair keys, Union county and EPSG:3857 shape checks, positive numeric overlap acres, and cascading join foreign keys.
- A repeated source polygon or polygon/parcel upsert can preserve its UUID; distinct polygons sharing a map-unit key remain distinct.
- Zero-acre overlaps and incorrect county are rejected; deleting either referenced row removes only the join row.
- Soil polygon `shape` uses unconstrained `GEOMETRY(Geometry)` intentionally: PostGIS coerces SRID 0 to 3857 when the column has a 3857 typmod, before a CHECK can inspect it. The `soil_polygons_srid_check` validates the original input SRID instead. Insert shapes with SRID 0 and 4326 and confirm both are rejected; insert a shape with SRID 3857 and confirm it is accepted.
- Goose down removes the join table first, then polygons, then map units. Run required static searches, `go test -race ./...`, and, when PostGIS is available, migrate up/down and exercise a half-square overlap fixture in EPSG:2264.

## Exclusions
No ingest, map UI, parcel changes, screening, source extras, geometry simplification, or deletion of retired soil records. This PR does not merge; the operator lands it after soil-1 and persist-1.

## Verification
Record the commands and outcomes in the PR body, including any unavailable local PostGIS checks.
