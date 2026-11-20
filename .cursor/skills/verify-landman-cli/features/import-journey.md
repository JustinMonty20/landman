# Import journey

One pass from an empty verify database to a classified import. Per-step commands live in the linked files. Baseline preconditions in [the feature map](README.md) apply.

## Sub-features

- `journey-reset` starts the verify container and applies migrations.
- `journey-seed` loads the captured Union sample and checks it.
- `journey-import` runs soil, then parcels, through `run-gis.sh`.
- `journey-classify` reads the stored publication and the sale slots.

## How to get to it (user POV)

- From the repository root, follow [Bootstrap the verify database](local-database-bootstrap.md), then [Import parcels and soil](import-parcels-and-soil.md).
- The runner is `.cursor/skills/verify-landman-cli/run-gis.sh`. It runs `./landman run union.gis.soil` and then `./landman run union.gis.parcels`.

## Driving it with make and the landman binary

Preconditions:

- The verify container, goose URL, and host URL come from `live-gis.md`.
- `./landman` exists after `make build`.

- Start `landman-verify-postgis` and wait until it is healthy.
- Run `make db-drop`, `make db-create`, and `make migrate-up` with the verify overrides. The goose version integer matches the highest prefix in `db/migrations/`.
- Run `check-schema`. Stdout is `schema: ok`. If it is not, stop and do not load the seed.
- Load `seed.sql`. Run `check-loaded`. Stdout prints the census and then `loaded: ok`. The census names `07102010` and `07102051`.
- Run the database doctor, then [check migration status](migration-status.md). The version and the status listing agree, and the status listing has no `Pending` row.
- Run the CLI doctor. Layer 215 and layer 197 both return HTTP 200. Those curls check reachability. They are not the page timings.
- Run `probe`. Exit code 0. The evidence directory contains `page-timings.json`.
- Run the soil command, then the parcel command, with `HOST_DATABASE_URL`. Do not pass `import`. That argument exits 1 with usage before a job starts.
- Run `classify`. Stdout is the verdict line. The evidence directory contains `drive-outcome.json`.

## Gotchas

- On `pass`, both seed parcel ids are still present. The county parcel count is greater than the seed parcel count. Soil polygons and overlaps for `union` and `union_county_gis` are greater than the seed insert counts. The job outcomes for `union.gis.parcels`, `union.gis.soil`, and `union.soil.screening` are `success`, and each of those attempts is `full_scan`. `soil_screening_heads` has the Union row. `seededSales` and `fullImport` are `met`.
- Stdout stays empty. Parcel stderr contains `Parcel capture`, `Parcel capture page`, and `Parcel capture complete`. Soil, screening, and qualification each print `Started`, `Running`, and `Finished` lines. `classify` proves the screening job, the screening head, and one soil assessment per county parcel on that head. It does not read `union.qualification.assess`.
- This journey does not run `reprocess`.
- Cleanup removes `landman-verify-postgis` and `landman-verify-data` after the evidence files are on disk. It does not remove `artifacts/verify/`.
