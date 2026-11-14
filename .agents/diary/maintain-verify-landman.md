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
