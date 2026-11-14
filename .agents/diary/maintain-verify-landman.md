# maintain-verify-landman

## Out-of-scope decision

The verify pass uses `docker run` for `landman-verify-postgis` and passes `DB_CONTAINER` on the `make` command line. `db-create` still creates the database named `landman` inside that container. A new Compose service or Makefile target is not required, so this pass does not add one.

## Out-of-scope decision

`README.md` says changing `POSTGRES_PORT` moves the published port. `docker-compose.yml` hardcodes `5432:5432`. The skill records that mismatch. The README is product documentation and stays out of this pass.

## Out-of-scope decision

`make migrate-create` writes a migration file. It is not an operator verification drive. Reset stays `db-drop` and `db-create` on the verify container, so `migrate-down` is not a mapped feature.

## Out-of-scope decision

PR 43 writes parcel sales from `SavePart`. `SavePart` upserts each parcel and its kept slots on the import transaction and deletes omitted sequences. `SaveSales` alone does not delete. The earlier note that `SaveSales` has no caller is wrong. This pass does not change product code. `RateLimit` 1 and the 10 minute deadline stay out of scope.
