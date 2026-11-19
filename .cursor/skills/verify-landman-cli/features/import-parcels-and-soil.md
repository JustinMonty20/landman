# Import parcels and soil

The isolated GIS journey runs soil first and parcels second. Each successful source command screens stored soil and reconciles stored qualification data. Soil must run first because the seed has polygons but no active soil set. The combined bare command belongs to the tax skill.

## Sub-features

- `import-build` builds the real CLI.
- `import-run` publishes soil and parcels through two separate source commands. Each command has its own ten-minute deadline.
- `import-jobs` checks clean full scans, stored screening, and parcel assessments.
- `import-verdict` compares seeded sale slots and database publication through `classify`.
- `import-usage` rejects unknown commands before database access.
- `import-reprocess` checks repair limits and empty repair without another county fetch.

## How to get to it (user POV)

- Use `./landman run union.gis.soil` and then `./landman run union.gis.parcels` with the isolated host database URL.
- Use `./landman run union.soil.screening` to screen stored soil only.
- Use `./landman reprocess union.gis.parcels --limit N` to repair at most `N` pending records. A successful repair also refreshes derived evidence.
- Use `verify-landman-tax` for bare `./landman`. That path also requires four manual tax files.

## Driving it with make and the landman binary

Preconditions:

- Complete the seed launch and both doctors in `live-gis.md`.
- Require `check-loaded` to exit 0 before any GIS command.
- Ensure no other verification import runs. Preserve source request budgets.

- Follow the exact runner command in `live-gis.md`. It invokes soil, then parcels. It stops on the first failure and captures each command's stdout, stderr, and exit status. Do not run additional bare imports.
- Run `classify` even after a failed runner. A pass requires empty stdout, published parcel and soil full scans, successful screening, a screening head, and one assessment per county parcel. Soil and overlap counts must exceed the seed. Seeded sale slots must match current county evidence. Do not query a county-wide sale total.
- Keep source-specific logs, combined `import.*` logs, `page-timings.json`, and `drive-outcome.json` under `artifacts/verify/$RUN_ID/`. Require `import.exit.txt` to match the runner exit.
- Run `env -u DATABASE_URL ./landman import`. Require exit 1 and usage text containing `run <job ID>`.
- Run `env -u DATABASE_URL ./landman reprocess union.gis.parcels --limit 0`. Require exit 1 and `reprocess limit must be between 1 and 100`.
- After classification, query `SELECT count(*) FROM quarantined_records WHERE job_id='union.gis.parcels' AND reprocessed_at IS NULL`. If zero, run `DATABASE_URL="$HOST_DATABASE_URL" ./landman reprocess union.gis.parcels --limit 1`. Require no parcel fetch lines and successful derived attempts. If pending rows exist, report the empty-repair check as not run; a repair would fetch county data again.

## Gotchas

- Soil runs before parcel fetch progress. `Fetch complete:` ends only parcel fetching, not derived refreshes. Quiet output is not proof of completion.
- The runner's two commands each have ten minutes. This does not verify the combined bare command's single ten-minute budget.
- Quarantined parcel records can leave prior parcel values unchanged. `success_with_quarantine` is non-clean and stops that source's derived work.
- A parcel command without an active soil set can publish parcels and then fail with pending screening. The seed alone does not supply an active soil set.
- `classify` reports `inconclusive: county-source-slow` only for its existing deadline and timed-probe conditions. USDA HTTP 502 is an external drive failure, not a slow-county pass. Keep the helper verdict distinct from the maintenance outcome `blocked`.
- Qualification failures can make a source command fail after source publication. The GIS classifier does not prove qualification correctness; run the qualification recipe separately.
- Likely vacancy is provisional. Missing tax data is not zero debt. The GIS journey does not verify tax ingestion.
- `make run` loads `.env`. Never use it for verification. Keep verify database overrides on every database Make command.
