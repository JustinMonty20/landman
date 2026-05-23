# Plan

Add a multi-cron workflow that starts from narrowed parcel candidates, checks county estate/will/probate dockets for ownership-change signals, and sends first-notice SMS/email alerts. The implementation should keep each cron independently testable and idempotent so retries do not duplicate alerts.

## Scope
- In: Candidate parcel persistence, docket polling cron(s), case-event normalization, deduped notification pipeline, and operational checkpoints per county/source.
- Out: UI work, CRM automation, third-party lead scoring, and non-county private data integrations.

## Action items
[ ] Define canonical data models for `CandidateParcel`, `CaseEvent`, and `NotificationEvent`, including stable dedupe keys (`county + parcel_id + case_number + event_type`).
[ ] Add a `parcel-reducer` cron job that reads the existing narrowing pipeline output and persists only the final surviving parcel IDs plus owner snapshot and run timestamp.
[ ] Add a `docket-monitor` cron job that loads latest candidates and queries county docket sources (estate/will/probate) through county-specific adapters behind interfaces.
[ ] Implement county adapter contracts with DI seams for HTTP, parser, clock, and checkpoint store so source logic is unit-testable and swappable per county.
[ ] Normalize raw docket results into typed `CaseEvent` records with parcel/owner linkage, event dates, and source metadata.
[ ] Add checkpointing per county/source (`last_run_at`, pagination cursor, or last seen filing date) so polling is incremental and restart-safe.
[ ] Add idempotent notification selection that only emits unseen `NotificationEvent` records based on dedupe key + status.
[ ] Add a `notifier` cron/worker that sends SMS and email via provider interfaces, with retry policy, dead-letter handling, and final delivery status persistence.
[ ] Add observability: per-run counts (`candidates_in`, `events_found`, `events_new`, `alerts_sent`, `alerts_failed`) and structured logs for audit/debug.
[ ] Add stdlib-only unit tests for adapter parsing, dedupe behavior, checkpoint logic, notifier retry/idempotency, and cross-cron handoff integrity.
[ ] Validate with `go test ./...` and `make test`, then run a dry-run mode that logs intended alerts without sending.

## Open questions
- Which counties and docket endpoints are first (and do they support API access vs HTML scrape)?
- What qualifies as an alertable event at v1 (new case filed, status change, executor appointment, sale order, etc.)?
- What alert cadence should be used for duplicates/updates (send once, send daily digest, or resend on status transition)?
