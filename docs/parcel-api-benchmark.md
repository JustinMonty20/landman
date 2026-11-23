# Measure parcel API requests

Use a migrated disposable copy of ingested Union County data. Import the four offline tax files through `jobs.Runner.Run` before measuring. Do not run the fixture integration test against the county copy.

Set `API_PERF_DATABASE_URL` to a PostgreSQL database whose name starts with `landman_api_perf_`. The tagged test checks `current_database()` before creating its temporary authentication user and session. It does not truncate parcel, soil, or tax tables. Run:

```sh
API_PERF_DATABASE_URL="$API_PERF_DATABASE_URL" go test -tags perf ./internal/auth -run '^TestParcelRealData$' -count=1 -v
```

The test mounts the real parcel handler behind the authentication server. Google login is outside the timed requests. Session lookup, host checks, deadline, and response headers remain inside. Each route runs five serial rounds and five rounds with four concurrent requests. Logs include actual HTTP status, error counts, response bytes, and p50 and p95 duration. p95 uses nearest rank (sorted index `ceil(0.95*n)-1`), so the five-sample serial p95 is its maximum and the 20-sample concurrent p95 is observation 19. Timing includes the complete response body read. `first_observed` is not a cold-cache measurement. The harness checks list identities/order and disjoint later pages, full-resolution map intersection and cell counts, published soil identity, and active tax evidence against independent queries on the copied database. It exercises absent lookup prefixes and moving/broad map views; a dense feature-limit request asserts HTTP 413 outside timing. A later-page request uses a cursor returned by the first page. HTTP 413 and 503 count as failures, not fast successful requests. Adjust the close-map viewport to a representative bounded view if the copied county has more than 200 intersecting parcels.

Record the machine load, PostgreSQL and PostGIS versions, source counts, first-use and repeat results, and request logs alongside query plans. Collect plans on the same disposable copy with `EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)` for the predicates in `internal/parcelapi/api.go`. For prepared SQL compare `SET plan_cache_mode=force_custom_plan` and `force_generic_plan` separately on selective and broad filters and a missing lookup prefix. Record buffer, execution and JIT time; do not profile during HTTP timing. For comparisons freeze the harness in a detached baseline-code worktree, keep the migration-19 baseline copy unchanged, migrate an independent cloned disposable database to 21, and run the same harness bytes at least five times per side in alternating order. Save logs, machine load, versions, and plan outputs outside Git. Compare a retained index only after repeated runs return the same results. No latency budget is assigned before the baseline.

For the isolated HTTP fixture tests, use a different migrated database named `landman_api_test_*`:

```sh
API_TEST_DATABASE_URL="$API_TEST_DATABASE_URL" go test -race -tags postgres ./internal/parcelapi -count=1
```

## Discovery contract

`GET /api/v1/parcels?discovery=browse` opts into paged, unfiltered county discovery. Without this explicit mode or a filter the legacy list remains empty. `GET /api/v1/parcels/map` always requires a bounded `bbox` and `zoom`; with `discovery=browse` its grid cells and close-zoom features mark all intersecting parcels as matches. Without browse or filters neutral close-zoom geometry remains available. The county count comes from the list response, not a county-wide geometry transfer. Changing discovery mode or any filter invalidates the keyset cursor. A changed published source stamp also invalidates it.

Both list and map accept `min_acres`, `max_acres`, `likely_vacant`, `delinquent`, `out_of_state_mailing`, `entity_name_indicator`, `corporate_name_indicator`, `flood_category`, `flood_contact`, `jurisdiction`, `foreclosure`, `soil_type`, and the paired `soil_use`/`soil_rating`. Selected predicates combine with AND; `foreclosure=true` means membership in either active published tax file feed, not an open case. Soil type is a stable map-unit key with positive stored overlap in the active soil set. Soil rating requires current published assessment, positive exclusive acreage, matching geometry and soil set, unexpired supported assessment method, no refresh failure, and a rated USDA interpretation. Valid uses are `conventional-septic`, `dwelling-without-basement`, `dwelling-with-basement`; ratings are `Not limited`, `Somewhat limited`, `Very limited`. Rating is mapped evidence, not site suitability. `GET /api/v1/parcels/soil-types` supplies up to 500 named units from the active soil set under the same authenticated data handler. The route returns 413 if the bounded choice limit is exceeded rather than silently dropping choices.
