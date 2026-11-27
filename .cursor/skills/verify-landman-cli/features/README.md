# landman verification map

These are extended CLI recipes. Use the small local parcel checks in `../SKILL.md` by default. Read this index when an extended local or live journey is selected. The live journey sequences features; it is not a default check.

## Baseline preconditions

- Define `verify_make` from `../SKILL.md`. Build the CLI with `verify_make build` so `./landman` exists at the repo root.
- Check container identity and database readiness before every drive. Use the doctors in `../live-gis.md` for the captured-seed journey. Local bootstrap, migration reads, and scheduler waiting do not need a source probe.
- Every database command uses container `landman-verify-postgis`, goose URL `GOOSE_DATABASE_URL`, and host URL `HOST_DATABASE_URL` from `live-gis.md`.
- Leave any container named `landman-postgis` untouched. Do not `docker compose` and do not source `.env`.
- The optional GIS import needs separate approval for outbound HTTPS to `atlas.unioncountync.gov` and the USDA Soil Data Access tabular endpoint. Without approval, report `probe`, the GIS source commands, and `classify` as not run. The classifier can request live sale evidence.
- Put evidence in `artifacts/verify/$RUN_ID/` and leave it there after cleanup.

## Driving conventions

- Start every recipe from the verify database launch unless its preconditions say otherwise.
- Treat every command as literal. Keep quoted names and flags unchanged.
- Use the explicit source commands in the GIS journey. Use the tax skill for the combined bare command. Drive the database with verify-only `make` overrides and `docker exec`.
- Run `check-schema`, `check-loaded`, `probe`, and `classify` through `go run ./.cursor/skills/verify-landman-cli/cmd/verify-landman`.
- Do not remove proof artifacts during cleanup.

## Proof and skip reporting

- Capture the user action and the resulting state, not only the last line.
- CLI proof includes the command, stdout, stderr, and exit code.
- Database mutation proof includes a later read (`verify_make migrate-version`, `verify_make migrate-status`, or `psql`).
- Record the feature ID and entry point used with every artifact.
- Report an unreachable path with the attempted command and the unmet precondition.
- Do not report a skipped entry point as verified through a different path.
- Do not report `go test` as proof of a feature in this map.

## Feature entry contract

Each feature file except the linked journey starts with an H1 title and a user-visible description. It then uses four H2 sections in this order. The journey is an ordered list that links those features.

1. `Sub-features` lists short IDs with one line for each behavior.
2. `How to get to it (user POV)` lists every user entry point.
3. `Driving it with make and the landman binary` starts with `Preconditions:` and uses a bullet for each user action, with the command and the observable result.
4. `Gotchas` lists traps that can waste or invalidate a verification run.

Keep implementation details out of the map. Name only user paths, stable handles, required state, commands, and observable proof.

## Database

- [Bootstrap the verify database](./local-database-bootstrap.md) covers the verify container, the `landman` database, goose migrations, and the captured Union County sample.
- [Check migration status](./migration-status.md) covers `verify_make migrate-version` and `verify_make migrate-status` after that database exists.

## CLI import

- [Import parcels and soil](./import-parcels-and-soil.md) covers separate `./landman run` source commands, stored screening, and `./landman reprocess`. The bare combined command belongs to the tax skill.
- [Schedule Union jobs](./schedule-union-jobs.md) covers `./landman schedule` and all five cron variables. A waiting check does not prove due source dispatch.
- [Qualification checks](./qualification.md) cover source publication, spatial evidence, refresh failures, and optional live zoning and road jobs.

## Journey

- [Import journey](./import-journey.md) runs reset, migrate, `check-schema`, seed, `check-loaded`, `probe`, import, and `classify` in that order.
