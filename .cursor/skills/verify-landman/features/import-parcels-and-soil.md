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
- **Read the seed before the import.** Run `docker exec -i "$VERIFY_CONTAINER" psql "$GOOSE_DATABASE_URL" -v ON_ERROR_STOP=1 -c "SELECT count(*) AS parcels, count(*) FILTER (WHERE likely_vacant) AS vacant, count(*) FILTER (WHERE NOT likely_vacant) AS built FROM parcels;" -c "SELECT count(*) AS map_units FROM soil_map_units;" -c "SELECT count(*) AS polygons FROM soil_polygons;" -c "SELECT count(*) AS overlaps FROM soil_polygon_parcels;" -c "SELECT count(*) AS sales FROM parcel_sales;" -c "SELECT source_parcel_id, likely_vacant, situs_line_1 FROM parcels WHERE source_parcel_id IN ('07102010', '07102051') ORDER BY source_parcel_id;"`. The counts are 36 parcels, 18 vacant, 18 built, 6 map units, 7 polygons, 71 overlaps, and 79 sales. `07102010` is likely vacant at `105 CUPPED OAK DR`. `07102051` is not, at `203 WHITE OAK LN`. Every parcel row uses `source_name` `union_county_gis`. This read is the reset check. Do not start `./landman` until it matches.
- **Import.** Run `DATABASE_URL="$HOST_DATABASE_URL" ./landman`, capturing stdout, stderr, and the exit code under `artifacts/verify/$RUN_ID/`. The process exits 0.
- **See parcel progress.** Stderr contains `[union_county_gis] Total parcels to fetch: ` with a count greater than 0, then `[union_county_gis] Batch size: 2000, Total batches: `, then `[union_county_gis] Fetch complete:`. Those lines start after the soil fetch. The soil fetch itself prints nothing.
- **See the summary.** Stdout contains `Imported parcels, ` and ` likely-vacant feature IDs`. This line is printed before soil storage and screening finish. Keep the process until it exits.
- **Read the database after exit 0.** Using `docker exec` and `GOOSE_DATABASE_URL`, `07102010` and `07102051` are still present. The `union_county_gis` parcel count is greater than 36. `soil_polygons` is greater than 7 and `soil_polygon_parcels` is greater than 71, both for `county_id = 'union'` and `source_name = 'union_county_gis'`. `job_attempts` has outcome `success` for `union.gis.parcels` and for `union.soil.screening`. `soil_screening_heads` has one row for that county and source. `soil_parcel_assessments` has the same count as the imported county parcels. `parcel_sales` is still 79. The import upserts the sample parcel IDs and does not write or delete sales. Do not require the seed soil counts to stay unchanged. The import replaces them with the county-wide set.
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
- `SaveSales` has no operator entry point. The 79 `parcel_sales` rows come from the seed file, which copies layer 215 sale attributes for the sample parcel IDs. After exit 0 that count is still 79. Do not treat those rows as proof that `./landman` wrote sales, and do not prove this feature by inserting sales from a scratch program.
- The import commits parcel shapes before screening. A screening failure can leave county parcel rows and the import summary behind, with exit code non-zero. That is a failed drive.
