# Import parcels and soil

`./landman` reads `DATABASE_URL` and ignores extra arguments, so `./landman import` is the same command. It runs `union.gis.parcels`, then `union.gis.soil`, then `union.soil.screening`. The parcel job upserts every parcel and its kept sale slots and prints how many feature IDs looked vacant. The soil job stores soil. The screening job screens that committed set. An error, or any outcome other than `success`, stops the later jobs.

## Sub-features

- `import-build` produces the `./landman` binary.
- `import-run` writes parcels and soil, then runs screening inside one 10 minute deadline.
- `import-summary` prints the vacant feature-ID total after the parcel commit.
- `import-jobs` records `job_attempts` for `union.gis.parcels`, `union.gis.soil`, and `union.soil.screening`.
- `import-verdict` runs `classify` and writes `drive-outcome.json`.

## How to get to it (user POV)

- From the repo root, run `make build`, then run `./landman` with `DATABASE_URL` set to the verify database host URL in `SKILL.md`.
- `make run` is the operator shortcut. It loads `.env` when that file exists. Verification does not use it.

## Driving it with make and the landman binary

Preconditions:

- The verify database launch in `SKILL.md` has exited 0, including `check-loaded`.
- Database doctor and CLI doctor have passed for this container.
- No other `./landman` process is running.
- `SOIL_SCREENING_MAX_AGE` is unset, or it is a valid positive Go duration. The default freshness interval is 30 days. An explicit run still refreshes USDA evidence.

- Run `make build`. Exit code 0 and `./landman` exists.
- Treat the `check-loaded` census as the reset check. Stdout includes `census parcels=` and a `census parcel` line for `07102010` and for `07102051`, then `loaded: ok`. Do not start `./landman` until that command exits 0.
- Run `probe` with `--evidence "$EVIDENCE"`, using the helper path in `SKILL.md`. Exit code 0. `page-timings.json` is in that directory. A slow data page still exits 0 when both metadata calls returned HTTP 200.
- Run `DATABASE_URL="$HOST_DATABASE_URL" ./landman`, capturing stdout, stderr, and the exit code under `artifacts/verify/$RUN_ID/`. Keep the process until it exits.
- Read parcel progress on stderr. It contains `[union_county_gis] Total parcels to fetch: ` with a count greater than 0, then `[union_county_gis] Batch size: 2000, Total batches: `, then `[union_county_gis] Fetch complete:`. Those lines start during the parcel job, before the soil fetch. The soil fetch itself prints nothing.
- Read the summary on stdout. It contains `Imported parcels, ` and ` likely-vacant feature IDs`. This line is printed before soil storage and screening finish.
- Run `classify` with the flags in `SKILL.md` after the process exits, including when that exit is not 0. A clean import exits 0 and `classify` prints `pass`. `drive-outcome.json` then records `seededSales` and `fullImport` as `met`. The county parcel count is greater than the seed parcel count. Soil polygons and overlaps for `union` and `union_county_gis` are greater than the seed insert counts. `job_attempts` has outcome `success` for `union.gis.parcels`, for `union.gis.soil`, and for `union.soil.screening`. `soil_screening_heads` has one row for that county and source. `soil_parcel_assessments` has the same count as the imported county parcels. Do not query a county-wide `parcel_sales` total. The import replaces the seed soil rows with the county soil set.
- Keep `import.stdout.txt`, `import.stderr.txt`, `import.exit.txt`, `page-timings.json`, and `drive-outcome.json`. On `pass`, the exit file contains `0`. Stderr for that success does not contain `import parcels:`, `store soil geometry:`, `required union.gis.parcels:`, or `context deadline exceeded`.

## Gotchas

- This drive hits the live county ArcGIS service and the USDA Soil Data Access tabular endpoint. There is no dry-run flag. Do not run two imports at once. The second run exits non-zero with `required union.gis.parcels did not complete cleanly: skipped_overlap`.
- The soil fetch prints nothing. It runs after `Fetch complete:` and after the import summary. The first `[union_county_gis]` line is the parcel job. After `Fetch complete:`, soil storage and screening also print nothing. The process can stay quiet for minutes. Wait for the exit code.
- The deadline is 10 minutes for parcel import, soil fetch, soil storage, and screening. Parcel batch size is 2000, at about one request per second. A deadline failure exits non-zero. Stderr then contains `required union.gis.parcels:`, `required union.gis.soil:`, or `required union.soil.screening:`. A parcel, fetch, or store failure also contains `import parcels:`, `fetch soil polygons:`, or `store soil geometry:`. The line contains `context deadline exceeded` or `rate: Wait(n=1) would exceed context deadline`. Do not stop after the first parcel batch, or after the import summary, and call the drive verified.
- `classify` prints `inconclusive: county-source-slow` and the helper exits 2 when stderr contains `context deadline exceeded` or `rate: Wait(n=1) would exceed context deadline`, and the county was slow. Slow means metadata returned HTTP 200 within 5 seconds and either a probe data page took 15 seconds or a parcel batch gap was 15 seconds. A fast probe page still counts as slow when the batch gap is 15 seconds. Without that deadline text, a non-zero exit is `fail`. That inconclusive verdict is not a pass. `go run` turns the helper exit into shell exit 1 and prints `exit status 2` on stderr. Read the verdict line. If the import summary is absent, `seededSales` is `not-run` and `fullImport` is `missed`. If the summary is present and the seeded slots match, `seededSales` is `met` and `fullImport` is `missed`.
- Each county request uses a 30 second client timeout and waits 15 seconds for response headers. The USDA name lookup uses a 5 second limit and can fail open while the parent context is still active. Screening uses the default HTTP client. A stalled county request fails the drive while the 10 minute deadline is still running.
- A short final page is valid when its length is the remainder under 2000. Any other short page fails. Stderr contains `import parcels:` and `count mismatch: page <n> got <x> want <y>`. The page number in that error is 0-based. `Batch <i>/<total> complete` is 1-based. A summed-page mismatch uses `count mismatch: fetched <got> want <want>`, also wrapped by `import parcels:`.
- Likely vacant means numeric zero `YEARBLT` or numeric zero `SQFT`. A zero improvement value, or a land type that contains `RURAL ACREAGE TRACT`, does not by itself mark a parcel vacant. An empty `PID` is stored with `likely_vacant` false even when the filter counted the feature. The stdout integer can be higher than `count(*)` of `likely_vacant` true rows.
- `make run` rebuilds before it executes, and Make exports `.env`. That URL is the operator database. Use `./landman` with `HOST_DATABASE_URL`.
- Fatal text on stderr includes `create GIS source:`, `fetch soil polygons:`, `create parcel writer:`, `import parcels:`, `commit parcels:`, `store soil geometry:`, `required union.gis.parcels:`, `required union.gis.soil:`, `required union.soil.screening:`, `required union.gis.parcels did not complete cleanly:`, `required union.gis.soil did not complete cleanly:`, `required union.soil.screening did not complete cleanly:`, and `invalid SOIL_SCREENING_MAX_AGE`. The process exits non-zero. A bad `DATABASE_URL` or a job-runner error reaches `log.Fatal` with no GIS prefix.
- Screening, assessment, and measurements print nothing. Their operator-visible failure text is `required union.soil.screening:`. `soil_measurements` may be empty when USDA returns no measurement rows. An exit of 0 still requires the screening job outcome `success` and a `soil_screening_heads` row.
- `SavePart` upserts each parcel and its kept slots on the import transaction and deletes omitted sequences. `SaveSales` alone does not delete. Compare sale slots through `classify`, which reads only the seed parcel ids. Do not prove this feature by inserting sales from a scratch program.
- The import commits parcel shapes before the soil job and screening. A later failure can leave county parcel rows and the import summary behind, with exit code non-zero. `classify` then prints `inconclusive: county-source-slow` only when the deadline text is present, the seeded slots match, and the county was slow by the rule above. Any other non-zero exit after that commit is `fail`.
