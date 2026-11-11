# Parcel qualification layers for Application V1

## Outcome and release scope

Ship three evidence groups in the Union County discovery map, synchronized results list, and parcel detail view: zoning/jurisdiction, road proximity, and current effective flood-hazard mapping. They help a buyer decide which parcels warrant further research before saving leads, recording personal observations, and manually contacting owners outside the application. They are not a general GIS viewer, an automated suitability verdict, or proof of seller motivation.

The operator explicitly committed all three groups to V1. They are no longer optional qualification candidates. Earlier development milestones may exercise personal lead tracking without them, but V1 is not complete until these layers and the tracking workflow both pass their release checks. This document is a plan, not authorization to start implementation.

Use branch `plan/parcel-qualification-layers` and PR title `[plan:parcel-qualification-layers] <summary>`. If delivery is split across PRs, create matching scoped plan files before opening differently named plan branches; do not break the repository's title/branch/plan convention.

## Existing inputs and boundaries

- [application-v1.md](application-v1.md) owns the buyer workflow, user isolation, saved lists, personal scratchpad, lead state machine, activity history, and follow-up reminders. This plan supplies evidence to that workflow, not another lead-management system. The current application explicitly excludes mailing preparation/export and owner-facing message generation or delivery.
- [soil-layer.md](soil-layer.md) owns durable multipart parcel persistence, stable parcel UUIDs, and soil screening. Reuse its parcel foundation. Do not wait for soil screening to implement these three groups, and do not change its soil release contract.
- [source-jobs-runner-scheduler.md](source-jobs-runner-scheduler.md) owns execution history, job exclusion, scheduling, cancellation, and quarantine. Use it rather than adding a parallel scheduler. A source job owns publication transactions and assessment freshness.
- Present code has the Union layer-215 fetcher, translator, parcel/sales writers, and an executable streaming likely-vacant IDs. The full parcel import, runner, these layers, and the application UI are planned rather than all implemented. No frontend source or established application API is present in this checkout; identify the application implementation location before the UI work. Backend completion alone cannot satisfy this plan.
- Keep utilities, contours, structures, address/subdivision layers, encumbrances, streams/lakes, watershed and agricultural/airport overlays outside this change. County boundary and FEMA coverage/panel metadata below are supporting inputs for the three committed groups, not additional user-facing layer projects. Do not implement ordinance interpretation, routing, legal-access certification, flood engineering, or automated lead rejection.

## Sources and drafting evidence

County service root: `https://atlas.unioncountync.gov/server/rest/services/OperationalLayers/MapServer`.

| Group | Source layers | Retained evidence |
| --- | --- | --- |
| Zoning | 190 Union County; 173 Fairview; 174 Indian Trail; 175 Lake Park; 176 Marshville; 177 Marvin; 180 Monroe; 181 Stallings; 183 Waxhaw; 184 Weddington; 186 Wingate | Geometry, source jurisdiction, original district code/description, source identifiers, available ordinance/case links, dates and provenance |
| Jurisdiction | 194 ETJ; 195 Municipalities | Geometry and original jurisdiction name; evidence of mapped jurisdiction, not a legal determination |
| Roads | 170 Road Centerlines | Line geometry, source ID, name, road class/status where supplied; proximity, not access rights |
| Flood | FEMA effective NFHL layer 28 Flood Hazard Zones | Geometry, `FLD_ZONE`, `ZONE_SUBTY`, `SFHA_TF`, study/version identifiers and source references |
| Supporting coverage | County 221; FEMA NFHL 0 Availability and 3 FIRM Panels | County query boundary, flood coverage, panel identifiers and `EFF_DATE` where available |

FEMA service root: `https://hazards.fema.gov/arcgis/rest/services/public/NFHL/MapServer`.

**Verified during drafting:** county metadata identifies the listed county layers as queryable feature layers. Fairview uses OID field `OBJECTID_1`; the other listed zoning layers use `OBJECTID`. Zoning fields differ: `ZONE`, `NEW_ZONIN`, `LP_Zone`, `Marvin_Zon`, `NEW_ZONING`, and `CurrentZon` occur; Marshville and Waxhaw expose both `Zoning` and `ZoneDesc`, whose meaning must be checked against sample values before choosing a mapping. Do not hardcode one district-field name across municipalities. Several descriptions report updates in 2022–2023; a fresh retrieval does not establish fresh zoning.

FEMA metadata returned the named polygon layers and a 2,000-feature limit for layers 28 and 3. A Union-area coverage-point query returned study `37179C`; a county-envelope panel query returned a panel record. The envelope also touches adjacent counties and is not an exact county clip. Both attempted layer-28 feature queries failed with connection resets. No full flood download, Union-wide coverage proof, or effective-date reconciliation has been verified yet.

**Source gate:** before implementation is considered ready, capture successful bounded feature queries and count/pagination checks for every source; confirm redistribution terms and attribution. For flood data, establish a reliable current effective NFHL feed and verify Union coverage. If that endpoint remains unusable, investigate an official FEMA download or NC floodplain feed, document its schema, effective status, coverage, and refresh method, and reconcile this plan before substituting it. Do not bypass access controls or silently use county layer 203, labeled FEMA 2014. The current-feed requirement remains a release gate, not an excuse to ship an empty flood layer.

## Evidence contracts

### Zoning and jurisdiction

- [ ] Store each district with its source jurisdiction and original code. A code such as `R-20` is not globally meaningful without its jurisdiction. Preserve case/ordinance links as evidence, validate link schemes, and retain source dates separately from retrieval dates.
- [ ] Intersect full parcel geometry with zoning, municipality, and ETJ polygons. Return every positive-area district intersection with acres/share, including split-zoned parcels. Measure area in EPSG:2264 and retain source-feature references. Do not choose a district by centroid or silently discard a smaller intersection.
- [ ] Treat municipal, ETJ, and county layers as independently dated evidence. Boundary overlap, contradictory assignments, missing coverage, and old/unknown dates must be visible. Do not let county zoning automatically override municipal zoning, or interpret absence from the municipality layer as proof of current county authority.
- [ ] Expose mapped jurisdiction and a route to the relevant planning authority/official map for verification. Confirm current jurisdiction and source coverage, including any municipality absent from the service list; report gaps explicitly. No inference that a district permits the buyer's intended use, subdivision, density, or minimum lot size without separately verified rules.
- [ ] Group non-overlapping mapped districts as split zoning, distinguish conflicting overlapping assignments, and calculate unmapped/ambiguous acreage without summing duplicate coverage. No universal zoning eligibility badge.

### Road proximity

- [ ] Show road centerlines and the nearest mapped road within a configured search radius, with its name, available class/status, distance in feet, and source date/freshness. Initial radius is one mile; record it in every derived result and expose the bounded-search meaning.
- [ ] Calculate minimum distance from the full parcel geometry to the road line in EPSG:2264, not from the parcel centroid. Use a spatial index and a bounded distance query. Retain tied nearest features rather than making an arbitrary road authoritative.
- [ ] Distinguish a distance result, no mapped road found within the radius, missing parcel geometry, and unavailable source. County-edge parcels may have nearer roads outside the county dataset; flag incomplete search coverage rather than claiming a globally nearest road.
- [ ] Zero distance means a mapped line intersects/touches the parcel, not frontage or lawful entry. Road names/classes/status do not prove public maintenance, a driveway permit, an easement, or legal access. Layer 159 ROW is map-only annotation and is not an ingestible ROW geometry source.

### Current flood evidence

- [ ] Keep original zone/subtype and SFHA designation. Present mapped special flood hazard area (1% annual-chance), mapped 0.2% annual-chance areas where classified, and regulatory floodway distinctly. Verify classification against source domains; zone X alone cannot distinguish all subtypes. Preserve unknown and unstudied categories rather than interpreting them as low risk.
- [ ] Return intersecting zones with acreage/share, floodway area, supporting panels/effective dates, coverage status, and verification links. A parcel can cross panels or studies; never reduce their effective dates to an invented single parcel date.
- [ ] Union intersected polygons within each category before measuring. Floodway may overlap SFHA: label it as an overlapping category, not additive extra flood acreage. Boundary-only contact has zero overlap area. Retain contributing feature references for map highlighting.
- [ ] Distinguish no mapped hazard overlap within verified coverage from unmapped/unknown coverage, failed source retrieval, and stale evidence. A FIRM panel outline alone is not proof that every location has usable digital hazard mapping. Validate layer-0 availability together with zone records and panel metadata; no intersection is never a flood-free certification.
- [ ] Use the current effective dataset, not preliminary proposals or historical county copies. Display both effective dates and fetched/last-success times. Link to official verification for letters of map change and parcel-specific determinations; V1 does not adjudicate LOMA/LOMR documents, insurance requirements, or site flood safety.

## Ingestion, storage, and recomputation

- [ ] Add source-specific typed adapters under `internal/connector/union/` and `internal/connector/fema/`, plus qualification persistence/spatial queries under `internal/qualification/`. Use existing shared HTTP behavior. Any shared ArcGIS paging helper must serve actual county and FEMA callers, not become a general plugin framework. Keep county-specific types out of `internal/models`.
- [ ] Add new sequential Goose migrations with a source-import manifest and domain-specific zoning, jurisdiction, road, flood-zone, and coverage/panel tables. Feature identity includes service, layer, and source feature ID; use stable source global IDs where verified, otherwise scope OIDs appropriately. Retain original SRID, source metadata, and provenance; store geometry at SRID 3857 with GiST indexes. Preserve polygon holes, multipart polygons, and multiline roads. No arbitrary JSON document is the public assessment contract.
- [ ] Stage each import, stream bounded pages, validate required fields, geometry type/validity, unique source IDs, transfer-limit flags, and complete requested coverage. Discover OID/page limits from metadata. Use consistent ordered paging and account for every ID; counts alone cannot prove a snapshot when the remote service changes. Record consistency limitations and fail/retry on detected drift. Do not fetch the national flood dataset or issue one remote request per parcel.
- [ ] Restrict FEMA retrieval spatially to the Union study area using the county boundary/envelope and local clipping; include boundary-crossing features and deduplicate repeated features if query tiling is needed. A county code filter may assist but must not silently omit relevant cross-boundary records. The flood zones, coverage, and panels must belong to one validated import group.
- [ ] Publish an import group atomically only after successful validation. Zoning/jurisdiction layers form one consistency group; roads form another; FEMA flood/coverage/panels form another. Keep previous successful generations for traceability, but expose failed refresh and age. Quarantined/missing required geometry cannot quietly create a clean generation. Retired features disappear from the new active membership without deleting stable parcel or user records.
- [ ] Use the planned runner with explicit county-qualification and FEMA source jobs, separate source outcomes, manual execution, and calendar schedules. County requests share the existing county-host rate budget of one request per second, including overlap with parcel/soil jobs; separate clients or staggered schedules alone do not enforce that budget. Resolve shared-client/host exclusion at composition time, not inside the scheduler. Give FEMA its own conservative limiter and bounded retry policy, honoring server retry guidance. Cancellation and HTTP/ArcGIS error bodies fail the relevant import.
- [ ] Initial refresh target is daily, using explicit `America/New_York` schedules plus manual refresh; record actual publication cadence and adjust before launch if source terms require it. Default retrieval-stale threshold is 48 hours since clean success, configurable per source. This threshold is not proof of content currency: old/unknown zoning dates remain separate warnings, and an old effective flood date can still be the current effective map.
- [ ] Derive per-parcel evidence after relevant successful source or parcel updates, keyed by parcel UUID and parcel-geometry/source-generation/method versions. Persist only complete assessments; a version mismatch is stale until recalculated. Trigger this through explicit composition/downstream job execution, not assumptions about cron order. Failure in one group must not erase other groups or mark them current incorrectly.
- [ ] Do not reuse stale append-only soil joins or change their contract. Source refresh and derived evidence must never delete or rewrite saved lists, scratchpad entries, activity/status history, reminders, or do-not-contact flags. Qualification findings do not automatically exclude a lead, transition its state, cancel reminders, or alter owner labels.

## Buyer-facing delivery

- [ ] Add three named map groups: Zoning and jurisdiction, Roads, and Flood evidence. Each has a legend, toggle, attribution, source age/status, and visible unavailable/stale behavior. Parcel boundaries and the selected parcel stay legible. The same source generation drives displayed shapes and computed parcel facts.
- [ ] Add parcel-detail sections and concise results-list summaries using the evidence contracts above. Clicking a district, road, or flood result highlights its contributing geometry. Missing/conflicting data must be distinguishable from a clean negative result, not shown as a blank panel.
- [ ] Provide filters for mapped zoning jurisdiction/code, road-distance threshold within the supported radius, and mapped SFHA overlap. Use explicit any-overlap semantics for zoning and show split/conflicting status. Filter calculations use full-resolution geometry, not simplified display shapes. Stale/unknown records remain separately selectable and counted; default discovery does not hide them as though they passed a suitability screen.
- [ ] Serve bounded viewport and parcel-detail queries from stored data; no live county/FEMA request on every pan or parcel click. Define pagination/feature limits and geometry simplification for display while preserving authoritative analysis geometry. Reuse the application's API/map stack once its location is established; do not invent a second frontend or unrestricted GIS endpoint.
- [ ] From the same review view, retain Save to list, Add scratchpad note, Change status, Set follow-up, and Exclude. Users record selection/rejection reasons and may reference the reviewed evidence generation; refresh does not silently rewrite that recorded rationale. Handwritten notes remain external manual activity. No mailing export/preparation, message generation/delivery, or campaigns.
- [ ] Apply the application's authentication and user-isolation rules to saved annotations/actions. Escape source text and validate external links. Shared public map evidence must not expose another user's private scratchpad, lists, activity/status history, or reminders.

## Work order and verification

### Establish source contracts and prerequisites

- [ ] Capture sanitized metadata and real feature fixtures for every listed county layer and the verified current flood feed. Resolve municipal field mappings, missing jurisdiction coverage, FEMA availability semantics, effective-date sentinels, source key stability, and redistribution attribution. Distinguish verified evidence from open questions in the implementation report.
- [ ] Confirm the durable parcel foundation and runner entry points have landed or identify their dependent work explicitly. Confirm where the application API/UI will be implemented. No duplicate parcel importer or speculative frontend scaffold.
- [ ] **Gate:** successful bounded flood-zone retrieval, complete Union coverage assessment, and verified classification/effective metadata are required before claiming the flood implementation is source-ready. Current drafting evidence does not yet pass this gate.

### Persist sources and derive parcel evidence

- [ ] Implement migrations, adapters, atomic generation publication, and typed qualification queries. Read-only source verification is separate from default tests; county-scale imports require approval and a disposable local database.
- [ ] **Unit:** local HTTP fixtures cover each field mapping and domain, pagination caps, duplicate/missing IDs, ArcGIS errors in HTTP 200, invalid geometry, timeouts, retries, cancellation, and schema drift. No live services or database in ordinary tests.
- [ ] **Integration:** migrated PostGIS fixtures cover a split-zoned parcel, municipality/ETJ conflict, gap in zoning, county-edge roads, equal-distance roads, a road touching the parcel, multiple flood panels/dates, nested floodway/SFHA, unknown zone subtype, unavailable coverage, polygon holes, and zero-area contact.
- [ ] **Refresh:** replay identical imports and verify stable identities; remove a source feature and verify it leaves active evidence; fail halfway through a group and preserve its previous generation with a failure warning; change parcel geometry and prove derived results become stale then recompute. User-owned data remains unchanged.
- [ ] **Geometry checks:** validate area/distance against known EPSG:2264 fixtures, with declared tolerances and units. Duplicate polygons cannot inflate totals; floodway is not added again to SFHA; map simplification never changes filter results.

### Deliver map, review, and filtering

- [ ] Implement all three map groups, detail/list evidence, filters, and failure states in the application. Backend tests or a raw GeoJSON dump alone do not meet this milestone.
- [ ] **Interaction checks:** a buyer selects a parcel, inspects all three evidence groups, sees dates/uncertainty, saves or excludes the lead with a reason, adds a scratchpad note, and can set a personal follow-up. Verify map/list synchronization, independent toggles, keyboard-accessible controls and textual evidence, legend readability, and accessible status indicators. Viewing evidence or setting a reminder never records a handwritten note as sent.
- [ ] **Failure checks:** block a source refresh and show stale/failed status without removing parcel search or prior saved work. Unknown values do not satisfy favorable filters. Cross-user tests verify private lists, scratchpad, activity history, and reminders stay isolated.
- [ ] **Performance:** record request count, transfer bytes, peak import memory, spatial query plans, viewport payload, and p50/p95 map/list/detail latency on a representative local dataset. Agree interactive budgets before UI acceptance and verify repeated map interaction makes no upstream GIS calls. Investigate measured slow queries before adding tiling infrastructure or worker pools.

### Release gate

- [ ] `go test -race ./... -count=1`, `go vet ./...`, and `go build ./...` pass; database-tagged tests pass against migrated PostGIS. Run the actual application's test/build commands once its location is identified; record those commands and results rather than inventing them now.
- [ ] Source receipts, current-generation/freshness checks, geometry fixtures, and end-to-end UI evidence exist for all three groups. A source-specific gap is visibly reported; an entirely unimplemented group or unreachable unvalidated flood feed cannot satisfy release.
- [ ] Application V1's scratchpad, explicit state transitions, dated activity, do-not-contact handling, follow-up reminders, and user-isolation checks still pass. The new layers are evidence for buyer decisions, not a replacement for personal lead tracking or permission to add excluded outreach features.

## Open gates, not deferred promises

The current FEMA flood-zone query needs a reliable tested retrieval path; municipal zoning currency and authority must be documented rather than assumed; and the user-facing application implementation location is not established in this checkout. These are implementation gates with concrete checks above. None authorizes omitting one of the three committed groups.

Scope decisions and the earlier ROW correction are recorded in [the layer-planning diary](../.agents/diary/land-quality-layers.md).
