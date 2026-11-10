# Bootstrap the local database

An operator starts the PostGIS container, creates the `landman` database, and applies goose migrations so parcel tables exist locally.

## Sub-features

- `db-env` provides Compose and Makefile connection settings.
- `db-up` starts the `landman-postgis` container and waits until Postgres accepts connections.
- `db-create` creates the `landman` database if it does not exist.
- `db-migrate` applies every pending file in `db/migrations/`.

## How to get to it (user POV)

- Copy `.env.example` to `.env` if `.env` is missing, then run `docker compose up -d postgis`, `make db-create`, and `make migrate-up`.
- The README section "Start local PostGIS" followed by "Create, drop, or reset the local database" and "Run migrations" is the same path.

## Driving it with make and the landman binary

Preconditions:

- Docker is available.
- Either this run will start `landman-postgis`, or database doctor already found a healthy container with that exact name.
- This run will not run `make db-drop` or `make migrate-down` on a database it did not create.

- **Provide env.** If `.env` is missing, run `cp .env.example .env`. Do not overwrite an existing `.env`. The file must set `DB_CONTAINER=landman-postgis`.
- **Start PostGIS.** Run `docker compose up -d postgis --wait`. `docker inspect` on `landman-postgis` shows running and healthy. If a container named `postgis` appeared instead, Compose did not read `DB_CONTAINER`; stop and fix `.env` rather than pointing the Makefile at the wrong name.
- **Create the database.** Run `make db-create`. Exit code `0`. A second run also exits 0 (the bootstrap SQL is idempotent).
- **Apply migrations.** Run `make migrate-up`. Exit code `0`.
- **Confirm usable schema.** Run `make migrate-version`. Stderr contains `goose: version <n>`, where `<n>` is the integer value of the highest filename prefix in `db/migrations/` (prefix `000004` prints as `4`). Then run `docker exec -i landman-postgis psql "$DATABASE_URL" -c '\d parcels'`. The table exists and includes `id`, `county_id`, and `shape`.
- **Proof.** Save the `migrate-up` output, the `migrate-version` output, and the `\d parcels` output under `artifacts/verify/$RUN_ID/`. Proof requires the version match and the `parcels` table description, not only `pg_isready`.

## Gotchas

- `pg_isready` can pass while the `landman` database is missing or while migrations have not run. Healthcheck success is not readiness for this feature.
- Without `.env`, Compose names the container `postgis` and the Makefile looks for `landman-postgis`.
- Host port `5432` is hardcoded in `docker-compose.yml`. Changing `POSTGRES_PORT` in `.env` does not move that mapping.
- `make db-drop` destroys the shared `landman` database. Do not use it to "reset" an instance the operator already had.
- Goose migrations that have already been applied are not replayed if you edit an old file. New behavior belongs in a new numbered file.
