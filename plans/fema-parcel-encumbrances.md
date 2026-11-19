# FEMA flood evidence and Parcel Encumbrances

## Purpose and status

Plan two later evidence groups for Union County parcel review:

- **Flood evidence.** Show current effective flood zones, floodways, panels, and unknown coverage.
- **Mapped encumbrances.** Show county polygons that can indicate rights of way, access easements, conservation restrictions, or other recorded labels.

This document authorizes no implementation or live county-scale import. It does not change the current county ingestion PR.

FEMA remains a required V1 input under [the V1 plan](v1.md) and [the qualification plan](parcel-qualification-layers.md). Scheduling FEMA as follow-up work does not remove that release requirement. Parcel Encumbrances is a separate later addition. This document does not make it a new V1 release requirement.

Use branch `plan/fema-parcel-encumbrances` and PR title `[plan:fema-parcel-encumbrances] <summary>` for this plan. If implementation uses separate PRs, create matching scoped plans and branches first.

## Existing work and dependencies

The county zoning, jurisdiction, boundary, and road backend merged in [PR 73](https://github.com/JustinMonty20/landman/pull/73). Reuse its source generations, spatial assessment storage, runner composition, and geometry regression tests. [PR 74](https://github.com/JustinMonty20/landman/pull/74) reconciled the merged migration versions. Inspect the current merged code before implementation.

Reuse stable parcel IDs, complete parcel geometry, `jobs.Runner.Run`, and the application's existing Go API and UI. Keep source publication separate from parcel assessment. Do not create another scheduler, parcel importer, application, or general GIS framework.

[Shared ingestion](shared-ingestion.md) covers parcels, tax, and soil. When the operator authorizes FEMA or encumbrance ingestion, evaluate the existing capture, verification, and publication interface before adding another implementation. Keep source records typed and keep publication atomic for each source generation.

Keep both evidence groups independent of zoning, roads, and soil. A failed flood refresh must not remove valid encumbrance evidence or other parcel evidence.

## Source evidence

### FEMA

The primary source is the official National Flood Hazard Layer, or NFHL, county archive for Union study `37179C`. Discover the current effective product `NFHL_37179C` through the FEMA Map Service Center catalog. The qualification plan records a successful public catalog form request to `https://msc.fema.gov/portal/advanceSearch`. Isolate that web-form dependency and verify its current response contract before implementation.

Use `https://hazards.fema.gov/arcgis/rest/services/public/NFHL/MapServer` layers 28, 3, and 0 for flood-zone, panel, and availability cross-checks. ArcGIS availability must not be a dependency of archive ingestion. County layer 203 is a historical FEMA copy, not an acceptable silent replacement.

The earlier investigation recorded the following facts. These are historical observations, not current release receipts:

- The dated archive `37179C_20260914.zip` downloaded twice with matching hashes. ZIP integrity, a nonzero HTTP byte range, 1,551 flood zones, and 152 panels were checked.
- Of those zones, 88 had invalid rings. A tested repair retained valid polygonal results with a very small measured area change.
- The boundary audit found 0.246 acres outside panel coverage. Another 375.721 acres inside panels had no mapped zone polygon. Both areas must remain unknown.
- Catalog behavior across future versions, feature identity across releases, attribution, and redistribution terms remain open checks.

### Parcel Encumbrances

Official source: `https://atlas.unioncountync.gov/server/rest/services/OperationalLayers/MapServer/217`.

The read-only Go-client evaluation on 2026-11-18 returned HTTP 200. It identified a queryable polygon layer named `Parcels Encumbrances`, an `OBJECTID` key, a 2,000-feature page limit, and spatial reference 102100 with latest reference 3857.

The observed 3,775 features had these `EncumbranceType` values:

| County label | Count |
| --- | ---: |
| Prescriptive Right Of Way | 2,126 |
| Private Ingress-Egress | 1,230 |
| Conservation | 81 |
| Other | 35 |
| Unknown | 15 |
| Easement | 2 |
| ROW | 1 |
| Utility | 1 |
| No category | 284 |

These are source labels, not verified legal classifications. The numeric `Type` and `VALIDATIONSTATUS` fields have no verified domain. `SUBCODE` values do not have a verified document-reference meaning.

The observed schema has no explicit deed book, page, instrument number, or document URL. It declares no attachments or relationships. `LegalStartDate`, `LegalEndDate`, and `ModifyDate` had no populated values. Some records had creation or edit timestamps; those are not legal effective dates.

The evaluation checked only small feature samples and one polygon's ring closure. It did not verify county-wide geometry, parcel intersections, legal effect, or complete coverage. Local investigation receipts are not committed release evidence. Capture sanitized fixtures and reproducible receipts during implementation.

## FEMA completion checks

- [ ] Discover the current effective Union product. Preserve the study identity, catalog response, selected product, publication date, retrieval time, and last catalog-check time. Do not hardcode the historical archive as the current product.
- [ ] Check the catalog weekly and support manual refresh. Download only when the product path or revision changes. Catalog failure must produce visible source status, not a false claim that old data is current.
- [ ] Use bounded retries and disk staging. Resume only after verifying HTTP status, byte offsets, and `Content-Range`. Reject a changed or inconsistent partial download.
- [ ] Verify byte count, checksum, ZIP cyclic redundancy checks, required archive components, study IDs, feature IDs, and record counts. Retain the exact verified archive as an immutable versioned artifact in shared durable object storage before parsing. Keep artifact ownership in the FEMA job, not in the runner.
- [ ] Parse with bounded memory. Store original geometry, original spatial reference, raw attributes needed for evidence, and repair status. Apply an explicit valid, nonempty polygon repair policy with measured area-delta limits. Reject or quarantine a generation outside those limits.
- [ ] Publish zones, panels, and coverage as one atomic generation. Preserve the previous successful generation on failure or cancellation. Preserve prior generations and their archive references for traceability.
- [ ] Retain source identities with their release scope. Verify later-archive identity behavior before assuming cross-release stability. Do not use repeated `GFID` values as unique keys.
- [ ] Retain zone and subtype separately. Distinguish mapped special flood hazard area, mapped 0.2 percent annual-chance area, regulatory floodway, unknown values, and unstudied coverage. Zone X alone is not a sufficient classification.
- [ ] Calculate full-parcel intersections and union coverage within each category before area measurement. Floodway can overlap the special flood hazard area; do not add that area twice. Boundary contact alone has zero overlap area.
- [ ] Return contributing features, acres, parcel shares, coverage gaps, panel dates, and official verification references. Keep panel effective dates, study index date, database revision, map revision dates, product publication, and retrieval time separate. Treat `99990909` as a sentinel, not a future date.
- [ ] Include relevant boundary-crossing features. Reconcile zone coverage with panel and availability evidence. A panel outline or absent zone intersection must not become a flood-free finding.
- [ ] Confirm source attribution and application redistribution terms before public release. Validate catalog changes and source identities against later products; do not describe fixture-only checks as completed live gates.

## Encumbrance completion checks

- [ ] Capture sanitized metadata and feature fixtures for layer 217. Check field types, domains, null labels, geometry, source dates, source terms, and coverage limits. Retain unresolved meanings as unknown.
- [ ] Use the existing county Go HTTP behavior, request identity, redirect policy, and shared one-request-per-second budget. All metadata, count, page, and retry requests must use that budget, including concurrent parcel, soil, zoning, and road jobs.
- [ ] Use ordered, bounded pages with count, transfer-limit, duplicate-ID, and schema checks. Treat `OBJECTID` as layer-scoped; test replay and retirement without claiming unverified cross-release identity.
- [ ] Publish encumbrances as a separate atomic source generation. Preserve raw source labels, original geometry, available edit dates, retrieval time, and audited repair results. Failure must preserve the last successful generation and remain visible.
- [ ] Intersect full parcel geometry with mapped polygons. Return each contributing feature and its mapped category. Calculate unioned overlap acreage without duplicate inflation. Distinguish positive-area overlap from boundary-only contact.
- [ ] Distinguish mapped overlap, no mapped overlap, unknown category, missing parcel geometry, unavailable source, and stale evidence. No mapped overlap must not mean no encumbrances or unrestricted title. An unknown or blank category must not pass a favorable filter.
- [ ] Show the source label and a warning to verify the recorded document and current survey. State that legal terms and effective dates are unavailable where the source does not provide them.
- [ ] Keep document lookup separate from this import. Do not invent instrument references from `SUBCODE`, scrape a deed registry, or infer legal rights. A later document-retrieval feature needs its own source and scope decision. Missing document linkage does not prevent an honestly labeled map warning.

## Shared jobs and application delivery

- [ ] Route all source and derived entry points through `jobs.Runner.Run` with stable source-specific IDs. Keep independent calendars and manual commands. Keep FEMA requests separate from the county-host budget; layer 217 shares the county budget.
- [ ] Derive assessments after relevant parcel or source changes through explicit composition. Provide reconciliation after a crash between source publication and calculation. Key results by parcel geometry, source generation, and method version.
- [ ] Reuse the reviewed geometry rules from the county backend. Preserve holes and multipart shapes. Separate source topology from area measurement. Declare analysis precision, measurement tolerances, and repair limits for each new input; do not assume a county-tested limit automatically fits FEMA.
- [ ] Add separate Flood evidence and Mapped encumbrances map groups in the existing application. Include toggles, legends, attribution, source status, and textual parcel evidence. Use the same source generation for displayed geometry and calculations.
- [ ] Serve bounded map and parcel queries from stored data. Map movement and parcel selection must not trigger live source requests. Simplified display shapes must not drive area calculations or filters.
- [ ] Preserve existing authentication and user isolation. Do not alter saved leads, notes, states, reminders, or recorded decisions when evidence changes. No automatic parcel rejection, legal-access certification, flood engineering, ordinance interpretation, or owner-facing outreach.

## Verification and handoff

Use [the review verification procedure](../docs/review-verification.md). Select checks for the changed behavior, not a fixed agent count.

- [ ] **Source fixtures.** Test catalog revision changes, malformed archives, failed range resume, checksum failures, missing components, invalid IDs, unknown categories, date sentinels, schema drift, source errors, cancellation, and bounded retries. Ordinary tests use local fixtures, not live services.
- [ ] **PostGIS integration.** Test multiple flood panels and dates, overlapping floodway and hazard polygons, coverage gaps, unknown zones, encumbrance categories and nulls, duplicates, holes, multipart shapes, boundary crossings, edge-only contact, mixed polygon-and-line intersections, and oblique shared boundaries.
- [ ] **Refresh.** Replay identical inputs, retire a feature, fail midway through each group, cancel a job, replace parcel geometry, and force concurrent status/publication cases. Verify stale then recomputed results and unchanged user records.
- [ ] **Numerical checks.** Use known area fixtures and independent measurement references with explicit tolerances. Small numerical error must not invent a restriction, remove a genuine mapped overlap within declared precision, or change a favorable filter result.
- [ ] **Application checks.** Verify independent toggles, selected-parcel highlights, dates, unknown and stale states, keyboard access, map/list consistency, and private-data isolation. A map warning must not imply verified legal rights or flood safety.
- [ ] **Scale checks.** Obtain approval for each county-scale import and use a disposable migrated database. Record request count, bytes, peak memory, import duration, spatial query plans, and map/detail latency. Agree interactive budgets before UI acceptance. Investigate measured slow paths before adding worker pools or tiles.
- [ ] **Release checks.** Run the repository's applicable Go, database, and UI checks. Save source receipts, fixture hashes, independent review results, and explicit unresolved gates. A source-specific unknown can be visible; an unimplemented FEMA group cannot satisfy V1.

Implement the source and stored-evidence work in independently verifiable units. Add each map group only when its stored API contract exists. FEMA completion remains part of the existing V1 release gate; encumbrance timing remains a separate delivery decision.

## Later geometry accuracy research

This is a separate research item, not a new implementation or V1 gate. The current calculations support mapped parcel review within documented precision. They do not provide surveyed boundaries, exact curved-edge measurements, or a proven error bound for every possible shape. Keep the existing source, full-county import, and performance gates.

- [ ] **Current guarantees.** Document the one-micrometer analysis grid, 10-meter edge subdivision, tested measurement tolerances, and unverified cases. Separate coordinate resolution, source positional accuracy, calculation precision, and legal boundary accuracy. A fine calculation grid does not establish accurate source boundaries.
- [ ] **Exact boundary calculations.** Compare the current method with exact arithmetic on the supplied coordinates. Test overlap, contact, containment, holes, multipart shapes, and mixed-dimension intersections. Determine which failures exact arithmetic prevents and which depend on source quality or projection.
- [ ] **Bounded measurements.** Research error bounds for projected areas and distances. Compare adaptive edge subdivision with the current fixed subdivision and independent higher-precision references. Do not describe a fixture comparison as a universal guarantee.
- [ ] **Decision uncertainty.** Identify cases where measurement error could change a road-radius result, nearest-road tie, parcel share, or overlap classification. Specify an uncertain result when the error range crosses a decision threshold. Do not turn uncertainty into a favorable result.
- [ ] **Cost and decision.** Measure runtime, memory, implementation cost, and maintenance cost on representative county shapes. Recommend whether to retain, improve, or replace the current method. Record the supported accuracy, remaining limits, and regression checks before any implementation decision.
