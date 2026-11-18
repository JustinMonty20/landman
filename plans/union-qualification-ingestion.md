# Union qualification ingestion

## Objective
Store Union County zoning, jurisdiction, county-boundary, and road features as retained source generations. Store parcel intersections and road distances as typed evidence. The buyer must be able to tell mapped facts from gaps and stale results. This PR does not add a buyer-facing API or map.

## Scope
- Use the existing Union GIS HTTP client for all county requests. The parcel, soil, zoning, and roads jobs share a cross-process county-host exclusion lock. Keep separate zoning and roads calendars.
- Publish all zoning, jurisdiction, and boundary layers as one group. Publish the roads layer as another group. A failed scan does not replace a head. Keep a failed refresh visible.
- Retain source IDs scoped by layer and generation, original source SRID, raw district codes, supported descriptions, safe links, source dates, and full 3857 shapes. Keep a spatial index for each shape table.
- Reconcile parcel assessments after successful source or parcel jobs. Also reconcile on schedule startup, the independent `UNION_QUALIFICATION_CRON` calendar, and `run union.qualification.assess`. A crash after source publication must not mark an old result current.
- Use EPSG:2264 for area and road distance. Keep every positive-area zone and jurisdiction hit. Union overlapping polygons before measuring uncovered or ambiguous acreage. Search roads within one mile and keep ties within 0.01 foot. Contact at zero feet does not prove access.
- Read assessments in a repeatable-read snapshot. Compare parcel shape fingerprints, source heads, method versions, source age, and refresh failures. Return stale reasons rather than presenting old evidence as current.

## Checked county mappings

The established Go GIS client returned metadata and two-feature samples for each of the 15 layers. Fixtures in `internal/connector/union/testdata/qualification/` retain only qualification attributes and geometry. Earlier Python requests returned HTTP 403; that did not establish source unavailability. `OBJECTID_1` identifies Fairview features, while the other layers use `OBJECTID`. Zoning district fields are `ZONE` for Union, Fairview, Weddington, and Wingate; `NEW_ZONIN` for Indian Trail; `LP_Zone` for Lake Park; `Marvin_Zon` for Marvin; `NEW_ZONING` for Monroe; and `CurrentZon` for Stallings. Marshville and Waxhaw samples show `Zoning` as district code and `ZoneDesc` as description. ETJ and municipality use `NAME`; roads use `FullName`, `FTR_CODE` when `RoadClass` is blank, `STATUS`, and `DateUpdate`. Metadata and sample attributes do not verify current legal authority or complete county coverage.

## Acceptance checks
- Local fixtures cover every layer mapping, paging limits and drift, duplicate IDs, schema changes, ArcGIS errors, cancellation, and invalid geometry. Captured layer 190 OID 229169 and layer 195 OID 116697 have ring self-intersections. In EPSG:3857, polygonal `ST_MakeValid` yields valid nonempty shapes with raw/fixed areas 484475569.7632419/484475569.76324135 m² and 26260261.30723843/26260261.307238452 m². Absolute area changes are 5.364418e-7 and 2.235174e-8 m²; both symmetric-difference areas are zero in the local PostGIS probe. County polygon repairs preserve original EWKB, layer/OID/SRID, status, and measured delta. Only valid nonempty polygonal outputs with absolute area delta at most 0.01 m² **and** relative delta at most 1e-8 are published; every other invalid polygon fails the generation without replacing its head. This bound is a source-data guard, not a guarantee of unchanged topology or legal accuracy. The captured samples, not full county features, establish these observed deltas. Sanitized captured county samples must not include owner or parcel contact fields.
- A migrated disposable PostGIS database checks polygon splits, overlapping and missing coverage, holes, zero-area contact, road ties, county-edge warnings, group failure, retired features, and stale then recomputed results.
- The offline race suite, vet, and build pass. The CI PostGIS lane runs qualification package tests.

## Verification and operating limits

Use `docs/union-jobs.md` for the five independent calendars, manual job IDs, and downstream retry. An ordered page scan checks count and transfer limits, but it is not a transactional county snapshot. The ten-minute job deadline may be too short for a full county scan; do not raise it or claim county-scale performance without an approved live run. `docs/union-qualification-verification.md` records local checks and database constraints. Failed source refreshes preserve the old head and mark its evidence stale. Source-generation replay creates a new generation; the layer-scoped source IDs remain stable within successive generations but no global identity across future county schema changes is verified.

## Boundaries and release gates
This PR does not ingest FEMA, add application routes or UI, or change private lead records. The original [qualification plan](parcel-qualification-layers.md) and [V1 plan](v1.md) still require FEMA and buyer-facing delivery. An ordered ArcGIS scan with stable counts is not a transactional county snapshot. Official zoning authority and its update date require verification before release. No county-scale live import is approved for this PR.
