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
