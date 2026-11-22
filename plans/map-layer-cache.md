# Cache every map layer by source revision

**Status:** Proposal for review. No implementation or deployment change is approved.

## C1. Decision in brief

Use one cache contract for every map layer and each supported zoom. Choose the storage format per data type. Keep PostGIS and the retained source artifacts as the sources of record.

Use fixed tiles for map geometry that changes with the viewport. A tile is a display result for one zoom level and map area. Adjacent views reuse tiles. Keep a small county overview where the UI needs one. Treat the overview as another view of the same layer, not as the limit of the design.

Start with a bounded cache in the Go API. Compare compressed GeoJSON tiles with Mapbox Vector Tiles, a binary format for lines and polygons. Measure before choosing the wire format or adding storage infrastructure. Keep existing viewport routes during the comparison. Do not round their bounds to manufacture cache hits.

## C2. Every layer has a cache policy

| Layer | Display unit and revision | What stays separate |
| --- | --- | --- |
| County outlines | Keep the existing static asset. The UI build identifies its revision. No PostGIS lookup. | If an authoritative county boundary becomes a separate map layer, key it by its published generation. |
| Parcels | Cache neutral geometry with stable parcel IDs in close-zoom tiles. Cache neutral density in fixed cells at low zoom. Use a committed parcel display revision. | Parcel details and exact search results use the existing API. Do not send a full county of parcel outlines. |
| Parcel match highlights and counts | Cache repeated results only for validated filters with the same semantic values. Include exact bounds or the defined grid unit. Include a matching-data revision for all data the query reads. | Uncached results use the same predicates as the list. Keep results that depend on a user out of the shared cache. |
| FEMA flood zones | Cache overview and close-zoom geometry by active FEMA release capture ID. | Parcel flood facts and freshness remain separate. An empty display tile does not prove that an area has no flood risk. |
| Zoning and jurisdiction | Cache each display layer at its supported zooms. Both use the published zoning group generation. They can share an encoded tile if the UI can switch each layer independently. | Keep missing geometry and refresh status outside the cached geometry. |
| Roads | Cache overview and close-zoom geometry by the active road generation. | Read road distance from the parcel assessment. Do not compute it from display lines. |
| Soil polygons | When the soil map exists, cache polygon tiles by the active soil set. Use low-zoom summaries if required. | Parcel screening uses its own run ID and freshness. If a later display layer colors polygons by USDA ratings, include the applicable screening revision in its key. |
| Structures | If a map layer is approved, use tiles keyed by the accepted structures observation. | Do not label structures or assessed values as verified vacancy evidence. Assessed values and tax are match inputs, not geometry sources. |
| Elevation or slope | If the terrain plan is approved, cache rendered raster tiles by release ID, display style revision, and tile coordinates. Raster tiles store a grid of cells. Read source windows from the stored raster, not from USGS. | Keep native-resolution analysis and unknown cells intact. Do not encode terrain as vector polygons or add terrain to the V1 release requirements. |

Selection outlines use the existing parcel geometry in the browser. They do not need a separate server cache. A new map layer must declare its revision, supported views, access rules, and behavior when data is absent. This spec does not add a new source or map layer.

## C3. Compare the choices

| Choice | Work it removes | When to use it |
| --- | --- | --- |
| Browser session memory | Repeat HTTP requests for a tile or overview within one tab | Use a bounded cache if the trace shows repeat views. Tie entries to the authorized session and display revision. Clear them on logout. |
| Go process memory | Repeat PostGIS spatial queries and encoding | Start here for database-backed layers. A restart empties the cache. Each API process has its own copy. |
| Precomputed PostGIS display geometry | Repeat simplification or aggregation on a cold request | Use if cold work dominates. Reads still reach PostGIS. Neutral summaries cannot replace filtered counts. |
| Prebuilt tile archive | Spatial queries for geometry, including cold requests | A packed file stores many tiles in one archive. Compare file size, build time, and read latency. Use it if cold requests or cache fills remain costly. Keep delivery authenticated. |
| Redis or a shared HTTP cache | Duplicate fills across API processes | Consider only if measurements show that multiple processes repeat enough work to justify a shared service. A shared service does not improve reuse for arbitrary bounding boxes. |

An HTTP entity tag can reduce transfer with `304 Not Modified`. The request still reaches the API. It saves spatial work only if the API can validate the tag before it runs the spatial query. Keep authenticated HTTP responses `no-store` in the first phase.

## C4. Proposed architecture

**Shape A: Produce display units on demand.** Add a private cache to `internal/parcelapi`. The map handler resolves the active revision in a read transaction. It requests an encoded display unit by a typed key. On a miss, the layer query reads the same snapshot. The cache owns byte limits, eviction, and control of duplicate fills. The handler owns access, source queries, and live status. Use the existing map stack. Do not add a public cache interface or a separate tile server.

**Shape B: Build display units after publication.** A display job reads a completed source revision and writes a versioned overview or tile archive. A manifest identifies the exact source revision and display format. The API serves the asset only when the manifest matches the active revision. If the build fails, keep the source publication and use the bounded query. This shape saves more cold work. It adds build jobs, file retention, and build status.

Choose Shape A first. Verify the same key and freshness rules on a parcel tile and an overlay tile before implementing the other layers. Those are early checks, not the completion gate. Keep the key independent of the encoding. FEMA supplies a release ID, parcels supply a display revision, and future terrain can supply a raster release ID. Shape B can use the same keys if measurements justify prebuilt assets.

## C5. Cache contract

- **Key.** Use the county, layer, source revision, display format version, and view. A view is a fixed tile, a county overview, or an exact validated query. Include style or band rules when they change the bytes. Keep neutral geometry keys separate from filter results.
- **Revisions.** Change the parcel display revision when any committed change alters displayed parcels or their geometry. Include parcel corrections from replay and repair. Match keys must cover parcel fields and every filter dependency, including tax and owner labels. Verify the existing source stamp against those dependencies before reuse. Bypass the cache when the required revision is not available. Keep the revision consistent with reads in the same database snapshot.
- **Publish.** A new revision uses a new key. A late fill cannot overwrite the result for the new revision. A failed refresh can keep the same geometry key. Read the failure and freshness status separately. A missing active reference returns unavailable, not a cached prior layer.
- **Bound.** Start tests with a 32 MiB cache limit per process and the existing 1 MiB limit for viewport responses. Include entry overhead. Bound concurrent fills and the number and bytes of tiles requested for a view. Combine concurrent misses for the same key. Do not store errors, oversized responses, or incomplete results. Set final limits from the baseline.
- **Protect.** Check the session and layer access before a shared cache read. Share only fields that all authorized users of the layer can read. Never put owner details, balances, private notes, user records, or parcel reports in shared display tiles. Keep session revocation and permissions independent of cached geometry.
- **Compose.** Use stable feature IDs where the browser must identify features across tile boundaries. Check tile edges for gaps and duplicate selections. Check that neutral geometry and match results refer to the same parcel display revision. Preserve exact counts and parcel-list agreement. Do not use display geometry for measurements, eligibility, or evidence.
- **Recover.** On restart, eviction, or cache disablement, use the bounded source query. If the API cannot validate the active revision, return the existing service-unavailable result. The first phase removes spatial queries on hits. It does not remove session checks or the active-revision read.

## C6. Evidence required before adoption

Use a disposable copy of representative Union County data. Do not call live sources. Use the same trace for baseline and candidate results. Include first load, county view, close-zoom panning, return to view, layer switches, filters, and parcel selection. Test one and four concurrent users. Report cold and warm results per layer. The [historical API benchmark](../docs/parcel-api-performance.md) measured roughly 793 ms for the county parcel grid and 15 ms for close parcel shapes. It did not measure the current overlays or browser draw time.

| Check | Proposed pass condition |
| --- | --- |
| Every layer and zoom | Verify the county asset and every current database-backed map layer. Cover county views and close views wherever supported. Demonstrate tile reuse across adjacent views. Verify that a revisited tile or overview makes no spatial query after the first fill. Count session and revision reads separately. |
| Correctness | Compare the source and cached results within the display simplification rules. Test stable selection, tile edges, exact filter counts, unavailable data, failed refresh, revision changes, late fills, and session revocation. Check parcel replay and a tax-only or owner-label-only change separately. |
| Latency and resources | Record p50 and p95 latency, network bytes, request count, spatial queries, fill time, browser draw time, cache bytes, and peak memory. Candidate limits are 30% lower warm p95 for repeated display reads and at most 10% slower cold p95. Set absolute interaction limits after the baseline. If a layer misses the limits, change its representation or policy before adoption. Do not drop it from the plan. |
| Storage format | Compare GeoJSON and binary vector tiles with the same visible data. Measure encoded bytes, browser draw time, and encoding cost. Compare archive size and build time only if the cold path justifies prebuilt assets. |

## C7. Decision for review

Approve a common tile-capable contract and measure the current layers before selecting a storage format. Choose a Go cache first unless the trace shows that cold builds dominate. Keep the [V1 qualification and freshness gates](v1.md), the [browser verification gates](browser-parcel-ui.md), and the independent source schedules. Do not treat cache readiness as source readiness.
