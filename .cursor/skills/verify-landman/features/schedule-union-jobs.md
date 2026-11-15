# Schedule Union jobs

`./landman schedule` waits until `UNION_PARCELS_CRON` and `UNION_SOIL_CRON` are due. It does not import at startup. Each due source then runs, and a clean source run screens the stored soil set. There is no screening cron.

## Sub-features

- `schedule-env` rejects a start that lacks either cron variable.
- `schedule-wait` stays running until the next due time and prints no parcel progress.

## How to get to it (user POV)

- From the repo root, run `make build`.
- Set `DATABASE_URL` to the verify host URL in `SKILL.md`.
- Set `UNION_PARCELS_CRON` and `UNION_SOIL_CRON`. Each value is `CRON_TZ=<zone>` plus five calendar fields.
- Run `./landman schedule`. Stop that PID with SIGTERM.

## Driving it with make and the landman binary

Preconditions:

- The verify database exists and `make migrate-up` has reached version 16.
- `./landman` exists from `make build`.
- No other `./landman` process is running.

- Run `./landman schedule` with `UNION_PARCELS_CRON` and `UNION_SOIL_CRON` unset. Exit code 1. Stderr contains `UNION_PARCELS_CRON requires CRON_TZ and five calendar fields`. The process does not open the database.
- Run `DATABASE_URL="$HOST_DATABASE_URL" UNION_PARCELS_CRON='CRON_TZ=UTC 0 0 1 1 *' UNION_SOIL_CRON='CRON_TZ=UTC 0 0 1 1 *' ./landman schedule`. The process stays running. Stdout and stderr stay empty. No `[union_county_gis]` line appears. Send SIGTERM to that PID only. The exit code is 0.
- Save both transcripts under `artifacts/verify/$RUN_ID/`.

## Gotchas

- A cron value without `CRON_TZ` and five fields exits 1 before the database opens. The error names that variable.
- The sample calendars in `.env.example` can be due during a drive. Use a date that is not today, such as `CRON_TZ=UTC 0 0 1 1 *`, so the process does not fetch the county.
- SIGINT and SIGTERM cancel the scheduler. Kill that PID only. Do not `pkill landman`.
- A due tick uses the same ten minute deadline as a manual run and contacts the county. This wait check must not use a cron that is due now.
