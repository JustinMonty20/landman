# landman

Data gathering and sanitization aspect of NC county GIS data.

## Database migrations

This project uses Postgres/PostGIS and plain SQL migration files.

Local Makefile database commands use `docker exec` against your running PostGIS container, so you do not need local `psql` or a separate migration CLI image.

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

Because commands run with `docker exec` inside the DB container, `localhost` means the Postgres server inside that same container.

### Create the database

Schema migrations run inside an existing database, so create the `landman` database first by connecting to the default `postgres` maintenance database:

```sh
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

This creates matching `.up.sql` and `.down.sql` files in `db/migrations/`.

### Run migrations

Apply all pending migrations:

```sh
make migrate-up
```

Roll back the latest migration:

```sh
make migrate-down
```

Check applied migration versions:

```sh
make migrate-version
```
