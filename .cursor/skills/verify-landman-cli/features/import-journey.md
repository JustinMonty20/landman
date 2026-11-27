# Import journey

One pass from an empty verify database to a classified import. Per-step commands live in the linked files. Baseline preconditions in [the feature map](README.md) apply.

## Reset through classify

1. Start `landman-verify-postgis` and wait until it is healthy. [Bootstrap the verify database](local-database-bootstrap.md).
2. Run `verify_make db-drop`, `verify_make db-create`, and `verify_make migrate-up` with the verify overrides. The goose version integer matches the highest prefix in `db/migrations/`.
3. Run `check-schema`. Stdout is `schema: ok`. If it is not, stop and do not load the seed.
4. Load `seed.sql`. Run `check-loaded`. Stdout prints the census and then `loaded: ok`. The census names `07102010` and `07102051`.
5. Run the database doctor, then [check migration status](migration-status.md). The version and the status listing agree.
6. Run the CLI doctor. Require an executable binary and a compatible Go version. Obtain live-source approval before the next step.
7. Run `probe`. Require HTTP 200 for layer 215 and layer 197 metadata. Exit code 0. The evidence directory contains `page-timings.json`. Its single-page timings are diagnostic, not full-county completeness proof.
8. Run `./landman run union.gis.soil`, then `./landman run union.gis.parcels`, with `HOST_DATABASE_URL`. Do not pass `import`. That argument exits 1 with usage before a job starts. [Import parcels and soil](import-parcels-and-soil.md).
9. Run `classify`. Stdout is the verdict line. The evidence directory contains `drive-outcome.json`.

On `pass`, both seed parcel ids are still present. The county parcel count is greater than the seed parcel count. Soil polygons and overlaps for `union` and `union_county_gis` are greater than the seed insert counts. The job outcomes for `union.gis.parcels`, `union.gis.soil`, and `union.soil.screening` are `success`, and each of those attempts is `full_scan`. `soil_screening_heads` has the Union row. `seededSales` and `fullImport` are `met`. The parcel command prints job and fetch progress on stderr and no stdout. Job and stage logs can include screening. `classify` checks stored soil screening and soil assessments, not qualification assessment. This journey does not run `reprocess`.

Cleanup removes `landman-verify-postgis` and `landman-verify-data` after the evidence files are on disk. It does not remove `artifacts/verify/`.
