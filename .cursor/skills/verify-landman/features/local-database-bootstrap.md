# Bootstrap the verify database

An operator can start PostGIS, create the `landman` database, and apply goose migrations. Verification does that on a container this run created, then loads a one-row seed.

## Sub-features

- `db-verify` starts `landman-verify-postgis` on `127.0.0.1:55432`.
- `db-reset` drops and recreates the `landman` database on that container.
- `db-migrate` applies every pending file in `db/migrations/`.
- `db-seed` loads `.cursor/skills/verify-landman/seed.sql`.

## How to get to it (user POV)

- Verification follows the verify database launch in `SKILL.md`.
- The README sections "Start local PostGIS", "Create, drop, or reset the local database", and "Run migrations" are the operator path. That path uses `docker compose` and the container `landman-postgis`. Verification does not take it.
- The README start snippet copies `.env.example` onto `.env` and runs `docker compose up -d postgis` without `--wait`. Verification does neither.

## Driving it with make and the landman binary

Preconditions:

- Docker is available.
- The shell has `VERIFY_CONTAINER`, `VERIFY_VOLUME`, `POSTGRES_URL`, and `GOOSE_DATABASE_URL` from `SKILL.md`.
- `DB_CONTAINER` on every `make` command is `landman-verify-postgis`.
- This run will not `docker compose` and will not `make db-drop` against `landman-postgis`.

- **Start the verify container.** Run the `docker run` in `SKILL.md`. `docker inspect` on `landman-verify-postgis` shows running and healthy, image `postgis/postgis:18-3.6-alpine`, and host port `55432` on `127.0.0.1`.
- **Reset and create.** Run `make db-drop DB_CONTAINER="$VERIFY_CONTAINER" POSTGRES_URL="$POSTGRES_URL"`, then `make db-create` with the same overrides. Both exit 0. A second `make db-create` also exits 0.
- **Apply migrations.** Run `make migrate-up DB_CONTAINER="$VERIFY_CONTAINER" DATABASE_URL="$GOOSE_DATABASE_URL"`. Exit code `0`.
- **Confirm the version.** Run `make migrate-version` with the same `DB_CONTAINER` and `DATABASE_URL`. Exit code `0`. Stderr contains `goose: version ` and the integer of the highest filename prefix in `db/migrations/`. A log timestamp precedes that line. Stdout is the Make recipe. `make migrate-up` stderr contains `goose: successfully migrated database to version: ` and the same integer. The highest file is `000014_soil_measurements.sql` until a later migration lands. Do not expect version `4`.
- **Confirm the schema.** Run `docker exec -i "$VERIFY_CONTAINER" psql "$GOOSE_DATABASE_URL" -c '\d parcels'` and a public base-table listing. `parcels` includes `id`, `county_id`, `shape`, and `likely_vacant`. The base tables include `parcels`, `parcel_sales`, `job_attempts`, `quarantined_records`, `soil_map_units`, `soil_polygons`, `soil_polygon_parcels`, `soil_screening_runs`, `soil_screening_heads`, `soil_screening_members`, `soil_screening_ratings`, `soil_polygon_sets`, `soil_polygon_set_members`, `soil_polygon_set_heads`, `soil_interpretation_keys`, `soil_screening_refresh_status`, `soil_assessment_keys`, `soil_parcel_assessments`, `soil_assessment_map_units`, `soil_component_facts`, `soil_measurements`, `goose_db_version`, and `spatial_ref_sys`.
- **Seed.** Run `docker exec -i "$VERIFY_CONTAINER" psql "$GOOSE_DATABASE_URL" -v ON_ERROR_STOP=1 < .cursor/skills/verify-landman/seed.sql`. Exit code `0`. A follow-up count of `source_parcel_id = 'verify-sentinel'` is `1`.
- **Proof.** Save the `migrate-up` output, the `migrate-version` output, the `\d parcels` output, the table listing, and the sentinel count under `artifacts/verify/$RUN_ID/`.

## Gotchas

- `pg_isready` can pass while the `landman` database is missing or while migrations have not run. Healthcheck success is not readiness for this feature.
- Goose `localhost:5432` is Postgres inside `landman-verify-postgis`. The host port `55432` is for `./landman` only.
- `make db-drop` always drops the database named `landman` on `DB_CONTAINER`. Without the command-line `DB_CONTAINER`, Make uses `.env` and can drop the operator database.
- `make db-create` does not pass `ON_ERROR_STOP`. A SQL error in the bootstrap script can still exit 0. Treat a later read of the `landman` database as the proof that create worked.
- The bootstrap SQL creates a database named `landman`. It cannot create a different name. Isolation is the container, not a second database name.
- Host port `5432` in `docker-compose.yml` is fixed. `POSTGRES_PORT` in `.env` does not change that mapping. The README sentence that says to update the URLs after changing `POSTGRES_PORT` describes a port Compose does not publish.
- `parcel_shape_parts` is a temporary table dropped on commit. A schema check after bootstrap, and a schema check after import, must not require it.
- `parcel_sales` exists after migrate. `./landman` does not call `SaveSales`. Do not seed sales and do not expect sales rows.
- `job_attempts` and `quarantined_records` exist after migrate. `./landman` writes `job_attempts` for `union.gis.parcels` and `union.soil.screening`. It writes `quarantined_records` only when a record is quarantined. There is no separate operator command for the job runner.
- Goose migrations that have already been applied are not replayed if you edit an old file. New behavior belongs in a new numbered file.
- `make migrate-create` writes a new SQL file. It is not a verification drive.
- `make migrate-down` rolls back one migration on `DATABASE_URL`. Reset the verify database with `make db-drop` and `make db-create` on `landman-verify-postgis`, not with `migrate-down`.
