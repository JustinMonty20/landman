# Check migration status

After the verify database exists, goose can show the applied version and whether each migration file is pending or done.

## Sub-features

- `migrate-version` prints the current goose version.
- `migrate-status` lists each migration file and whether it is applied.

## How to get to it (user POV)

- Run `make migrate-version` with the verify `DB_CONTAINER` and goose `DATABASE_URL` from `SKILL.md`.
- Run `make migrate-status` with those same overrides.

## Driving it with make and the landman binary

Preconditions:

- Database doctor passed for `landman-verify-postgis`.
- `make db-create` has been run on that container, so the `landman` database exists.
- This is a read of goose state. It does not apply or roll back migrations.

- Run `make migrate-version DB_CONTAINER="$VERIFY_CONTAINER" DATABASE_URL="$GOOSE_DATABASE_URL"`. Exit code 0. Stderr contains `goose: version ` and the integer of the highest filename prefix in `db/migrations/` after a complete `make migrate-up`. A log timestamp precedes that line. Stdout is the Make recipe. The highest file is `000016_job_attempt_kind.sql`. The version integer is 16.
- Run `make migrate-status` with the same overrides. Exit code 0. Stderr names every `db/migrations/*.sql` file. After a complete `make migrate-up`, stderr does not contain `Pending`. Stdout is the Make recipe.
- Save both command transcripts under `artifacts/verify/$RUN_ID/`. The version integer and the status listing must agree with each other and with the files on disk. Do not invent a pass from `SELECT 1` alone.

## Gotchas

- These targets need `DB_CONTAINER=landman-verify-postgis` on the command line. A value from `.env` points them at the operator container.
- `migrate-version` and `migrate-status` before `db-create` open the `landman` database and fail when that database does not exist yet.
- Pending files after `migrate-up` mean bootstrap is incomplete. Do not report this feature verified until a follow-up `make migrate-up` leaves no pending landman migrations, or record the failure and the pending names.
- Make prints the recipe on stdout because the Makefile lines are not silenced. Goose writes the version line and the status table on stderr, with a log timestamp in front of each line. Save both streams.
