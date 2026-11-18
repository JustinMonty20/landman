# Union parcel full-scan performance

## Objective and boundary

Reduce the Union parcel full-scan time on the saved 118,202-feature capture without changing the existing ten-minute attempt deadline. This plan follows the parcel evidence contract in `plans/parcel-ingestion-evidence.md` and does not authorize live county requests, database writes outside task-owned disposable PostGIS, retention changes, or changes to other source adapters.

Keep exact durable capture bytes, checksum verification, source and normalized hashes, whole-PID quarantine, stable IDs, complete geometry, omitted-sale deletion, pending derived reconciliation, and atomic publication. Replay remains offline and cannot claim a new observation.

## Measurement and change

- **Baseline.** Check the saved capture manifest and every response checksum. Run the unmodified PR #77 head on an empty migrated PostGIS 18/3.6 database. Measure the first import and identical second import separately. Sample SQL activity and `pg_stat_statements.track=all`, including nested foreign-key checks and omitted-sale deletes. Do not use the second import as the first-import baseline.
- **Candidate.** Batch the raw-key lookup and quarantine resolution already performed for every complete PID into the existing 200-PID publication window. Keep the same selection tokens, source cutoff, and publication transaction. Measure a fresh database with the same capture and settings. Stop at the smallest change that meets the deadline.
- **Verification.** Compare the exact feature, parcel, and sale counts; complete geometry and stable IDs; unchanged rows and timestamps; failed-first rollback and retry; quarantine selection; and actual query counts. Run focused PostGIS regressions and repository CI equivalents. Record raw timing and query logs outside Git, not in this document.

## Decision and limits

Use a paired first-import result for the budget decision. A single pair is not proof of a repeatable speedup. Repeat alternating first-import trials if resources permit, and report load and run-to-run spread. A timeout, error, or unverified source receipt is a failed attempt, not a faster import. Do not extend the deadline if no bounded improvement meets the budget; report the measured blocker instead.
