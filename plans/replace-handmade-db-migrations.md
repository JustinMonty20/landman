# Replace handmade database migration tool with goose

## Context
- The current migration flow is handmade in the `Makefile`: it shells into a Postgres/PostGIS Docker container with `psql`, creates a custom `schema_migrations` table, scans `db/migrations/*.up.sql`, and manually records applied versions.
- Existing migrations are plain SQL split into `.up.sql` / `.down.sql` pairs under `db/migrations/`.
- The project is Go-based and currently has no database migration dependency in `go.mod`.
- Goal: move to the simplest practical Go ecosystem database migration tool and remove the custom migration runner logic.

## Approach
- Recommended tool: `pressly/goose` CLI, using plain SQL migrations.
- Keep migrations as developer/ops commands, not application-startup migrations.
- Run goose from inside Docker/container tooling rather than requiring a host-installed goose binary.
- Convert existing migration pairs to goose-compatible SQL migration files with `-- +goose Up` / `-- +goose Down` sections.
- Before converting/rerunning migrations locally, use the existing handmade tool to run down migrations, then drop and recreate the database so goose starts from a clean database state.
- Update `Makefile` migration targets to call goose inside Docker instead of manually running `psql` loops.
- Keep `db/bootstrap/create_database.sql` and `make db-create` for creating the database, because normal migration tools connect to an existing database.
- Document how to run goose through Docker and how to reset an already-migrated local database.

## Files to modify
- `Makefile`
- `README.md`
- `db/migrations/000001_init_schema.sql` (new goose-compatible replacement for current pair)
- `db/migrations/000001_init_schema.up.sql` / `db/migrations/000001_init_schema.down.sql` (remove after conversion)
- Optional: `db/bootstrap/drop_database.sql` or a Makefile inline `psql` command for dropping the local `landman` database during reset.
- Optional: `.env.example` if we want a checked-in local config template.

## Reuse
- Existing SQL schema in `db/migrations/000001_init_schema.up.sql`.
- Existing rollback SQL in `db/migrations/000001_init_schema.down.sql`.
- Existing `DATABASE_URL`, `POSTGRES_URL`, `DB_CONTAINER`, `MIGRATIONS_PATH`, and `db-create` Makefile setup.
- Existing documentation section in `README.md` for database setup.
- Existing architecture guidance in `docs/ARCHITECTURE.md` recommending versioned up/down migrations.

## Steps
- [x] Confirm migration tool choice as `goose`, run from Docker/container tooling.
- [x] Add Makefile variables for goose Docker execution, for example a `GOOSE_IMAGE` plus a reusable command that mounts `db/migrations` and passes `DATABASE_URL` to goose with the Postgres driver.
- [x] Before replacing the handmade targets, reset the already-migrated local database using the current tool flow:
  - run the existing `make migrate-down` until all handmade migrations are rolled back;
  - drop the `landman` database while connected to `POSTGRES_URL`/the default `postgres` database;
  - recreate it with `make db-create`.
- [x] Add a documented reset helper if desired, e.g. `make db-drop`, that terminates active connections and drops only the local `landman` database.
- [x] Replace custom `migrate-up`, `migrate-down`, and `migrate-version` recipes with Dockerized goose CLI calls.
- [x] Add `migrate-status` using Dockerized `goose status`.
- [x] Replace custom `migrate-create` recipe with Dockerized `goose -dir /migrations create $(name) sql`, ensuring created files land in `db/migrations`.
- [x] Convert `000001_init_schema.up.sql` and `000001_init_schema.down.sql` into a single goose SQL migration file.
- [x] Remove the old split migration files after conversion so the migration directory only contains goose-formatted files.
- [x] Update README database migration docs for Dockerized goose commands, create/up/down/status/version commands, and the local reset flow for databases already migrated by the handmade tool.
- [x] Optional: add `.env.example` because README references it but the repo currently does not contain one.

## Verification
- Confirm Docker can run the selected goose image/command, e.g. `make migrate-version` or `make migrate-status`.
- Using the current handmade targets before implementation, run down migrations and drop the already-migrated local database.
- Recreate the database with `make db-create`.
- On the clean database: `make migrate-up`, inspect `goose_db_version`, confirm `parcels` and `parcel_sales` exist.
- `make migrate-version` or `make migrate-status` shows version `000001` applied.
- `make migrate-down` removes the schema and records rollback in goose history.
- `go test ./...`
