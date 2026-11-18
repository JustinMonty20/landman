# Parcel evidence verification

## Scope

Verification uses local HTTP fixtures and new disposable PostGIS databases.
No county, USDA, operator database, or deployment access was used.
Only parcel ingestion adopts the contract in this change.
Other source adapters remain unchanged. The soil dependency check supports parcel refresh decisions.

## Completed checks

The offline race suite, vet, build, staticcheck, Go formatting, migration-version check, and Python checks passed.
PostGIS race checks passed for parcels, the parcel job, parcel storage, jobs, soil, tax storage, qualification, authentication, and the parcel API.
The guarded tax fixture passed with the required local disposable database name.
The initial extra database run stopped at name guards. Correct database names resolved that setup failure.
Browser checks were not selected because this change does not modify browser code or its dependencies.

Focused fixtures cover retained bytes, source CRS, exact source numbers, stable fingerprints, and source-only changes.
They check unchanged parcel and sale content, source observations, important field history, multipart geometry, and PID-wide quarantine.
They also check cancellation, publication failure, source ordering, and omitted sale deletion.
Replay tests reject substituted source bytes even when local checksums are replaced.
They rebuild an empty database after the fixture server closes and retry a failed publication without a source request.
Historical replay cannot replace newer source state. Repeated correction does not create a new source observation.
Persisted dependency tests distinguish owner-only changes, geometry changes, expiry, missing assessments, and failed refreshes.

Migration 24 passed on empty and populated disposable databases.
The populated fixture retained its parcel, sale, IDs, content, geometry, timestamps, and previous job attempt exactly.
The built replay command also passed with HTTP proxies pointed at a closed local port.
It retained the parcel UUID and original observation time without source freshness or soil work.
The migration created no historical source evidence. Rollback and reapplication passed.

The decimal fingerprint regression fails with the original signed rounding helper and passes with the correction.
The corrected helper matches PostgreSQL half-away-from-zero scale rounding.
Parcel source validation still rejects negative amounts.

## County-sized fixture

The 118,000-feature gate passed a deliberate failed first publication, successful retry, and unchanged repeat.
The attempts took 3m32.65s, 3m41.45s, and 1m51.85s. Each stayed below the ten-minute limit.
The failed publication left no parcel or sale rows and retained quarantine evidence.
The repeat preserved all 117,999 parcel IDs, 353,995 sale IDs, mapped values, EWKB, and topology.
The actual sales foreign-key integrity query found no orphan rows.
The actual omitted-sale deletion predicate used the parcel-sales index in a rolled-back measured transaction.
These are single-run synthetic deadline and correctness results. They do not establish a speedup or live county performance.

## Evidence and release limits

Local logs and input hashes are under `artifacts/verify/parcel-ingestion-evidence/` and remain outside Git.
The repository review tool records the candidate commit, diff, input hashes, and receipt hashes.
GPT-6 Luna independently reviewed the committed code and the receipts. The final focused review passed.
The branch then rebased onto a browser-only main update. The receipt checker reported `UNCHANGED PATCH`.
The cross-vendor decision-trail audit did not run. xAI returned HTTP 402 for insufficient credits or subscription access.
This access failure is not a code failure or an audit pass. The PR is for operator review, not a merge-ready claim.

Local storage requires backups. No automatic retention expiry or cloud backend is added.
Checksums do not prove source authenticity. Offset pagination does not prove remote snapshot isolation.
The existing source deletion and wider V1 release requirements remain unchanged.
The operator must review the parcel contract before implementation on any other source.
