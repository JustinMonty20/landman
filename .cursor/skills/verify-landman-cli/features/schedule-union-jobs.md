# Schedule Union jobs
`./landman schedule` waits until parcel, soil, zoning, road, and qualification calendars are due. At startup it reconciles stored qualification data without a county fetch. It does not import source data until a calendar is due. A due source runs through the job runner. A soil run screens stored soil. A parcel run screens stored soil only when that screen is due. Both reconcile qualification data. There is no screening calendar.

## Sub-features
- `schedule-env` rejects start when any required calendar is absent or invalid.
- `schedule-wait` stays running until a due time and does not fetch county data before then.

## How to get to it (user POV)
- At the repository root, run `make build`.
- Set `DATABASE_URL` to the verify host URL in `live-gis.md`.
- Set `UNION_PARCELS_CRON`, `UNION_SOIL_CRON`, `UNION_ZONING_CRON`, `UNION_ROADS_CRON`, and `UNION_QUALIFICATION_CRON`. Each value is `CRON_TZ=<zone>` plus five calendar fields.
- Run `./landman schedule`. Stop only that process with `SIGTERM`.

## Driving it with make and the landman binary
Preconditions:
- The verify database exists and all current migrations have been applied.
- `./landman` exists after `make build`.
- No other `./landman` process runs.
- Run `./landman schedule` with the five calendar variables unset. It exits 1. Stderr contains `UNION_PARCELS_CRON requires CRON_TZ and five calendar fields`. The process does not open the database.
- Set the first four calendar variables to `CRON_TZ=UTC 0 0 1 1 *`. Leave `UNION_QUALIFICATION_CRON` unset. Run `./landman schedule`. It exits 1 and stderr contains `UNION_QUALIFICATION_CRON requires CRON_TZ and five calendar fields`.
- Set all five calendar variables to `CRON_TZ=UTC 0 0 1 1 *`. Run `./landman schedule`. The process stays running. Startup stderr contains `[union.qualification.assess]` and `No active generation; skip` when no zoning or roads head is stored. It does not write `[union_county_gis]`. Send `SIGTERM` only to that process. It exits 0.
- Save each transcript under `artifacts/verify/$RUN_ID/`.

## Gotchas
- A calendar without `CRON_TZ` and five fields exits 1 before the database opens. The error names the variable.
- A calendar in `.env.example` can be due during a drive. Use a date that is not today, such as `CRON_TZ=UTC 0 0 1 1 *`, so the process does not fetch county data.
- `SIGINT` and `SIGTERM` cancel the scheduler. Kill only that PID. Do not use `pkill landman`.
- A due parcel, soil, zoning, or road tick contacts the county under a ten-minute limit. Its downstream screening or qualification gets a separate ten-minute limit. A due qualification tick stays at ten minutes and only reconciles stored rows. Do not use a calendar due now for this wait check.
