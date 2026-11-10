# landman verification map

This directory is the maintained source for verifying the user-facing behavior of landman. Read the index before driving the app, then use the matching feature file as the recipe.

## Baseline preconditions

- Build the CLI with `make build` so `./landman` exists at the repo root.
- Run the CLI doctor in `.cursor/skills/verify-landman/SKILL.md` before a stream drive.
- A stream drive needs outbound HTTPS to `atlas.unioncountync.gov`. It does not need Postgres.
- Database features need Docker, a `.env` copied from `.env.example` if missing, and a healthy `landman-postgis` container. Run the database doctor before those drives.
- The default PostGIS service is a shared singleton (container `landman-postgis`, host port `5432`, volume `landman-postgis-data`). Do not start a second instance. Do not `make db-drop` or `make migrate-down` on a database this run did not create.
- Do not drive a `./landman` process or a PostGIS container this run did not start, except for additive database checks (`make db-create`, `make migrate-up`, read-only `psql` / `migrate-version`) against an already-healthy instance after doctor passes.
- Put evidence in `artifacts/verify/$RUN_ID/` and leave it there after cleanup.

## Driving conventions

- Start every recipe from the baseline state unless its preconditions say otherwise.
- Treat every command as literal. Keep quoted names and flags unchanged.
- Drive the CLI as `./landman` (or `make run`). Drive the database with `make` and `docker exec landman-postgis psql`.
- Restore nothing after a stream drive; it has no local writable state besides process output.
- Do not remove proof artifacts during cleanup.

## Proof and skip reporting

- Capture the user action and the resulting state, not only the last line.
- CLI proof includes the command, stdout, stderr, and exit code.
- Database mutation proof includes a later read (`make migrate-version`, `make migrate-status`, or `\d parcels`).
- Record the feature ID and entry point used with every artifact.
- Report an unreachable path with the attempted command and the unmet precondition.
- Do not report a skipped entry point as verified through a different path.
- Do not report `go test` as proof of a feature in this map.

## Feature entry contract

Each feature file starts with an H1 title and one paragraph describing the user-visible behavior. It then uses exactly four H2 sections in this order.

1. `Sub-features` lists short IDs with one line for each behavior.
2. `How to get to it (user POV)` lists every user entry point.
3. `Driving it with make and the landman binary` starts with `Preconditions:` and uses labeled bullets that pair each user action with an exact command and observable result.
4. `Gotchas` lists traps that can waste or invalidate a verification run.

Keep implementation details out of the map. Name only user paths, stable handles, required state, commands, and observable proof.

## Features

- [Stream vacant parcel IDs](./stream-vacant-parcel-ids.md) covers building the CLI and streaming likely-vacant Union County parcel IDs from the live GIS layer.
- [Bootstrap the local database](./local-database-bootstrap.md) covers starting PostGIS, creating the `landman` database, and applying goose migrations.
- [Check migration status](./migration-status.md) covers `make migrate-version` and `make migrate-status` after the database exists.
