# Parcel discovery prototype

## Run

Use Node.js 22.12 or later and npm. Configure Google sign-in, apply the authentication migration, and start the Go application API first. See [application login](../docs/application-login.md).

From the repository root, start the API in a separate terminal:

```sh
make -s api
```

```sh
cd ui
npm ci
npm run dev
```

Open the local URL printed by Vite. The map fills the application area. Filters are closed at startup. The right-hand panel shows all matching results. Filters use only the left-side panel. Use its edge arrow (**Show filters**) to open it. The layout is fixed; no comparison controls remain. The UI uses a softer graphite map background (`#24272b`), lighter charcoal panels (`#303438`), blue match highlights, and an amber selected-parcel outline. Dark mode is independent of the operating-system preference.

## Review

Start with zero filter matches. All sample parcel boundaries remain visible in neutral gray. An active filter highlights its matches in blue and lists them on the right. Clearing filters removes highlights and results, not parcel boundaries. Search and map clicks can inspect any parcel; a selected parcel has an amber outline even if it is not a filter match.


- Type `DEMO-001` or `Sample Oak Lane`. Select a suggestion to locate the parcel and open its details directly. Typing does not change the map or filter matches. No separate Go button is required.
- Combine acreage, likely-vacant, mailing-outside-NC, and LLC-name filters. The right-hand list and map show the same matching parcels. The selected parcel has a separate outline. Filters remain open while you select a parcel.
- Select a result with a click or the keyboard. Parcel details replace the list in the right-hand panel. Close details with **×** to restore the filtered list and its page. Select a map shape to inspect the same parcel. The other matching parcels remain visible on the map.
- Use only the right-edge arrow (**Hide matches**) to hide the list. Use the right-edge arrow (**Show matches**) to restore it. Use only the left-edge arrow to hide filters. Panels slide toward their screen edge. Reduced-motion preferences disable the animation. Filters, map highlights, list mode, and page do not change. Selecting a map parcel still opens details; closing details restores the list.
- Move or zoom the map. Compare all matches with matches in the map view.
- Close and open the side panel. Active filters remain visible. Click an active-filter label, or focus it and press Enter, to remove only that filter. Removing the last filter restores zero matches while parcel boundaries remain visible.
- Test desktop and tablet widths. The results list and details remain usable without WebGL.

## UI components

Use [Skeleton](https://www.skeleton.dev/) for standard UI controls. `@skeletonlabs/skeleton` supplies button and form styles. `@skeletonlabs/skeleton-svelte` supplies the result `Pagination` component. Tailwind CSS and its forms plugin compile the shared styles.

Skeleton styles native inputs, checkboxes, and selects; these do not need custom Svelte wrappers. Application-specific map rendering, result rows, and sliding panels remain custom. The Google button keeps Google's pre-approved artwork.

`src/theme.css` sets the approved charcoal and blue colors through Skeleton theme properties. `src/style.css` contains the map layout and application-specific styles. Dark mode is fixed with the `dark` class and `color-scheme: dark`, independent of the operating-system preference. Prefer Skeleton components over new custom standard widgets.

## Google button

The login screen uses Google's unmodified, pre-approved dark button artwork. The local file is `static/brand/google-signin-dark.png`. It is the Android + Web, dark, square, text-visible, 2x PNG from [Google's asset archive](https://developers.google.com/static/identity/images/signin-assets.zip). Display it at 180 by 40 pixels. Follow [Google's branding guidelines](https://developers.google.com/identity/branding-guidelines).

The button uses the existing server-side Google login route. It does not load an additional identity SDK, request calendar access, or make an external image request.

## Checks

```sh
npm run check
npm test
npm run build
npx playwright install chromium
npm run test:e2e
npm audit
```

Browser tests use desktop and tablet sizes. They check that the default map uses more than 90% of the screen height and that side panels use less than 40% of the screen width. Tests use a separate local port so they can run while the development server remains available. Screenshots are written to the ignored `test-results/` directory. The static build is written to the ignored `build/` directory. No Go build is required for this prototype.

## Boundaries

All 24 parcels, IDs, owners, addresses, dates, and boundaries are fictional. The map uses MapLibre with local geometry and no external tiles. The acreage values do not describe the display geometry. This is a UI prototype with a fixed side-panel layout, not a county map or a source-evidence assessment.

Filtering runs in the browser only for this small sample set. This is not the production county-query design. County-wide queries, bounded map geometry, source-generation consistency, and measured performance budgets remain required by `plans/browser-parcel-ui.md`.

The application API is in `cmd/api/`. Google sign-in, the allowed-account list, user identities, and revocable database sessions are implemented. The UI checks `/api/me` before showing discovery. Server-only settings stay in the repository-root `.env`. Browser tests use explicit authentication fixtures, not an application bypass.

Private lead storage, tax requests, calendar access, and reminders are not implemented. The UI makes no county or FEMA requests. Reloading clears sample discovery changes. The authentication migration adds tables without changing parcel or tax data.

Zoning, road, flood, and tax evidence are not connected. Positive filters exclude unknown evidence. Mailing state does not prove owner residence. An LLC-name match does not prove registration or corporate status.
