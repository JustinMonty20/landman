# Schedule Union jobs
`./landman schedule` waits until parcel, soil, zoning, road, and qualification calendars are due. Startup registers the calendars without a qualification assessment or county fetch. Work starts only when a calendar is due. A due source runs through the job runner. Soil success starts stored screening; parcel success starts it when due. Qualification runs on its own calendar. There is no screening calendar.

## Sub-features
- `schedule-env` rejects start when any required calendar is absent or invalid.
- `schedule-wait` stays running until a due time and does not fetch county data before then.

## How to get to it (user POV)
- At the repository root, run `verify_make build`.
- Set `DATABASE_URL` to the verify host URL in `live-gis.md`, or use the isolated fixture database in `verify-landman/database.md`.
- Set `UNION_PARCELS_CRON`, `UNION_SOIL_CRON`, `UNION_ZONING_CRON`, `UNION_ROADS_CRON`, and `UNION_QUALIFICATION_CRON`. Each value is `CRON_TZ=<zone>` plus five calendar fields.
- Run `./landman schedule`. Stop only that process with `SIGTERM`.

## Driving make and landman binary
Preconditions:
- The verify database exists and all current migrations have been applied.
- `./landman` exists after `verify_make build`.
- No other `./landman` process runs.
- Run `./landman schedule` with the five calendar variables unset. It exits 1. Stderr contains `UNION_PARCELS_CRON requires CRON_TZ and five calendar fields`. The process does not open the database.
- Set the first four calendar variables to `CRON_TZ=UTC 0 0 1 1 *`. Leave `UNION_QUALIFICATION_CRON` unset. Run `./landman schedule`. It exits 1 and stderr contains `UNION_QUALIFICATION_CRON requires CRON_TZ and five calendar fields`.
- Set all five calendar variables to `CRON_TZ=UTC 0 0 1 1 *`. Run `./landman schedule`. The process stays running. It does not write `[union_county_gis]`. Send `SIGTERM` only to that process. It exits 0.
- Save each transcript under `artifacts/verify/$RUN_ID/`.

## Gotchas
- A calendar without `CRON_TZ` and five fields exits 1 before the database opens. The error names the variable.
- A calendar in `.env.example` can be due during a drive. Use a date that is not today, such as `CRON_TZ=UTC 0 0 1 1 *`, so the process does not fetch county data.
- `SIGINT` and `SIGTERM` cancel the scheduler. Kill only that PID. Do not use `pkill landman`.
- Due source ticks can contact the county. Scheduled source attempts have a ten-minute inner deadline; qualification assessment has one hour. Derived screening has its own ten-minute context. Do not infer the standalone command's outer deadline from these inner contexts. Never use a calendar due now for this wait check.
