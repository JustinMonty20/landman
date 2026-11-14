# Import parcels and soil

`./landman` reads `DATABASE_URL`, fetches Union County soil and parcels, upserts every parcel, stores soil, runs soil screening, and prints how many feature IDs looked vacant.

## Sub-features

- `import-build` produces the `./landman` binary.
- `import-run` writes parcels and soil, then runs screening inside one 10 minute deadline.
- `import-summary` prints the vacant feature-ID total after the parcel commit.
- `import-jobs` records `job_attempts` for `union.gis.parcels` and `union.soil.screening`.

## How to get to it (user POV)

- From the repo root, run `make build`, then run `./landman` with `DATABASE_URL` set to the verify database host URL in `SKILL.md`.
- `make run` is the operator shortcut. It loads `.env` when that file exists. Verification does not use it.

## Driving it with make and the landman binary

Preconditions:

- The verify database launch in `SKILL.md` has exited 0, including the seed load.
- Database doctor and CLI doctor have passed for this container.
- No other `./landman` process is running.
- `SOIL_SCREENING_MAX_AGE` is unset, or it is a valid positive Go duration. The default freshness interval is 30 days. An explicit run still refreshes USDA evidence.

- **Build.** Run `make build`. Exit code `0` and `./landman` exists.
- **Read the seed before the import.** Run `docker exec -i "$VERIFY_CONTAINER" psql "$GOOSE_DATABASE_URL" -v ON_ERROR_STOP=1 -c "SELECT count(*) AS sentinel FROM parcels WHERE source_name = 'verify-landman' AND source_parcel_id = 'verify-sentinel';" -c "SELECT count(*) AS county_parcels FROM parcels WHERE source_name = 'union_county_gis';" -c "SELECT count(*) AS soil_polygons FROM soil_polygons;"`. The sentinel count is `1`. The county parcel count is `0`. The soil polygon count is `0`.
- **Import.** Run `DATABASE_URL="$HOST_DATABASE_URL" ./landman`, capturing stdout, stderr, and the exit code under `artifacts/verify/$RUN_ID/`. The process exits 0.
- **See parcel progress.** Stderr contains `[union_county_gis] Total parcels to fetch: ` with a count greater than 0, then `[union_county_gis] Batch size: 2000, Total batches: `, then `[union_county_gis] Fetch complete:`. Those lines start after the soil fetch. The soil fetch itself prints nothing.
- **See the summary.** Stdout contains `Imported parcels, ` and ` likely-vacant feature IDs`. This line is printed before soil storage and screening finish. Keep the process until it exits.
- **Read the database after exit 0.** Using `docker exec` and `GOOSE_DATABASE_URL`, the sentinel count is still `1`. The `union_county_gis` parcel count is greater than 0. `soil_polygons` and `soil_polygon_parcels` for `county_id = 'union'` and `source_name = 'union_county_gis'` are greater than 0. `job_attempts` has outcome `success` for `union.gis.parcels` and for `union.soil.screening`. `soil_screening_heads` has one row for that county and source. `soil_parcel_assessments` has the same count as the imported county parcels. `parcel_sales` is still `0`.
- **Proof.** Keep `import.stdout.txt`, `import.stderr.txt`, `import.exit.txt`, the before SQL, and the after SQL. The exit file contains `0`. Stderr for that success does not contain `import parcels:`, `store soil geometry:`, `required soil screening:`, or `context deadline exceeded`.

## Gotchas

- This drive hits the live county ArcGIS service and the USDA Soil Data Access tabular endpoint. There is no dry-run flag. Do not run two imports at once. The second run exits non-zero with `source import did not complete cleanly: skipped_overlap`.
- The soil fetch prints nothing. The first `[union_county_gis]` line appears only after that fetch returns. After `Fetch complete:`, soil storage and screening also print nothing. The process can stay quiet for minutes. Wait for the exit code.
- The deadline is 10 minutes for soil fetch, parcel import, soil storage, and screening. Parcel batch size is 2000, at about one request per second. A deadline failure exits non-zero. Stderr then contains one of `fetch soil polygons:`, `import parcels:`, `store soil geometry:`, or `required soil screening:`, followed by `context deadline exceeded` or `rate: Wait(n=1) would exceed context deadline`. Do not stop after the first parcel batch, or after the import summary, and call the drive verified.
- Each county request uses a 30 second client timeout and waits 15 seconds for response headers. The USDA name lookup uses a 5 second limit and can fail open while the parent context is still active. Screening uses the default HTTP client. A stalled county request fails the drive while the 10 minute deadline is still running.
- A short final page is valid when its length is the remainder under 2000. Any other short page fails. Stderr contains `import parcels:` and `count mismatch: page <n> got <x> want <y>`. The page number in that error is 0-based. `Batch <i>/<total> complete` is 1-based. A summed-page mismatch uses `count mismatch: fetched <got> want <want>`, also wrapped by `import parcels:`.
- Likely vacant means numeric zero `YEARBLT` or numeric zero `SQFT`. A zero improvement value, or a land type that contains `RURAL ACREAGE TRACT`, does not by itself mark a parcel vacant. An empty `PID` is stored with `likely_vacant` false even when the filter counted the feature. The stdout integer can be higher than `count(*)` of `likely_vacant` true rows.
- `make run` rebuilds before it executes, and Make exports `.env`. That URL is the operator database. Use `./landman` with `HOST_DATABASE_URL`.
- Fatal text on stderr includes `create GIS source:`, `fetch soil polygons:`, `create parcel writer:`, `import parcels:`, `commit parcels:`, `store soil geometry:`, `required soil screening:`, `source import did not complete cleanly:`, `required soil screening did not complete cleanly:`, and `invalid SOIL_SCREENING_MAX_AGE`. The process exits non-zero. A bad `DATABASE_URL` or a job-runner error reaches `log.Fatal` with no GIS prefix.
- Screening, assessment, and measurements print nothing. Their operator-visible failure text is `required soil screening:`. `soil_measurements` may be empty when USDA returns no measurement rows. An exit of 0 still requires the screening job outcome `success` and a `soil_screening_heads` row.
- `SaveSales` has no operator entry point. `parcel_sales` stays empty. Do not prove this feature by inserting sales from a scratch program.
- The import commits parcel shapes before screening. A screening failure can leave county parcel rows and the import summary behind, with exit code non-zero. That is a failed drive.
