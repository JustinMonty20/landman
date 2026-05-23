# Bootstrap landman database outside migrations

## Context
- User wants the setup to create a Postgres database named `landman`, not only create tables.
- Important Postgres/migration constraint: normal schema migrations run inside an existing database connection, so the migration that creates tables in `landman` cannot also create the `landman` database it is connected to.
- `CREATE DATABASE` must be run while connected to another database, usually the default `postgres` maintenance database.

## Approach
- Do not put `CREATE DATABASE landman` in `db/migrations/000001_init_schema.up.sql`; that migration should run against the already-created `landman` database.
- Add a bootstrap SQL script for creating the `landman` database using `psql`.
- Add a Makefile target that runs the bootstrap script against `POSTGRES_URL`.
- Document that normal migrations use `DATABASE_URL` pointing at `landman`, while bootstrap uses `POSTGRES_URL` pointing at `postgres`.

## Files to modify
- `Makefile`
- `README.md`
- `db/bootstrap/create_database.sql`

## Steps
- [x] Add `db/bootstrap/create_database.sql` using psql `\gexec` to create `landman` if it does not already exist.
- [x] Add Makefile target `db-create` that requires `POSTGRES_URL` and runs the bootstrap script.
- [x] Document bootstrap flow in `README.md`.
- [x] Run `go test ./...`.

## Verification
- `go test ./...`
- Optional local DB verification:
  - `export POSTGRES_URL='postgres://user:password@localhost:5432/postgres?sslmode=disable'`
  - `make db-create`
  - `export DATABASE_URL='postgres://user:password@localhost:5432/landman?sslmode=disable'`
  - `make migrate-up`
