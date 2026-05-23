# Postgres SQL migrations with golang-migrate

## Context
- User wants to set up database migration scripts in this Go project.
- User confirmed the target database is Postgres and prefers plain/raw SQL migrations run from the CLI, not app-startup migrations.
- Repository is a Go module (`github.com/JustinMonty20/landman`) with no current application database layer in code.
- `Makefile` already references `golang-migrate/migrate` targets (`migrate-up`, `migrate-down`) and a conventional `./db/migrations` path, but the repo does not appear to have migration files yet.
- `docs/ARCHITECTURE.md` already recommends PostgreSQL/PostGIS and migration tooling with versioned up/down migrations.

## Approach
- Use `golang-migrate/migrate` as the migration tool because it is common in Go, supports plain SQL up/down files, supports Postgres well, and the current `Makefile` already partially assumes it.
- Keep migrations outside application startup and run them via CLI/Makefile.
- Add a `db/migrations` directory with an initial SQL migration pair.
- Update Makefile targets to read the Postgres connection string from `DATABASE_URL` instead of hardcoding credentials.
- Document install/setup and common commands in the README.

## Files to modify
- `Makefile`
- `README.md`
- `db/migrations/000001_init_schema.up.sql`
- `db/migrations/000001_init_schema.down.sql`
- Optional: `.env.example` if the project wants a shared local `DATABASE_URL` template.

## Reuse
- Existing `Makefile` migration targets:
  - `migrate-up`
  - `migrate-down`
  - `deps` already mentions `github.com/golang-migrate/migrate/v4` and Postgres/file drivers.
- Existing architecture guidance in `docs/ARCHITECTURE.md`:
  - PostgreSQL/PostGIS is the intended database direction.
  - Keep migrations in version control.
  - Support up/down migrations.
- Recommended migration file naming convention from `golang-migrate`:
  - `{version}_{name}.up.sql`
  - `{version}_{name}.down.sql`

## Steps
- [x] Keep `golang-migrate/migrate` as the selected tool; do not introduce goose, Atlas, or ORM auto-migrations.
- [x] Create `db/migrations/`.
- [x] Add `000001_init_schema.up.sql` with the initial Postgres schema. If PostGIS geometry columns are needed in the first schema, include `CREATE EXTENSION IF NOT EXISTS postgis;`.
- [x] Add `000001_init_schema.down.sql` that cleanly reverses the initial schema in dependency-safe order.
- [x] Update `Makefile` migration targets to use `$(DATABASE_URL)` and fail with a helpful message when it is not set.
- [x] Add Makefile helpers for common CLI operations, such as creating a migration and checking migration version, if desired.
- [x] Document local usage in `README.md`, including installing the `migrate` CLI, setting `DATABASE_URL`, creating migrations, running up, rolling back one migration, and checking version.

## Verification
- Install or confirm the `migrate` CLI is available: `migrate -version`.
- Start a local Postgres database and set `DATABASE_URL`, for example: `postgres://user:password@localhost:5432/parcel_db?sslmode=disable`.
- Run `make migrate-up` and confirm the schema/tables are created.
- Run `migrate -path=./db/migrations -database "$DATABASE_URL" version` or a Makefile wrapper to confirm the applied version.
- Run `make migrate-down` and confirm rollback works.
- Run `go test ./...` to ensure the migration setup did not affect existing Go tests.
