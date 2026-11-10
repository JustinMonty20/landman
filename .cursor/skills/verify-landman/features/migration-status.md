# Check migration status

After the local database exists, an operator can ask goose which version is applied and which migration files are pending or done.

## Sub-features

- `migrate-version` prints the current goose version.
- `migrate-status` lists each migration file and whether it is applied.

## How to get to it (user POV)

- Run `make migrate-version`.
- Run `make migrate-status`.

## Driving it with make and the landman binary

Preconditions:

- Database doctor passed: `landman-postgis` is running and healthy, and `DATABASE_URL` is set.
- `make db-create` has been run at least once so the `landman` database exists.
- This is a read of goose state. It does not apply or roll back migrations.

- **Show version.** Run `make migrate-version`. Exit code `0`. Output contains `goose: version <n>`. After a complete `make migrate-up`, `<n>` is the integer value of the highest filename prefix in `db/migrations/` (prefix `000003` prints as `3`).
- **Show status.** Run `make migrate-status`. Exit code `0`. Every `db/migrations/*.sql` file is listed. After a complete `make migrate-up`, none of those files are pending.
- **Proof.** Save both command transcripts under `artifacts/verify/$RUN_ID/`. The version and the status listing must agree with each other and with the files on disk. Do not invent a pass from `SELECT 1` alone.

## Gotchas

- These targets need the container name `landman-postgis`. A Compose service that came up as `postgis` will fail the Makefile's container check.
- `migrate-status` before `db-create` talks to `DATABASE_URL`'s `landman` database and will fail if that database does not exist yet.
- Pending files after `migrate-up` mean bootstrap is incomplete. Do not report this feature verified until a follow-up `make migrate-up` leaves no pending landman migrations, or record the failure and the pending names.
