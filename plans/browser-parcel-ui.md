# Browser parcel application

## Purpose and delivery order

Build one browser application for parcel discovery and personal lead tracking. Use the delivery order in [v1.md](v1.md): start the UI when parcel and tax ingestion work, then add owner labels and the required zoning, road, and flood evidence as those sources become available. This plan records the selected UI design; it does not claim that the application or its API exists today.

Keep the Go service, PostGIS migrations, and the UI in one repository. Put the UI in a separate folder. Build the Go service and UI as separate artifacts. Use SvelteKit for the application UI and MapLibre GL JS to draw the map. Use Skeleton for standard controls, form styles, and shared component behavior. Keep custom code for parcel/map behavior and the approved sliding-panel layout. Set the approved colors through Skeleton theme properties. Keep Google's pre-approved sign-in button artwork unchanged. This is a browser application; do not add a desktop or mobile client. Establish the exact application and API folders at the start of the UI work.

## Approved layout and colors

Use the map as the main desktop and tablet view. Let it fill the application area below a compact header. Put search over the map. Use only a left-side filter panel; do not retain the filter-bar alternative or layout comparison controls.

Use one right-hand panel. Show matching results by default. Selecting a parcel replaces the list with details. The details **×** restores the filtered list and its page. Edge arrows slide filters left and results or details right. Only arrows hide filter and results panels. Keep active-filter labels visible when filters are closed. Make each label a button that removes only its own filter, synchronizes the form field, and updates map highlights and results without opening the panel. Hiding a panel preserves filters, map highlights, and list state. Keep hidden controls outside keyboard navigation and honor reduced-motion preferences.

Use dark mode only. Use a softer graphite map background (`#24272b`), lighter charcoal panels (`#303438`), light gray text, blue match highlights, and an amber selected-parcel outline. Apply the same colors to map controls and native form controls. These layout and color choices are approved; changes require an operator decision.

The local prototype lives in `ui/` and uses fictional data. It does not establish county-scale performance or complete the application API, authentication, private records, or required source integration.

## Discovery view

Show a map with the matching-parcel list in the right-hand panel. Give both views the same county-wide filter state. The parcel finder shows address and parcel-ID suggestions while buyer types. Show suggestions without a debounce delay. Typing alone does not change the map, matches, or selected parcel. Selecting a suggestion locates that parcel and opens its details directly; do not require a separate Go action. The list includes all county matches in stable, paged order; it must not depend on the current map area. Show the county match count. Let the buyer switch the list between all matches and matches in the current map view, and label the two counts clearly.

Keep all visible parcel boundaries as neutral map context, whether they match or not. Highlight matching parcels only when at least one filter is active. With no active filters, show zero matches in the list and no match highlights. Search and map selection can still inspect any parcel without adding it to the filter results. Request this context as bounded viewport geometry in production, not a full-county browser download. Apply the same filters when the buyer moves the map; do not require a second Apply action. At county-wide zoom, show match counts or density instead of every parcel outline. Show individual colored parcel shapes when the buyer moves closer. Keep the selected parcel outline distinct from filter colors. Give every color and highlight a text label in the legend and list; color alone must not carry meaning.

Support combinations of parcel and evidence filters as data becomes available. Examples include the provisional `likely_vacant` signal, the later validated vacancy tier, mailing outside North Carolina, entity-name matches, and corporation-name matches. Follow [parcel-owner-labels.md](parcel-owner-labels.md): an out-of-state mailing address does not prove owner residence; an LLC is an entity match, not a corporation match. Display unknown and stale states separately. Do not turn a missing result into a favorable match. Use stored parcel IDs to join list rows, map shapes, and details.

Let the buyer use Tab to reach list controls and use keyboard controls or a click to select a result. A list selection highlights its parcel on the map and opens its detail panel. If that parcel is outside the map view, move the map to it. A map selection selects the same parcel in the list. Keep list and map selection synchronized when filters or map position change. Provide a usable text list and parcel details when the map cannot be used.

## Parcel review and actions

Open a parcel detail and action panel when the buyer selects a parcel. Show source facts, dates, and uncertainty beside the relevant facts. Show available tax evidence and, as required sources arrive, zoning and jurisdiction, road proximity, and flood evidence. Use the map groups, legends, and evidence rules in [parcel-qualification-layers.md](parcel-qualification-layers.md). Do not imply that mapped evidence certifies legal access, flood safety, or permitted use.

In the panel, let the buyer save the parcel to a private list, add a scratchpad entry, set an explicit lead state, record dated personal activity, exclude a lead with a reason, and set a personal follow-up reminder. Show the reminder queue and controls to snooze, reschedule, and cancel. Follow the reminder and user-isolation contract in [v1.md](v1.md). The buyer writes handwritten notes and contacts owners outside the application. Do not generate owner messages, mailing files, or campaigns.

For a saved parcel, show a **Request tax info** action. Each explicit action makes one bounded DevNet parcel-page request; saving, viewing, filtering, or moving the map does not make that request. Store each successful displayed observation for that buyer and parcel with its source URL and observation time. Show the observation as dated evidence, not a current payoff quote. Show denied, unclear, or failed results as errors, not zero balances. Apply the request and access limits in [tax-data-ingest.md](tax-data-ingest.md#saved-parcel-request-tax-info-action). Define retention for repeated explicit requests and removal of a saved parcel during implementation. Do not put private observations or personal lead data in shared map data.

### External parcel research links

Deliver these three browser links together in the parcel detail panel. Make them available without saving the parcel. Open each external page only when the buyer selects its link.

- **Open in Google Maps (satellite).** Show the link for every parcel with valid geometry, regardless of vacancy tier or site address. Center the view on a point inside the stored parcel boundary. Do not embed imagery. The external view does not show the Landman boundary or guarantee current imagery.
- **Open Union County property record.** Use `https://property.spatialest.com/nc/union#/property/{parcel_id}`. Verify the link with a real county parcel before release.
- **Open Union County tax record.** Use the DevNet parcel page, for example `https://unionnc.devnetwedge.com/parcel/view/09336350/2026`. Use a verified tax year for the record. Do not hard-code the example year or assume the current calendar year is available.

Use the county parcel identifier for the property and tax links. Preserve leading zeros. Do not use the Landman UUID or an unverified fallback identifier. Omit a link if its required data is missing. Do not send owner data or private lead data. These links do not authorize scraping, prefetch, or server-side requests. The DevNet link is separate from the saved-parcel **Request tax info** action. Defer that action and its database storage to a later delivery. Opening a link does not change vacancy tiers or save an observation.

## Map and performance contract

Use PostGIS for county-wide matching, counts, list pages, and full-resolution spatial calculations. The browser must not load every county parcel to calculate a filter. Request bounded geometry for the visible area from stored data; do not query county or FEMA services on each map move or parcel selection. Limit response size, cancel obsolete map requests, and simplify display shapes by zoom. Display simplification must not change filter results or evidence calculations.

Start with bounded viewport geometry queries. Keep the map-data boundary suitable for a later vector-tile endpoint if measurements justify it. Vector tiles are small map-data files for one map area and zoom level. Do not add a separate tile service by default. If measurements show that tiles are needed, use only the small public parcel IDs and status fields needed for map styles; cache by source generation. Fetch private actions and parcel detail through authenticated API requests. Keep source generations and freshness consistent between displayed shapes and parcel facts.

Before UI acceptance, agree target budgets and measure request count, response size, browser drawing time, and p50/p95 latency for initial map load, map move, filter change, list page, and parcel selection with a representative Union County dataset. Test county-wide and close zoom, repeated moves, and combined filters. Investigate slow queries and large map responses before adding tile infrastructure. Do not describe a framework choice alone as proof of map speed.

## Acceptance checks

- [ ] The UI and Go service build as separate artifacts in one repository. The UI runs in a browser and uses one application API and map stack.
- [ ] One county-wide filter produces the same match IDs and county count for list and map. Moving the map shows matches in the new view without changing the filter. The all-matches and in-view list modes show their different counts.
- [ ] At county-wide zoom, the map shows aggregate match information; at close zoom, it shows matching parcel shapes. The selected parcel remains distinct. Legend text and list labels explain each highlight.
- [ ] Keyboard or click selection in the list selects and locates the parcel on the map. Map selection selects the same list item. The buyer can inspect a result without using map controls.
- [ ] A selected parcel opens details, a private scratchpad, lead actions, and personal reminder controls. Source dates, unknown states, and stale evidence are visible. A source refresh does not change private buyer records.
- [ ] Request tax info is available for a saved parcel only. Each explicit action makes one request. Save, view, filter, and map movement make no DevNet request. Stored observations are dated, private, and separate from county tax files; a failure is not a zero balance.
- [ ] Verify all three external research links in the same delivery. Test an unsaved parcel, valid and missing link inputs, a county parcel number with leading zeros, and a supported DevNet tax year. Verify that opening the parcel panel does not contact these sites.
- [ ] Full-resolution stored data determines filters and spatial evidence. Visible geometry is bounded and simplified for display. Repeated map work makes no live county or FEMA request.
- [ ] Measure and record the agreed interactive budgets with representative county data. Add vector tiles only when measured results justify them. Verify user isolation, keyboard use, and the end-to-end map-to-follow-up workflow.
