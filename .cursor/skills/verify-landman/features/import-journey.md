# Import journey

One pass from an empty verify database to a committed import. Per-step commands live in the linked files. Baseline preconditions in [the feature map](README.md) apply.

## Reset, migrate, seed, import

1. Start `landman-verify-postgis` and wait until it is healthy. [Bootstrap the verify database](local-database-bootstrap.md).
2. `make db-drop`, `make db-create`, and `make migrate-up` with the verify overrides. The goose version integer matches the highest prefix in `db/migrations/`.
3. Load `seed.sql`. The sentinel parcel count is 1. County parcels and soil polygons are 0.
4. Run database doctor, then [check migration status](migration-status.md). The version and the status listing agree.
5. Run CLI doctor. Layer 215 and layer 197 both return HTTP 200.
6. Run `./landman` with `HOST_DATABASE_URL`. [Import parcels and soil](import-parcels-and-soil.md).
7. After exit 0, read the sentinel, the county parcels, the soil rows, `soil_parcel_assessments`, `job_attempts`, and `parcel_sales`.

The sentinel is still 1. County parcels, soil polygons, and soil-to-parcel rows are greater than 0. Both job outcomes are `success`. `parcel_sales` is 0. `soil_screening_heads` has the Union row. Screening and assessment do not print their own lines. The SQL read is the proof that they ran.

Cleanup removes `landman-verify-postgis` and `landman-verify-data` after the evidence files are on disk. It does not remove `artifacts/verify/`.
