# Run Union County jobs and change their schedules

A job is a named import function. The runner records each attempt in `job_attempts` and prevents two processes from running the same job ID at the same time. The scheduler is a Go component that calls the runner at set times.

## Current executable behavior

`./landman` runs three jobs in order. It does not start the scheduler. Each start of `./landman` makes one attempt at each job. A second process that uses the same job ID while the first runs records `skipped_overlap`. The second process stops before it starts a dependent job.

1. `union.gis.parcels` imports parcel pages and commits parcel shapes and sales.
2. `union.gis.soil` fetches soil polygons, stores the soil set, and computes parcel overlaps.
3. `union.soil.screening` reads that committed soil set and publishes soil assessments.

The caller starts the next job only when the previous job has outcome `success`. An error or any other outcome stops the sequence. The whole command has a 10-minute context deadline. The three jobs do not share one database transaction. If a later job fails, an earlier job can have committed data. A new command starts the parcel scan again.

To run the import manually, set `DATABASE_URL` to a migrated PostGIS database and run `./landman`. `make run` loads `.env` and can use the operator database. Use a separate database for tests. The program contacts the live Union GIS and USDA services. There is no partial-import flag.

Inspect attempt status with SQL after a run:

```sql
SELECT job_id, outcome, processed_count, quarantined_count, started_at, finished_at, error_text
FROM job_attempts
WHERE job_id IN ('union.gis.parcels', 'union.gis.soil', 'union.soil.screening')
ORDER BY started_at DESC;
```

`success` means the job finished without quarantine. `success_with_quarantine` means the job reported invalid records. `failed` means an error stopped the job. `interrupted` means a canceled or abandoned attempt. `skipped_overlap` means another process held the same job ID. These outcomes do not prove that a remote offset-paged source is a complete snapshot. An import does not delete parcels that are absent from a later source response.

## Change a schedule after scheduler wiring exists

`internal/jobs/scheduler.go` provides `NewScheduler`, `Start`, `Stop`, and manual `Run(ctx, jobID)`. `main.go` does not call these methods. There are no active cron jobs or schedule values to edit today. To add automatic runs, add code-defined job definitions in the executable and start a long-running scheduler process. Keep the three Union stages in their required order. Do not schedule each stage independently. A time offset between independent schedules does not prove that parcel import succeeded before soil joins or that screening uses its committed set.

Set a schedule on the *ordered Union import*, not on three independent stages. When a scheduler definition invokes the ordered import, give that definition a stable ID. Keep the stage IDs stable as well. Do not use the same ID for two independently active jobs. Update the schedule string in that definition, test it, deploy the new executable, and restart the scheduler process. A changed Go string cannot change a process that is already running.

A calendar expression has `CRON_TZ=<timezone>` followed by five fields: minute, hour, day of month, month, and day of week. `NewScheduler` rejects a definition without `CRON_TZ`, a malformed expression, and a repeated job ID. Examples of expressions for a future ordered import definition follow:

```text
CRON_TZ=America/New_York 0 2 * * 0      Sunday at 02:00 local time
CRON_TZ=America/New_York 0 2 * * 0,3    Sunday and Wednesday at 02:00 local time
CRON_TZ=UTC 0 3 1 */3 *                First day of each third month at 03:00 UTC
```

Use `UTC` if local daylight-saving time must not change the time of a run. The scheduler starts at the next due time. It does not run immediately and does not replay times that passed while the process was stopped. A due run that overlaps a run with the same job ID is skipped and recorded. The scheduler does not retry a failed job automatically before its next due time. `@every` is accepted by the cron library, but an interval is relative to process start. Do not use it to express a calendar deadline or a 180-day source freshness rule.

For future Union tax data, keep the four file feeds in one ordered tax job. Preserve each file's freshness test. A calendar cron expression is not the same as 180 days of freshness. See `plans/source-jobs-runner-scheduler.md` and `plans/tax-data-ingest.md` before you add that job.

## Add a Union ingestion entry point

Put each new Union County ingestion path in a `jobs.Definition` with a stable ID and a `Run` function. Call the definition only through `jobs.Runner.Run`, or through `jobs.Scheduler`, which calls the same runner. Keep network fetches, transaction limits, and source rules in the job function. Check the outcome before you start work that depends on that job. Do not call a Union importer directly from `main`, a command, or a scheduled callback. `AGENTS.md` requires this rule for future changes.
