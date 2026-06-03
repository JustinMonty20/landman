# landman

Data gathering and sanitization aspect of NC county GIS data.

## Database migrations

This project uses Postgres/PostGIS and plain SQL migrations managed by [`goose`](https://github.com/pressly/goose).

Local Makefile migration commands run goose from a Docker container, so you do not need to install goose locally. The goose container shares the network namespace of your running PostGIS container, so `localhost` in `DATABASE_URL` points at Postgres inside that DB container.

Migration files live in:

```text
db/migrations/
```

### Prerequisite

Start your local PostGIS container, for example with image:

```text
postgis/postgis:18-3.6-alpine
```

### Configure local database URLs

The Makefile automatically loads `.env` when present. Start from the example file:

```sh
cp .env.example .env
```

Then edit `.env` for your container name and local Postgres credentials:

```env
DB_CONTAINER=landman-postgis
POSTGRES_URL=postgres://user:password@localhost:5432/postgres?sslmode=disable
DATABASE_URL=postgres://user:password@localhost:5432/landman?sslmode=disable
```

### Create, drop, or reset the local database

Schema migrations run inside an existing database, so create the `landman` database first by connecting to the default `postgres` maintenance database:

```sh
make db-create
```

Drop only the local `landman` database:

```sh
make db-drop
```

If your local database was already migrated with the old handmade migration tool, reset it before using goose:

```sh
# Before switching to goose targets, roll back old handmade migrations.
make migrate-down
make migrate-down # repeat until it says no applied migrations remain

# Then drop and recreate the local database.
make db-drop
make db-create
```

The bootstrap script is idempotent and lives at:

```text
db/bootstrap/create_database.sql
```

### Create a migration

```sh
make migrate-create name=add_new_table
```

This creates a goose-formatted `.sql` file in `db/migrations/`.

### Run migrations

Apply all pending migrations:

```sh
make migrate-up
```

Roll back the latest migration:

```sh
make migrate-down
```

Check the current migration version:

```sh
make migrate-version
```

Check full migration status:

```sh
make migrate-status
```
