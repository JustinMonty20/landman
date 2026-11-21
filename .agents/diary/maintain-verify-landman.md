# maintain-verify-landman

## Out-of-scope decision

The verify pass uses `docker run` for `landman-verify-postgis` and passes `DB_CONTAINER` on the `make` command line. `db-create` still creates the database named `landman` inside that container. A new Compose service or Makefile target is not required, so this pass does not add one.

## Out-of-scope decision

`README.md` says changing `POSTGRES_PORT` moves the published port. `docker-compose.yml` hardcodes `5432:5432`. The skill records that mismatch. The README is product documentation and stays out of this pass.

## Out-of-scope decision

`make migrate-create` writes a migration file. It is not an operator verification drive. Reset stays `db-drop` and `db-create` on the verify container, so `migrate-down` is not a mapped feature.

## Out-of-scope decision

PR 43 writes parcel sales from `SavePart`. `SavePart` upserts each parcel and its kept slots on the import transaction and deletes omitted sequences. `SaveSales` alone does not delete. The earlier note that `SaveSales` has no caller is wrong. This pass does not change product code. `RateLimit` 1 and the 10 minute deadline stay out of scope.

## Live proof

`capture-seed` at 2026-11-14T23:06:21Z replaced `seed.sql` from the public query endpoints. The 36 parcel ids are unchanged. After a fresh migrate to version 14, `check-loaded` reported 18 vacant, 18 built, 7 map units, 9 polygons, 71 overlaps, and 79 sales. Map unit `115369` (`W`) has null names because the previous seed had no name for that key.

The live `./landman` import hit the 10 minute deadline at batch 29/60 with `import parcels: parcelstore: lookup parcel: timeout: context deadline exceeded`. `classify` recorded `inconclusive: county-source-slow`, `seededSales` `not-run`, `fullImport` `missed`. Metadata was fast (parcel 123 ms, soil 43 ms). The first data pages were fast (397 ms and 905 ms). Later parcel batch gaps grew past 15 seconds. That best-effort miss does not block the seed and schema edits. Evidence is `artifacts/verify/20261114T230615-21306`, which is gitignored.

## Out-of-scope decision

`go run` turns a helper exit of 2 into shell exit 1. The skill now says to read the verdict line. This pass does not add a wrapper binary. `control-cli` and `cursor-team-kit` `deslop` are not installed here.

## Out-of-scope decision

Comment review asked to bind sale ids through `database/sql` and to delete the `seed.sql` header. The header is the capture record, and `psql -c` is the helper's query path. Neither change landed. The batch-1 gap skip did land as a behavior fix: `FetchSoilPolygons` returns before `Total parcels to fetch`, so that gap is the first parcel page.

## Out-of-scope decision

Left product code alone after the independent-schedule merge. The live `./landman` run on 2026-11-15 hit the ten minute deadline at parcel batch 31 of 60. `classify` printed `inconclusive: county-source-slow`. That is the documented slow-county result. It is not a map patch.

## Out-of-scope decision

Did not run `./landman run union.gis.parcels` or `./landman run union.gis.soil`. This pass keeps one county fetch, and that fetch is bare `./landman`.

## Out-of-scope decision

Did not refresh `seed.sql`. After migrate to version 15, `check-schema` printed `schema: ok` and `check-loaded` printed `loaded: ok`. Migration `000015_job_pending.sql` adds no column. The capture header is still 2026-11-14T23:06:21Z.

## Out-of-scope decision

Added `features/schedule-union-jobs.md` instead of folding `./landman schedule` into the import feature. The map contract is one user-visible behavior per feature file, and schedule does not import at startup.

## Out-of-scope decision

Left `[union_county_gis] Fetch cancelled after <n> batches` off the stable handles. A cancelled parcel fetch still exits with `context deadline exceeded` and `required union.gis.parcels:`, which the map already names.

## Out-of-scope decision

Did not describe `ANALYZE parcels, parcel_sales` in the feature map. That statement writes no log line.

## Out-of-scope decision

Did not run `./landman run union.gis.parcels` or `./landman run union.gis.soil`. This pass keeps one county fetch, and that fetch is bare `./landman`.

## Out-of-scope decision

`cursor-team-kit` `deslop` and `control-cli` are not installed here. The helper tests and the live drive are the checks.

## Product gap

The live `./landman` import on 2026-11-15 started at 04:07:03Z on `landman-verify-postgis`. Parcel fetch finished in 2m56s. `union.gis.parcels` and `union.gis.soil` recorded `success`. `union.soil.screening` recorded `interrupted` at 04:17:03Z with `soil assessment hits: timeout: context deadline exceeded`. Probe pages were 469 ms and 981 ms. `classify` printed `fail`, with `seededSales` `met` and `fullImport` `missed`. This pass does not change product code. Evidence is `artifacts/verify/20261115T040419Z-8693`, which is gitignored.

## Out-of-scope decision

Left `reprocess` inside the import feature. A second feature file would repeat the same database and the same screening path.

## Out-of-scope decision

Did not refresh `seed.sql` after migration 16. `attempt_kind` has a default, the seed does not insert `job_attempts`, and `check-loaded` printed `loaded: ok`. The capture header is still 2026-11-14T23:06:21Z.

## Out-of-scope decision

Did not put the vacant-count stdout line back into the product. `docs/union-jobs.md` does not mention it. The helper now reads publication from `job_attempts`.

## Out-of-scope decision

Did not run `./landman run union.gis.parcels` or `./landman run union.gis.soil`. This pass keeps one county parcel fetch, and that fetch is bare `./landman`.

## Live proof

On 2026-11-15 at 11:13:08Z, bare `./landman` against `landman-verify-postgis` fetched 118202 parcel records in 1m0s. Stdout was empty. `union.gis.parcels` and `union.gis.soil` recorded `full_scan` `success` with zero quarantine. `union.soil.screening` recorded `interrupted` at 11:23:08Z with `timeout: context deadline exceeded`. Probe pages were 510 ms and 969 ms. `classify` printed `fail`, with `seededSales` `met` and `fullImport` `missed`. `./landman reprocess union.gis.parcels --limit 1` then exited 0 with empty output and no parcel fetch. Its parcel attempt was `reprocess` `success` with processed 0, and screening recorded `success`. Evidence is `artifacts/verify/20261115T111044-10364`, which is gitignored.

## Out-of-scope decision

Edited `.cursor/skills/verify-landman-cli` because `04306a0` moved the feature map, seed, and launch out of `.cursor/skills/verify-landman`. That directory is now the router. The corrections belong in the map.

## Out-of-scope decision

Did not refresh `seed.sql`. After migrate to version 41, `check-schema` printed `schema: ok` and `check-loaded` printed `loaded: ok` for 36 parcels, 18 vacant and 18 built. The capture header is still 2026-11-14T23:06:21Z.

## Out-of-scope decision

Did not drive `union.gis.zoning`, `union.gis.roads`, `union.fema`, `union.gis.structures`, `union.gis.assessed_value`, or `replay`. This pass keeps one GIS journey, soil then parcels. The usage line now names the undriven commands. No new feature file without a live drive.

## Product gap

On 2026-11-20, `./landman run union.gis.parcels` against `landman-verify-postgis` committed `full_scan` `success` with processed 118204, then `union.soil.screening` recorded `interrupted` at the 10-minute deadline with `soil assessment map unit areas: timeout: context deadline exceeded`. Stderr also printed a Postgres dial timeout on that same cancelled context. Probe pages were 437 ms and 910 ms. `classify` printed `fail`, with `seededSales` `met` and `fullImport` `missed`. An empty `reprocess --limit 1` then finished screening and qualification. This pass does not change product code. Evidence is `artifacts/verify/20261120T035159Z-4781`, which is gitignored.

## Out-of-scope decision

`cursor-team-kit` `deslop` is not installed here. The helper tests and the live drive are the checks.

## Out-of-scope decision

Did not edit `.cursor/skills/verify-landman-api`, `.cursor/skills/verify-landman-tax`, or `.cursor/skills/verify-landman-ui`. The sweep named `GET /api/v1/flood/map`, `GET /api/v1/qualification/map`, `GET /api/v1/parcels/{id}/flood`, `GET /api/v1/parcels/{id}/vacancy`, and `owner_labels` on the parcel read in `internal/parcelapi/api.go`, plus stored shapes on `ui/src/routes/+page.svelte` and the `/coverage` route. This pass did not drive those skills.

## Out-of-scope decision

Did not start a second GIS import after the assessment count joined `soil_screening_heads`. The helper test checks that join. The 2026-11-20 journey remains the one county drive.

## Skipped refactor

Did not rewrite `features/import-journey.md` into the four-heading contract. The drive followed `live-gis.md`.

## Out-of-scope decision

Left the journey sentence that screening and assessment print no lines. `internal/qualification/derive.go` logs step lines when a qualification head exists. Soil stderr on this drive was empty, and parcel stderr contained only the deadline.

## Product gap

On 2026-11-20 at 04:39:20Z, `./landman run union.gis.parcels` against `landman-verify-postgis` logged `Parcel capture page` lines and committed `full_scan` `success` with processed 118204 after 7m36s. `union.soil.screening` then recorded `interrupted` at the same command's 10-minute deadline. The error was `refresh expired screening: timeout: context deadline exceeded`, followed by a Postgres dial timeout on that cancelled context. Probe pages were 437 ms and 1069 ms. An empty `reprocess --limit 1` finished screening in 4m17s with no parcel capture lines. The separate downstream deadline in `unionExecutor` does not extend `./landman run union.gis.parcels`, because `run` still limits that mode to 10 minutes before the executor starts. This pass does not change product code. Evidence is `artifacts/verify/20261120T043546Z-50629`, which is gitignored.

## Out-of-scope decision

Did not run bare `./landman`, `union.fema`, zoning, roads, structures, assessed value, or the publication-retry performance test. Bare `./landman` with tax paths set and FEMA paths unset exited 1 with `UNION_FEMA_ARTIFACT_DIR is required` before it opened the database. `plans/union-terrain-ingestion.md` names no `landman` command. `docs/parcel-publication-retry.md` describes a store change and a Go test, not a new flag.

## Live proof

After the successful reprocess screening, county parcels and assessments on the current `soil_screening_heads` row were both 117985. The unfiltered `soil_parcel_assessments` count was 118021. The extra 36 rows are the seed screening run. The head join is the count `classify` uses. This pass did not start a second county import to re-drive that count.

## Out-of-scope decision

The earlier notes that left the journey silent, and that skipped the four-heading rewrite, describe the 03:51Z drive. The 04:35Z drive showed capture and screening lines on stderr. `features/import-journey.md` now uses the four headings and names those lines.

## Out-of-scope decision

Did not edit `docs/union-qualification-verification.md`. It still says zoning method `stored-overlay-measured/5`. The code constant in `internal/qualification/store.go` is `stored-overlay-measured/4` with the EPSG:3857 grid suffix. The map names the code string.

## Out-of-scope decision

`cmd/owner-label-backfill` prints a stored-parcel classification count and does not fetch the county. The router selects the CLI skill for it. This pass does not add a feature file for it.

## Out-of-scope decision

Did not edit `docs/union-jobs.md`. The scheduler section says to set both calendar expressions, while `cmd/landman/main.go` requires five. The same section says a clean parcel run always screens. A parcel command screens only when stored screening is due.

## Out-of-scope decision

Edited the bare-command paragraph in `verify-landman-tax` because the CLI map sends `./landman` with no arguments there. That paragraph still said the combined process has one ten-minute deadline and listed only parcels, soil, screening, and tax. The command checks the three FEMA paths before it opens the database, then gives each later step its own ten minutes. This pass did not run that full command.

## Live proof

On 2026-11-20 at 04:41:21Z, the seed still loaded after goose version 41. `check-schema` printed `schema: ok`. `check-loaded` printed 36 parcels, 18 vacant and 18 built. One `run-gis.sh` drive on `landman-verify-postgis` logged `Parcel capture page` and no `Batch` lines. Soil exited 0, including screening. The parcel command committed `full_scan` `success` with processed 118204, then screening recorded `interrupted` at that command's 10-minute deadline. Qualification on that failure printed `Wait for assessment lock` and did not print `Finished`. Probe pages were 481 ms and 947 ms. `classify` printed `fail`, with `seededSales` `met` and `fullImport` `missed`. Empty `reprocess --limit 1` exited 0 with no parcel capture line. Screening then finished in 4m32s. Evidence is `artifacts/verify/20261120T044121Z-5952`, which is gitignored.

## Out-of-scope decision

Did not edit `docs/union-qualification-verification.md`. Its older paragraphs still say a two-grid stop and a generation rollback. `TestPrecisionRetryPartition` and `TestNoGridRetryPreservesSliverAndRollsBackOnFailure` passed on `landman_tax_integration`. A fine-grid miss logs `retry=no-grid` and uses the stored shapes. A required hit that still has no positive area or share fails the assessment attempt and keeps the previous rows. The published generation stays. The map now says that. The doc stays out of this PR.

## Out-of-scope decision

`cursor-team-kit` `deslop` is not installed here. The live drive and the precision tests are the checks. Claude Opus and Sonnet were rejected for weekly quota, so the source readers ran on Grok.

## Index and doctor correction

`features/README.md` said `import-journey.md` keeps the heading `Reset through classify`. That file uses the four feature sections. The CLI doctor paragraph said exit 0 also calls the USDA tabular endpoint. `probe` requests only layers 215 and 197. The soil command logs `Fetch USDA names` and `Fetch USDA interpretations` during screening. An interpretation fetch error fails screening. A map-unit name error during soil store does not fail that store. Evidence is `artifacts/verify/20261120T044121Z-5952/soil.stderr.txt`.

## Live proof

On 2026-11-20 at 21:31:06Z, after merging `cebc5db`, the 2026-11-14 seed still loaded on goose version 41. `check-schema` printed `schema: ok`. `check-loaded` printed `loaded: ok`. One `run-gis.sh` drive on `landman-verify-postgis` logged `Checked` at every 10000 parcel groups through 110000, then `Parcel publication complete: processed=118205`. Soil exited 0. The parcel command committed `full_scan` `success` in 8m22s, then `union.soil.screening` recorded `interrupted` at that command's 10-minute deadline. Qualification on that failure printed `Wait for assessment lock` and did not print `Finished`. Probe pages were 214 ms, 49 ms, 492 ms, and 1093 ms. `classify` printed `fail`, with `seededSales` `met` and `fullImport` `missed`. Pending parcel quarantine was 0. Empty `reprocess --limit 1` exited 0 with `Repair: 0 pending records selected` and no `Parcel capture` line. Screening was still due, so it printed `Finished` after 4m19s, and qualification printed `Finished`. A second empty reprocess exited 0, printed qualification `Finished`, and did not print a screening line. Missing calendars and a missing `UNION_QUALIFICATION_CRON` each exited 1 before the database opened. A not-due `./landman schedule` printed the startup reconcile, did not fetch the county, and exited 0 on SIGTERM. `TestPrecisionRetryPartition` and `TestNoGridRetryPreservesSliverAndRollsBackOnFailure` passed. Empty `union.qualification.assess` printed the no-head skip and exited 0. Evidence is `artifacts/verify/20261120T212924Z-18426`, which is gitignored.

## Product gap

The same parcel deadline remains after the assessment speedup. `./landman run union.gis.parcels` still shares one 10-minute limit with derived screening. Publication can succeed and screening can still be interrupted. The cancelled context still logs a Postgres dial timeout. This pass does not change product code.

## Out-of-scope decision

Did not add headed assessment step lines, including `Release source geometry views`, to `qualification.md`. Those lines print when a generation exists. This journey has no zoning or road head. The no-head order still matches. Live zoning and roads stay behind explicit live approval.

## Out-of-scope decision

Did not refresh `seed.sql`. Goose version 41 applied, `check-schema` and `check-loaded` passed, and `classify` reported `seededSales` `met`. The capture header is still 2026-11-14T23:06:21Z.

## Live proof

On 2026-11-20 at 22:05Z, after merging `9735872`, the 2026-11-14 seed still loaded. `check-schema` printed `schema: ok`. `check-loaded` printed `loaded: ok` for 36 parcels. With `LANDMAN_HEAP_PROFILE_DIR` unset, `./landman run union.qualification.assess` exited 0 and printed no heap line. `run-gis.sh` then logged no heap line. Soil exited 0. The parcel command published 118205 parcels in 8m6s, then screening recorded `interrupted` at the 10-minute deadline. Qualification printed `Wait for assessment lock` and did not print `Finished`. With the variable set to an existing directory, `./landman run union.gis.soil` logged `Heap profile socket` before capture, kept a private `heap-*` directory and `heap.sock` while running, served `GET /heap` with HTTP 200, exited 0, and removed that directory. Stdout stayed empty. A missing parent and a file parent each exited 1 with `create heap profile directory` and did not start the soil job. `./landman import` with a missing parent still exited 1 with usage. Evidence is `artifacts/verify/20261120T220503Z-43627`, which is gitignored.

## Out-of-scope decision

Did not add `.agents/skills/landman-heap-profile` or `.agents/skills/landman-ui-cdp` to the verify feature map. Those are operator capture skills. The verify map records the `LANDMAN_HEAP_PROFILE_DIR` startup contract on the CLI. The API process recipe already starts with `env -i` and does not set the variable. This pass did not start `cmd/api`.

## Out-of-scope decision

Did not edit `docs/union-qualification-verification.md` after the roads assessment change. That file still says nearest segments are chosen before projection, and it still says the EPSG:3857 8 km candidate filter remains. Current code projects centerlines once and searches a 5,280-foot radius. The map now says those sentences are not the current job. `docs/union-jobs.md` still says to set both calendar expressions and that a clean parcel run always screens. Those product sentences stay out of this PR.

## Out-of-scope decision

Did not start `./landman run union.gis.parcels` after `run-gis.sh` stopped. The runner stops on the first nonzero status. That stop is the one import drive.

## Live proof

On 2026-11-21 at 01:44Z, after merging `23f1f22`, the 2026-11-14 seed still loaded on goose version 41. `check-schema` printed `schema: ok`. `check-loaded` printed 36 parcels, 18 vacant and 18 built. `TestManualRoadAssessmentGetsIndependentHour` passed. Empty `union.qualification.assess` printed the no-head skip and exited 0. Missing calendars and a missing `UNION_QUALIFICATION_CRON` each exited 1. A not-due `./landman schedule` printed the startup reconcile, did not fetch the county, and exited 0 on SIGTERM. Probe metadata was 135 ms and 41 ms. Probe pages were 415 ms and 985 ms. `run-gis.sh` published 11250 soil polygons in 2m58s, then USDA interpretation fetch used the rest of the shared ten-minute deadline. Screening recorded `interrupted` at 7m2s. Qualification printed `Wait for assessment lock` and did not print `Finished`. The process also logged a Postgres dial timeout on the cancelled context. The runner exited 1 and did not start parcels. `classify` printed `fail`, with `seededSales` `not-run` and `fullImport` `missed`. Pending parcel quarantine was 0. Empty `reprocess --limit 1` exited 0, printed `Repair: 0 pending records selected`, did not print `Parcel capture`, and finished screening in 4m55s. Qualification then printed `Finished`. Evidence is `artifacts/verify/20261121T014246Z-9802`, which is gitignored.

## Tool failure

The Task tool rejected `comment-sicko` as a subagent type. The map diff has no code comments, so nothing was deleted from that review.

## Product gap

PR 98's merge commit `e7f4aa0` left `cmd/landman/main.go` unable to compile. `afterSource` still opens with `if id == parcelJobID` and then uses `case soilJobID`. Commit `bb3da92` had `switch id` and `case parcelJobID`. `go test ./cmd/landman` fails at `main.go:438`. Main's Go checks for that merge failed the same way. This pass does not edit product code, so it cannot build `./landman` or record a qualification attempt.

## Live proof

PR 100 restored `switch id` in `afterSource`. On 2026-11-21 the 2026-11-14 seed still loaded on goose version 41 in `landman-verify-postgis`. `check-schema` printed `schema: ok`. `check-loaded` printed `loaded: ok` for 36 parcels. `go build -o ./landman ./cmd/landman` exited 0. `go test ./cmd/landman -count=1` passed. A not-due `./landman schedule` stayed running, wrote no stderr, inserted no `job_attempts` row, and exited 0 on SIGTERM. `run-gis.sh` published 11250 soil polygons and finished screening in 15s. The parcel command published 118205 parcels in 7m43s, then screening recorded `interrupted` at that command's ten-minute deadline. Neither GIS log contained `[union.qualification.assess]`. `job_attempts` after the runner had soil success, screening success, parcel success, and screening interrupted. `./landman run union.qualification.assess` then printed the no-head skip and added one `full_scan` `success` row with `processed_count` 0. Pending parcel quarantine was 0. Empty `reprocess --limit 1` printed `Repair: 0 pending records selected`, did not print `Parcel capture`, finished screening in 4m13s, and did not print `[union.qualification.assess]`. `classify` printed `fail`, with `seededSales` `met` and `fullImport` `missed`. Evidence is `artifacts/verify/20261121T031500Z-qualify`, which is gitignored.

## Product gap

`./landman run union.gis.parcels` still shares one ten-minute limit with the following soil screening. Publication can succeed and that screening can still be interrupted. `docs/union-jobs.md` line 49 still says a manual roads command gets an automatic 60-minute assessment. Line 127 says qualification does not run after source imports. Those product sentences stay out of this pass. The roads command still builds a one-hour downstream context whose callback returns immediately.
