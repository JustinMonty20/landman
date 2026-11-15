# Run Union jobs and configure independent schedules

## Manual commands

Build with `go build -o landman .`. Set `DATABASE_URL` to a PostGIS database migrated through `000015_job_pending.sql`.

These commands contact the live Union GIS or USDA services. Do not use an operator database for tests. `make run` loads `.env` and can use that database.

```sh
# Existing ordered import. Parcel source, soil source, then one derived refresh.
./landman

# Import only parcels, then refresh from stored soil. No county soil fetch.
./landman run union.gis.parcels

# Import only soil, then refresh. No county parcel fetch.
./landman run union.gis.soil

# Retry overlaps and required USDA screening without either county fetch.
./landman run union.soil.screening
```

Every source import and derived refresh uses `jobs.Runner.Run`. The default command keeps its ten-minute overall deadline. Each single-source command shares that deadline with its downstream refresh. A source error or non-clean outcome stops dependent work. A derived error fails the command but does not change an already successful source attempt.

A parcel-only run needs a stored soil set. If no set exists, the source can succeed while screening records `pending` and the command fails with the missing prerequisite. Run the soil source to ingest a set and refresh screening. A stored empty set is not a missing set.

Derived refresh uses immutable members of the current stored set. It neither records a new county soil fetch nor changes soil fetch timestamps. It still contacts USDA for required interpretations. `SOIL_SCREENING_MAX_AGE` sets the maximum age of published evidence and defaults to `720h`. It is not a fetch-skipping interval.

## Start the scheduler

Stop old executable versions before enabling the new scheduler. Use the same migrated database for all processes that must share exclusion and attempt history.

Set both independent calendar expressions explicitly. There are no default schedules.

```sh
export UNION_PARCELS_CRON='CRON_TZ=America/New_York 0 2 * * 0,3'
export UNION_SOIL_CRON='CRON_TZ=UTC 0 3 1 */3 *'
./landman schedule
```

The example imports parcels on Sunday and Wednesday at 02:00 New York time, and soil at 03:00 UTC on the first day of each calendar quarter. Change either environment variable independently, then restart the process. Each expression requires `CRON_TZ` and five calendar fields. Interval descriptors such as `@every` are not accepted by this command. Use UTC to avoid daylight-saving transitions.

Only the two source jobs have schedules. After either source succeeds cleanly, the executable runs `union.soil.screening`. It refreshes overlaps and screening from the current stored soil set. There is no independent screening cron.

The scheduler waits for the next due time on startup. It does not replay missed ticks or retry failures before the next due time. A tick overlapping the same source job is recorded as `skipped_overlap`. Each scheduled source and its downstream work share a ten-minute deadline. SIGINT or SIGTERM cancels active jobs and waits for workers to finish.

## Inspect incomplete work

```sql
SELECT job_id, outcome, processed_count, quarantined_count,
       started_at, finished_at, error_text
FROM job_attempts
WHERE job_id IN ('union.gis.parcels', 'union.gis.soil', 'union.soil.screening')
ORDER BY started_at DESC;
```

- `success` means that job finished without quarantine, not that every downstream job succeeded.
- `success_with_quarantine` means that the job reported invalid records. It does not trigger screening.
- `failed` means that an error stopped the job.
- `interrupted` means that cancellation or process abandonment stopped the attempt.
- `skipped_overlap` means that another process held the same job ID. It is not success.
- `pending` means that a prerequisite is missing, currently a stored soil set for screening.

Concurrent successful sources wait for a cross-process refresh lock before claiming the screening job. A waiting refresh loads the set after acquiring the lock. If the lock session closes during screening, a later derived attempt retries an overlap skip until the running attempt ends or the deadline expires. Existing set-head checks and parcel SHARE locks prevent publication against replaced soil membership or an inconsistent parcel snapshot. Parcel writes can wait during assessment publication. Reads remain available.

Required USDA or database failure preserves the prior screening head. Derived overlap updates and screening publication commit together. Historical polygon and legacy overlap rows remain in the database. Refresh writes only current-set overlaps; screening never treats old legacy joins as current evidence.

Scheduler errors and non-clean outcomes appear in process logs. A failure while connecting or waiting for refresh exclusion occurs before the derived attempt starts, so inspect logs as well as history. There is no persistent dispatch queue. After a crash between source commit and derived completion, use the manual screening command or wait for the next successful source run. A new source run starts its scan from the beginning.

Offset paging and upserts do not prove a complete remote snapshot. Absent parcels are not deleted. See [the independent-schedule plan](../plans/union-independent-schedules.md) for the live gate and remaining verification.

## Add another ingestion entry point

Define a stable ID and a `jobs.Definition`. Invoke it through `jobs.Runner.Run`, directly or through scheduler composition. Keep source fetching, ordering, and transaction rules inside the definition or its explicit ordered caller. Do not call Union ingestion directly from an executable or scheduler callback.

Future Union tax work remains one ordered tax job with per-file freshness checks. Calendar scheduling does not replace its 180-day freshness rule. Read [the source-job plan](../plans/source-jobs-runner-scheduler.md) and [the tax plan](../plans/tax-data-ingest.md) before wiring that source.
