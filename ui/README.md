# County coverage UI

## Run

Use Node.js 22.12 or later and npm. Configure Google sign-in and the authentication migration. See [application login](../docs/application-login.md).

Start the API from the repository root:

```sh
make -s api
```

Start the UI in a separate terminal:

```sh
cd ui
npm ci
npm run dev
```

## County coverage

The map shows nine county outlines. Union, North Carolina, has ingestion configured. Parcel display is not connected yet. Anson, Mecklenburg, Cabarrus, Gaston, and Stanly in North Carolina, plus Lancaster, York, and Chesterfield in South Carolina, show **Locked — ingestion not configured**.

The initial regional view shows all nine county outlines and locked labels. Select a county outline or label to enter its county view. County labels are buttons and support keyboard selection. The map has no county dropdown. Selection sets the search area. Zoom and map movement do not select another county. Use **Show all counties** to restore the regional view. Use **Locate selected county** to return to the selected county.

Map zoom below 9 shows the regional view. Zoom from 9 to below 13 shows the county view. Zoom 13 or greater shows the parcel view. Regional view hides parcel controls without clearing filter values or panel preferences. A compact summary shows the selected county and filter count only when filters are active. County labels are hidden at parcel zoom to reduce map clutter. Locked counties show their explanation at all zoom levels, without parcel controls.

When WebGL is not available, use **Show on map** links on the coverage page to select a county. WebGL is the browser graphics interface used by the map.

No parcels are shown. The page does not use the fictional parcel fixtures. Union county and parcel views show search, the left-side filter panel, and the right-side results panel. Parcel search is disabled until the Union parcel API is connected. The grouped filter layout is approved. Until parcel data is connected, its controls change selections only, not parcel results or map highlights. A short data-availability notice remains; mock and preview labels are removed. Use the edge arrows to hide or show filters and results. The results panel states that results are unavailable, not that a search found zero parcels. County counts or density at county zoom and bounded parcel geometry at close zoom still require the parcel API connection. Existing ingestion jobs are unchanged. A configured status does not assert that a source is current or that an import succeeded.

## Filter design

The left panel groups fields into expandable **Owner**, **Soil**, **Tax**, and **Parcel** sections. Multiple sections can remain open. Long content scrolls inside the panel. The selection count and **Clear all** action remain at the bottom.

Owner includes **Business-name match** and **Out-of-state owner**. Out-of-state uses the owner's mailing address outside North Carolina. A business-name match includes LLC terms; it does not verify registration. Tax includes reported delinquency and foreclosure records. Parcel includes acreage and provisional vacancy. Section headings show selection counts. Selection buttons remove one field at a time.

Soil shows a disabled **Soil type** dropdown. Real choices must come from connected county soil data. The panel supplies no invented soil types or text search.

Selections remain when sections close, the panel hides, or the map returns to regional zoom. A compact button appears only when filters are selected. It opens the filter panel again. Controls make no parcel API requests until data integration. Real filter semantics, available fields, soil-overlap rules, source dates, and unknown states remain integration work.

## Coverage page

Open **County coverage** from the map to visit `/coverage`. This authenticated page contains the full county list, ingestion configuration, parcel display availability, and map legend. Ingestion details do not appear in the map results panel. The map keeps only county selection, locked labels, and a short notice for a selected locked county.

Each **Show on map** link opens that county through a validated `county` query parameter. **Return to map** retains the selected county. Unknown county values use the default regional view. The coverage page does not fetch ingestion history or assert source freshness.

## Boundary source

`src/lib/data/counties.json` and `src/lib/data/chesterfield-stanly.json` contain local GeoJSON from the U.S. Census Bureau TIGERweb county service. GeoJSON is a JSON format for geographic shapes. The source was retrieved on 2026-11-17:

- Service: <https://tigerweb.geo.census.gov/arcgis/rest/services/TIGERweb/State_County/MapServer/1>
- Original query: `GEOID IN ('37179','37007','37119','37025','37071','45057','45091')`
- Chesterfield and Stanly query: `GEOID IN ('45025','37167')`
- Fields: `GEOID,NAME,STATE,COUNTY`
- Output: `f=geojson`, `outSR=4326`, `geometryPrecision=5`, `maxAllowableOffset=0.001`

The boundaries are simplified for display. Do not use them for legal boundaries or parcel evidence calculations. The browser makes no Census, county, tile, or font requests. County labels are local HTML buttons. County availability is explicit UI configuration, not an ingestion status API.

## Browser debugging and profiling

Use the [Landman UI CDP skill](../.agents/skills/landman-ui-cdp/SKILL.md) to inspect browser errors and profile Svelte UI actions in a local Chromium session. Use the [Landman UI evidence skill](../.agents/skills/landman-ui-evidence/SKILL.md) for fixture-based QA screenshots and action recordings.

## Checks

```sh
npm run check
npm test
npm run build
npx playwright install chromium
npm run test:e2e
```

Browser checks cover desktop and tablet layouts, authentication, county selection, locked states, panel controls, local map resources, and unavailable WebGL. Screenshots are written to the ignored `test-results/` directory.

## Boundaries

This is a county coverage view, not a completed parcel application. Real parcel queries, private lead records, tax requests, and reminders are not connected. Required zoning, road, and flood evidence and county-scale performance checks remain in [the browser application plan](../plans/browser-parcel-ui.md) and [the V1 plan](../plans/v1.md). This change does not alter those release gates.

Keep standard controls in Skeleton styles. Keep the approved dark palette and Google's pre-approved sign-in artwork unchanged.
