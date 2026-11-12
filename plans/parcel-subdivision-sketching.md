# Parcel subdivision sketching

## Outcome
Let the buyer explore a highlighted parcel by drawing, moving, and deleting proposed dividing lines on the map. Show the resulting conceptual lot areas and counts so the buyer can compare possible layouts and decide what to investigate further.

This is a planning aid, not a subdivision application, survey, legal boundary, development approval, or guarantee that lots are buildable or sellable. Sketches never modify source parcel geometry or lead records.

## Product boundaries
- Start from one selected parcel and its authoritative boundary. Keep the parcel identity and source acreage visible throughout the exercise.
- Use editable linework as hypothetical cuts. A user can add a line, drag its endpoints/control points, reposition it, and delete it. Provide undo/redo and reset-to-original.
- Derive proposed lot polygons only where cuts form valid partitions within the parcel. Make incomplete, dangling, crossing, or invalid cuts visibly unresolved rather than implying they create lots.
- Display each resulting lot's area in acres and the total lot count. Show unallocated/remainder area or geometry errors where applicable; totals must reconcile to the parcel area within a stated tolerance.
- Allow comparing layouts for the current session; persistence/sharing is not assumed. If saved sketches are later desired, keep them separate from official parcel geometry and buyer lead activity.
- Do not infer zoning compliance, minimum lot size, frontage, access, septic suitability, utility availability, flood safety, or legal subdivisibility from the drawing. Existing qualification evidence can be viewed as context, not treated as approval.

## Per-lot evidence overlays
Recompute available parcel evidence against each generated lot as the sketch changes. Show both map geometry and a per-lot summary where the source supports it. These are screening/context results, not a buildability score or regulatory determination.

- **Flood:** show intersecting mapped flood categories and affected acreage/share for each lot; distinguish SFHA, mapped 0.2% annual-chance areas, and regulatory floodway where classified. Include source effective-date and coverage/freshness status. Distinguish verified no mapped overlap from unknown, stale, or unavailable evidence.
- **Soils:** show intersecting soil map units and mapped limitations/area for each lot using the available use-specific screening evidence. Do not present this as septic approval, engineering suitability, or buildability certification.
- **Zoning and jurisdiction:** show mapped zoning districts and municipal/ETJ evidence crossing each lot, including split, conflicting, unmapped, or stale evidence. Do not infer permitted use, minimum lot size, subdivision eligibility, or density from a district code.
- **Road proximity:** show nearest mapped road, distance, and any mapped line contact/intersection for each lot when available. A line touch or zero distance does not establish frontage, an easement, maintenance responsibility, or legal access; county-edge search coverage may be incomplete.
- Each layer can be toggled independently. Keep source attribution, source dates, retrieval freshness, and unavailable/stale states visible. Selecting an evidence result highlights its contributing geometry without obscuring the sketch or authoritative parcel boundary.
- Parcel-level owner, sale, and tax facts remain parcel context rather than subdivision constraints unless a later product decision defines a useful per-lot interpretation.

## Interaction and evidence
- Enter the tool from a selected/highlighted parcel and keep its boundary emphasized above map overlays.
- Draw and edit on the map with keyboard-accessible controls and a non-map way to inspect lot count, acreage, and per-lot evidence. Clearly distinguish sketch lines and generated lots from official parcel boundaries.
- Show area units, calculation method/projection, and precision; use an appropriate local projected coordinate system for area calculations rather than display-map coordinates.
- Handle multipart parcels, holes, cuts ending at boundaries, and invalid/self-intersecting geometries explicitly. Never silently drop parts or report a misleading zero-lot result.
- Keep source parcel acreage separate from computed sketch area and disclose any small numerical difference caused by geometry processing.

## Open product decisions
- Whether sketches should be session-only or saved against the buyer's private account.
- Whether the interaction is limited to straight cut lines or supports curved boundaries and more advanced construction tools.
- Whether any lot-size targets are user-entered reference guides; no ordinance-derived legality checks are included without a separately verified rules source and explicit scope.
- Whether the first release supports multiple simultaneous parcel sketches or one active parcel at a time.

## Acceptance checks
- [ ] On a selected parcel, a user can draw, move, and remove proposed dividing lines, undo/redo changes, and reset the sketch without changing the source parcel.
- [ ] Valid partitions produce individually labeled lot areas in acres and a correct lot count; incomplete or invalid linework is clearly reported and does not produce false lots.
- [ ] Computed lot areas plus any explicit remainder reconcile to the source parcel area within a documented tolerance, including multipart and holed geometry fixtures.
- [ ] Area calculations use a suitable projected CRS and are independent of map zoom/display projection.
- [ ] The map and accessible textual summary distinguish hypothetical geometry from authoritative parcel boundaries and show the source acreage alongside sketch acreage.
- [ ] As lines change, per-lot flood, soil, zoning/jurisdiction, and road-proximity evidence updates where available; each result shows relevant acreage/share, source freshness, and uncertainty/coverage status.
- [ ] Flood summaries distinguish mapped categories and verified no-overlap from unknown, stale, or unavailable evidence; soil summaries remain screening evidence; zoning does not imply permitted subdivision; road contact/distance does not imply legal access.
- [ ] Overlay toggles work independently, and selecting a result highlights its contributing evidence geometry without confusing it with the sketch or official boundary.
- [ ] The UI makes clear that results are conceptual and do not establish legal or regulatory approval, access, buildability, or saleability.
- [ ] Sketching does not alter parcel source data, saved lead state, scratchpad, or qualification evidence.
