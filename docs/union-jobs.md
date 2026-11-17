# Run Union jobs and configure independent schedules

## Manual commands

Build with `go build -o landman ./cmd/landman`. Set `DATABASE_URL` to a PostGIS database migrated through `000018_tax_file_imports.sql`.

The parcel and soil commands contact the live Union GIS or USDA services. The tax job reads operator files; it does not download county files. Do not use an operator database for tests. `make run` loads `.env` and can use that database.

```sh
# Import parcels, then soil, then refresh derived screening, then import four tax files.
# Set the five tax inputs below before this command.
./landman

# Import only parcels, then refresh from stored soil. No county soil fetch.
./landman run union.gis.parcels

# Import only soil, then refresh. No county parcel fetch.
./landman run union.gis.soil

# Repair at most 25 pending parcel records from the current county source.
# A clean repair then refreshes derived evidence from stored soil.
./landman reprocess union.gis.parcels --limit 25

# Retry overlaps and required USDA screening without either county fetch.
./landman run union.soil.screening
```

Every source import and derived refresh uses `jobs.Runner.Run`. The default command keeps its ten-minute overall deadline. Each single-source command shares that deadline with its downstream refresh. A source error or non-clean outcome stops dependent work. A derived error fails the command but does not change an already successful source attempt.

A parcel-only run needs a stored soil set. If no set exists, the source can succeed while screening records `pending` and the command fails with the missing prerequisite. Run the soil source to ingest a set and refresh screening. A stored empty set is not a missing set.

Derived refresh uses immutable members of the current stored set. It neither records a new county soil fetch nor changes soil fetch timestamps. It still contacts USDA for required interpretations. `SOIL_SCREENING_MAX_AGE` sets the maximum age of published evidence and defaults to `720h`. It is not a fetch-skipping interval.

## Quarantine and explicit parcel repair

Missing or unreadable rings and invalid constrained numeric fields produce durable quarantine evidence keyed by `OBJECTID` and reason. Numeric validation rejects non-finite values and values that would overflow after column-scale rounding. Values that round to the stored maximum remain valid. Every part of that effective PID is excluded, including parts on earlier pages. Existing parcel fields, sales, UUID, and shape remain unchanged. Valid unrelated PIDs still publish. A missing PID uses the existing `OBJECTID` fallback and cannot be likely vacant.

Full scans stage normalized parts on disk and publish accepted PIDs through the existing parcel writer in one transaction. Fields and sale slots remain last-part-wins. Geometry uses all accepted parts. An indexed temporary identity table tracks every observed `OBJECTID`, including invalid records. Contradictory PID assignments for one `OBJECTID` fail the entire scan before publication. Repeated observations of the same `OBJECTID` and PID retain last-part-wins behavior. HTTP errors, ambiguous identity, malformed responses, SQL failures, failed dissolution, and quarantine-storage failure abort publication. Quarantine evidence already written survives that rollback. Payloads are sensitive JSON, limited to 64 KiB, and must not be copied into logs.

`reprocess union.gis.parcels --limit N` requires an integer from 1 through 100, checked before database access. It selects pending entries by oldest `last_seen_at`, then UUID. Repeated entries for one PID are coalesced. It refetches the current selected `OBJECTID`, validates its identity, and fetches every current part of its effective PID. Saved payloads supply identity only and are never replayed as parcel attributes.

One repair allows at most 200 HTTP requests and 10,000 returned features, including identity checks. All county fetching shares a two-minute deadline inside the ten-minute command deadline. Each response is limited to 32 MiB. Missing or changed identity, incomplete counts or pages, duplicate parts, and exhausted limits fail the repair. No partial repair is published. Reduce `--limit` after request-budget exhaustion. A single oversized PID requires a later full scan instead.

Full scans and repairs claim the same `union.gis.parcels` job ID. There is no repair cron. Repairs record `attempt_kind = 'reprocess'`; ordinary attempts default to `full_scan`. Migration 16 cannot be rolled back while repair history exists. Last clean full-import evidence requires both `outcome = 'success'` and `attempt_kind = 'full_scan'`.

Evidence resolves only with successful complete-PID publication, in the same transaction. Repair resolves only the selected quarantine UUIDs whose `last_seen_at` tokens are unchanged, even when complete geometry requires more parts or another reason exists for the same `OBJECTID`. A clean full scan still resolves all older evidence for recovered observed records. A later invalid observation replaces the evidence payload, increments its occurrence count, and clears `reprocessed_at`. A crash before publication leaves pending evidence and can be retried safely.

Clean repairs use the same downstream refresh as clean source imports, without another county soil fetch. A downstream error preserves the successful repair attempt and fails the command. Quarantined repairs do not trigger screening. Retry derived work with `run union.soil.screening` after a downstream failure.

Inspect pending identity and reason without exposing owner payloads:

```sql
SELECT id, raw_record_key, reason, occurrence_count, first_seen_at, last_seen_at
FROM quarantined_records
WHERE job_id = 'union.gis.parcels' AND reprocessed_at IS NULL
ORDER BY last_seen_at, id;

SELECT finished_at
FROM job_attempts
WHERE job_id = 'union.gis.parcels'
  AND attempt_kind = 'full_scan' AND outcome = 'success'
ORDER BY finished_at DESC LIMIT 1;
```

A quarantined import or repair records `success_with_quarantine` and exits unsuccessfully as required work. It is not clean freshness evidence. See [controlled ingestion verification](ingestion-gaps-verification.md) for fixture coverage and unverified live gates.

## Import the four manual tax files

Download each complete county file outside the application. Do not change an input file during a run. Set these five variables in the shell that starts the job. Choose a persistent private directory for retained source files. Confirm that its filesystem supports directory sync. Set private permissions and a retention duration before the first run. Keep the input files and artifact directory outside Git.

```sh
export UNION_TAX_ARTIFACT_DIR="$TAX_ARTIFACT_DIR"
export UNION_TAX_DELINQUENT_PATH="$TAX_INPUT_DIR/delinquent.csv"
export UNION_TAX_BILLING_PATH="$TAX_INPUT_DIR/billing.zip"
export UNION_TAX_FORECLOSURE_PATH="$TAX_INPUT_DIR/foreclosure-sales.pdf"
export UNION_TAX_PIPELINE_PATH="$TAX_INPUT_DIR/foreclosure-pipeline.pdf"
./landman
```

Set `TAX_INPUT_DIR` and `TAX_ARTIFACT_DIR` to directories you selected before you run these commands. `./landman` runs the parcel import, soil import, soil screening, and the four-file tax job once each. `./landman run union.tax` runs only the four tax files. Individual parcel, soil, and screening commands remain available for targeted runs. There is no `./landman run` command without a job ID.

The job checks all four files before its first typed write. It retains and verifies their exact bytes, then imports delinquent, billing, foreclosure sales, and foreclosure pipeline in that order. The stored county URLs identify known source references; they are not proof that the application fetched these files. A changed file replaces its prior active version only after a complete successful publication. A failed file stops later files. A repeated identical file can link parcels added since the prior run. Check `job_attempts` for `union.tax` and the tax import records for source and storage evidence. Do not set a tax schedule in this local mode.

This command selects local artifact storage. S3 (cloud object storage) support in the artifact package is not an S3 input path or a bucket trigger. A later cloud change must define the upload event, four-file identity and order, retries, access controls, and explicit S3 selection. Do not use `s3://` paths with this command.

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
SELECT job_id, attempt_kind, outcome, processed_count, quarantined_count,
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

The manual `union.tax` job uses verified file hashes and parcel links to decide whether a file needs a new publication. It does not use a 180-day timer. Read [the source-job plan](../plans/source-jobs-runner-scheduler.md) and [the tax plan](../plans/tax-data-ingest.md) before you add another input path.
