# Show all parcels and narrow the results with filters

## Scope

Show all stored Union County parcels in the paged list when no filter is active. Show parcel counts at county zoom and bounded outlines at close zoom. Use the same filters for the map and list. Hide parcels that do not match. Keep layer visibility separate from the parcel filter. Retain the approved flood contact rules and the existing parcel detail panel.

This decision replaces the zero-results default in the [browser UI plan](browser-parcel-ui.md) and the [parcel API plan](parcel-data-api.md). The current API still has that default. Preserve it for existing API clients unless the API contract explicitly changes.

## New filters

- **Reported foreclosure record.** Match a parcel if it has a record in either published Union County foreclosure feed. Use the current file membership and parcel links. A match does not establish an open legal case, a current balance, or an upcoming sale.
- **Soil type.** Select a named USDA map unit by its stable key. Match parcels with positive-area overlap with that unit in the active stored soil set. Shared edges and points do not count.
- **Soil rating.** Select an intended use and a published USDA rating. Match parcels with positive exclusive acreage in that rating for that use. Use the current stored assessment. Missing, stale, unrated, uncovered, and ambiguous areas do not count as known rated acreage. A match does not establish site suitability.
- **Other filters.** Connect the supported acreage, owner indicators, reported delinquency, provisional vacancy, flood, and jurisdiction filters. A parcel must match every selected filter. One parcel appears once even if several source records match.

## Verification

- Verify that the default paged list includes unfiltered parcels. Verify that a filter removes nonmatching rows and outlines, and that clearing it restores the default.
- Verify matching identities and counts for equivalent list and map bounds. Test combined filters and cursor invalidation after a relevant publication changes.
- Use disposable PostGIS fixtures for both foreclosure feeds, superseded file membership, soil overlap, boundary-only contact, stale and missing ratings, and ambiguous soil coverage.
- Measure county counts, page reads, and bounded map views on representative data before release. Keep full-county geometry out of the browser.
- Rebuild Svelte. Use `/landman-ui-evidence` to record asserted desktop and tablet states for the default, a combined filter, and clear.
