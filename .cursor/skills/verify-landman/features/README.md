# landman verification map

This directory is the maintained source for verifying the user-facing behavior of landman. Read the index before driving the app, then use the matching feature file as the recipe. The journey sequences those features. It is not a feature file, and it does not use the four-section feature contract.

## Baseline preconditions

- Build the CLI with `make build` so `./landman` exists at the repo root.
- Run the doctors in `.cursor/skills/verify-landman/SKILL.md` before a drive.
- Every database command uses container `landman-verify-postgis`, goose URL `GOOSE_DATABASE_URL`, and host URL `HOST_DATABASE_URL` from `SKILL.md`.
- Leave any container named `landman-postgis` untouched. Do not `docker compose` and do not source `.env`.
- An import needs outbound HTTPS to `atlas.unioncountync.gov` and to the USDA Soil Data Access tabular endpoint.
- Put evidence in `artifacts/verify/$RUN_ID/` and leave it there after cleanup.

## Driving conventions

- Start every recipe from the verify database launch unless its preconditions say otherwise.
- Treat every command as literal. Keep quoted names and flags unchanged.
- Drive the CLI as `DATABASE_URL="$HOST_DATABASE_URL" ./landman`. Drive the database with `make` overrides and `docker exec`.
- Run `check-schema`, `check-loaded`, `probe`, and `classify` through `go run ./.cursor/skills/verify-landman/cmd/verify-landman`.
- Do not remove proof artifacts during cleanup.

## Proof and skip reporting

- Capture the user action and the resulting state, not only the last line.
- CLI proof includes the command, stdout, stderr, and exit code.
- Database mutation proof includes a later read (`make migrate-version`, `make migrate-status`, or `psql`).
- Record the feature ID and entry point used with every artifact.
- Report an unreachable path with the attempted command and the unmet precondition.
- Do not report a skipped entry point as verified through a different path.
- Do not report `go test` as proof of a feature in this map.

## Feature entry contract

Each feature file starts with an H1 title and one paragraph describing the user-visible behavior. It then uses exactly four H2 sections in this order.

1. `Sub-features` lists short IDs with one line for each behavior.
2. `How to get to it (user POV)` lists every user entry point.
3. `Driving it with make and the landman binary` starts with `Preconditions:` and uses a bullet for each user action, with the command and the observable result.
4. `Gotchas` lists traps that can waste or invalidate a verification run.

Keep implementation details out of the map. Name only user paths, stable handles, required state, commands, and observable proof.

## Database

- [Bootstrap the verify database](./local-database-bootstrap.md) covers the verify container, the `landman` database, goose migrations, and the captured Union County sample.
- [Check migration status](./migration-status.md) covers `make migrate-version` and `make migrate-status` after that database exists.

## CLI import

- [Import parcels and soil](./import-parcels-and-soil.md) covers `./landman` writing parcels, joining soil, and running screening against the verify database. It also covers `./landman run`.
- [Schedule Union jobs](./schedule-union-jobs.md) covers `./landman schedule` and the two cron variables.

## Journey

- [Import journey](./import-journey.md) runs reset, migrate, `check-schema`, seed, `check-loaded`, `probe`, import, and `classify` in that order.
