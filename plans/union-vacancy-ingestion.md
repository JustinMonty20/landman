# Union vacancy evidence ingestion

## Scope

Capture Union Structures layer 171 and Assessed Value layer 0 as separate sources. Use the [shared ingestion contract](shared-ingestion.md) and the [vacancy evidence plan](vacancy-building-tax-evidence.md). Use the existing application and county request controls. Do not add automatic schedules in this change. Do not run a live full-county import without approval.

## Capture and replay

- Register each manual job with `jobs.Runner.Run`. Use the existing county client, rate limit, and host exclusion. Keep the two source captures independent.
- Retain bounded exact response bytes and request metadata with `ingest.Engine`. Raw SHA-256 identifies each response. A separate versioned canonical multiset fingerprint compares normalized rows with multiplicities against the latest generation. Reversions A to B to A publish A again. Replay can correct its own latest generation but must not contact the county or replace newer evidence.
- Preserve repeated assessment rows in the raw capture. Do not assume `OBJECTID` is unique in layer 0. Reject malformed or count-mismatched observations. A failed refresh must not replace the latest accepted bounded observation. The runner's generic `full_scan` attempt label is not a completeness assertion; `coverage_verified` stays false.
- **Live gate.** Structures capture accepts at most 250,000 features in 2,000-row pages ordered by `OBJECTID ASC`, reconciled against unique numeric `returnIdsOnly` membership lists before and after paging, with count receipts at both ends. Page OIDs must match the sorted source ID list exactly; count, missing or repeated IDs, changed membership, and transfer-limit responses reject publication. These checks detect observed drift but do not prove a snapshot or source-wide completeness. Assessed Value remains limited to 2,000 rows: its `OBJECTID` repeats, and neither a verified disjoint partition rule retaining whole repeated-ID groups nor a unique total row order has been established. Never use offset paging on that nonunique ID or discard duplicates. Manual `full_scan` remains a runner attempt label, not a verified scan. Countywide assessment capture and real tiers remain blocked pending source-supported assessment partition completeness and live source identity, snapshot behavior, transfer-limit, repeat-scan, terms, dates and coverage verification. Fixtures cannot establish those live properties.
- **Response limits.** HTTP refusal bodies, malformed JSON and invalid CRS pages are retained when bounded response bytes are available. Transport failures with no body have nothing to retain. Responses over 32 MiB are truncated to 32 MiB plus one byte and rejected; artifact-store limits may prevent retaining even that prefix.

## Likely-vacant filters

- Join the sources to parcels independently. Compare footprint polygons with the full parcel polygon. Positive-area overlap counts even without full containment. Edge contact does not count. Keep `PIN_Num` as source evidence. Flag a mismatch between the reported parcel number and spatial overlap.
- Use `market_building` only from an applicable `AV` (Appraisal) row. Do not use `PY` (Prior Year), `market_land`, or a higher year or amount as a substitute. Treat identical duplicate rows as one observation for selection. Keep missing values, conflicting applicable rows, and an unverified appraisal year unknown.
- Show **strong candidate** only for an explicit zero building value, verified footprint coverage with no positive-area overlap, and parcel facts that do not report a structure. Show **conflict** when comparable sources disagree. Show **review** when building evidence needs manual review. Show **unknown** if required data, coverage, or parcel identity cannot be verified. Tax debt and imagery do not change the tier.
- Keep `likely_vacant` active until both sources and parcel matching pass validation, real tiers are backfilled, and all readers can use the new tiers. Do not edit migration `000038` or enable real tiers from unverified captures.

## Source evidence

A point query for parcel `09336350` returned two identical `AV` rows and two identical `PY` rows. Both categories reported year `2027`. This confirms the category labels for that response. It does not prove that year `2027` is effective. 98.46% of the mapped footprint area overlapped the saved parcel geometry, not vice versa. Two additional appraisal samples had no footprint match by parcel number. Those searches did not test spatial overlap or county coverage.

## Verification

- Use local HTTP fixtures and disposable PostGIS. Verify cancellation, malformed and partial captures, duplicate assessment rows, hash failure, identical repeats, and offline replay. Verify that an old replay preserves newer evidence.
- Keep coverage, effective appraisal year, conflicting row selection, county-wide paging, and import duration as live verification checks. Do not claim that fixture tests pass these checks.
- Add the satellite link in the [browser parcel plan](browser-parcel-ui.md). Do not embed imagery in this work.

## Remaining county-scale measurement
- Structures capture and offline replay reject more than 256 MiB of combined response bytes; each response also remains limited to 32 MiB. The 250,000-row count gate and job deadline still apply. The aggregate budget limits retained bytes, not peak heap or database publication time. An over-budget live capture is rejected, never downgraded to the older two-receipt protocol.
- Before countywide use, measure actual county Structures count, aggregate response bytes, peak heap during normalization and fingerprinting, publication duration, deadline margin, ID reconciliation stability, and repeat-scan coverage under approved live access. Fixture tests do not establish these measurements. Assessment remains capped at 2,000 rows and cannot page without a supported complete partition.
- Two-receipt historical Structures captures may replay offline with their original bounded single-page gates and HTTP audit checks. Their published provenance explicitly says source-ID membership was not reconciled. New live fetches always use paged, reconciled Structures capture.
