# Read-only parcel data API

## Objective

Provide an authenticated Go API for the parcel, soil, and tax evidence already stored in PostGIS. Measure query efficiency with representative ingested Union County data. Establish the measured baseline before setting latency acceptance limits.

## Approved access model

Use the existing Google sign-in, allowed-account checks, and database-backed application sessions. Keep separate UI and API processes behind one browser origin. Follow [application login](../docs/application-login.md). Require authentication on every data route. Do not add bearer tokens, private lead records, ingestion routes, or UI changes.

## Data and module design

Use one `internal/parcelapi` module with a small HTTP handler interface. Keep typed input parsing, fixed parameterized SQL, result types, bounds, and response limits inside this module. Do not add a generic repository interface, forwarding service, response cache, or copied parcel read table.

Use `parcels.id` as the stable parcel identity. Keep county and source restrictions explicit. Use stored parcel facts for discovery. Name the acreage field used by filters. Label `likely_vacant` as provisional evidence. Do not implement planned owner labels or vacancy tiers in this change.

Read the existing soil assessment through `soil.CurrentAssessment`. Do not calculate spatial evidence or fetch source data during requests. Keep full soil evidence separate from the small parcel-facts response.

Read tax evidence from the four typed feeds and their active file-import records. Restrict retained foreclosure cases to the active imported file. Do not use the vacancy-restricted `tax_owed` view. Keep billed tax separate from reported delinquent balances. Return decimal money as strings. A missing row is not a verified zero or a current payoff quote. Preserve source dates, unavailable states, and source refresh failures. Do not expose private artifact paths, session data, or raw source payloads.

## Read contracts

| Route | Result |
| --- | --- |
| `GET /api/v1/parcels/lookup` | Capped parcel-number or address prefix suggestions. Lookup does not change discovery filters. |
| `GET /api/v1/parcels` | Stable keyset pages and matching counts. Support county-wide and bounded in-view results. |
| `GET /api/v1/parcels/map` | Bounded neutral parcel context and filter matches. Use grid counts at broad zoom and simplified shapes at close zoom. |
| `GET /api/v1/parcels/{id}` | Parcel facts, source identity, sales, and map location. |
| `GET /api/v1/parcels/{id}/tax` | Bills, reported back-tax totals and year details, foreclosure cases, and pipeline evidence. |
| `GET /api/v1/parcels/{id}/soil` | Published soil assessment or an explicit unavailable result. |
| `GET /api/v1/sources` | Available source dates, active file identities, and import status. |

Use acreage, provisional vacancy, and reported delinquency as initial supported filters. Apply the same predicates to list, counts, and map matches. With no active filter, return zero discovery matches but retain neutral map context and parcel lookup. Reject unsupported filters rather than ignoring them.

Validate UUIDs, bounded query lengths, cursor contents, finite ordered WGS84 bounds, zoom, and page size before SQL. Use bounded SQL deadlines and read-only transactions. Use the spatial index against stored geometry. Simplify only display geometry. Cap map features and response bytes. Return aggregates or an explicit limit error instead of a partial map presented as complete.

Use a consistent snapshot for the rows and counts in each response. Return source stamps and parcel update evidence so clients can detect a refresh between requests. Do not promise a retained database snapshot across independent HTTP requests. Keep separately fetched soil evidence explicitly versioned. Reject a cursor when its filter or supported source stamp differs.

## Design comparison

The direct-handler proposal keeps SQL, HTTP validation, and response limits in one module. The separate read-model proposal adds a module for SQL and response construction. Both proposals reject copied read tables without measured need.

Select the direct-handler proposal for its smaller public interface. Retain the second proposal's separate source stamps and explicit cross-request consistency limit. Do not add a parcel publication counter or change ingestion merely to serve this first read API. The configured design-judge pool uses the same vendor as the parent, so the separate-vendor design judge does not run. Require a different model for the final independent code and evidence review.

## Verification

- [x] Require valid sessions and allowed accounts on all data routes. Test absent, malformed, expired, revoked, and disallowed sessions. Database errors must deny access.
- [x] Verify query validation, keyset page order, lookup independence, county and viewport counts, and agreement between list and map filters.
- [x] Verify neutral map context, broad-zoom aggregation, close-zoom simplification, multipart shapes, feature and byte limits, and request cancellation.
- [x] Verify tax active-file selection, distinct delinquent years and totals, missing evidence, decimal values, and both foreclosure feeds.
- [x] Verify soil assessment provenance, stale and unavailable states, and no request-time recomputation or source fetch.
- [x] Add disposable PostGIS integration tests to CI. Test through HTTP and the existing soil read interface.
- [x] Measure a disposable copy of retained ingested county parcels and soil assessments. Import the four existing offline tax files through `jobs.Runner.Run`. Do not change the source databases.
- [x] Record dataset counts, PostgreSQL and PostGIS versions, machine load, query plans with buffers, first-observed and repeated timings, p50 and p95 request latency, errors, actual response contents, and response sizes.
- [x] Measure lookup, first and later list pages, combined filters, county counts, close and broad map views, parcel facts, tax detail, and soil detail. Include at least five repeated rounds and four concurrent requests. Measure real authenticated HTTP requests as well as SQL plans.
- [x] Investigate the slowest measured paths. Retain index or SQL changes only with repeatable evidence and unchanged results. Do not claim optimal performance or a speed improvement without a comparable baseline.
- [ ] Run repository Go checks, database checks, and one independent final review. Open a PR with the measured results and remaining performance limits.

## Scope limits

This is the first read API, not the completed browser application or V1 release. UI integration, private user workflows, DevNet requests, owner labels, vacancy tiers, zoning, roads, flood evidence, deployment configuration, and vector tiles remain outside this change. Preserve the release gates in [V1](v1.md) and [the browser plan](browser-parcel-ui.md).
