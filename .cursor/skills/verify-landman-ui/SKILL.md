---
name: verify-landman-ui
description: Verify the Landman browser application. Use for ui/, parcel discovery, map layers, parcel details, private favorites, Leads, browser authentication, or Playwright changes.
---
# Verify Landman UI

Verify the SvelteKit browser application from `ui/`. The application reads parcel evidence from the API and stores private favorites and lead states through account routes. Browser tests use fixed local inputs, called fixtures. A fixture pass does not prove live API data or private database storage.

Read `ui/package.json`, `ui/playwright.config.ts`, and the source and tests for the changed path. Use `plans/browser-parcel-ui.md` for the application contract. Use the relevant later plan for parcel-first discovery, detail preferences, or the favorites board. Plans describe requirements, not proof that each feature exists. Application README claims can be older than source.

## Checks

Run from `ui/`:

```sh
npm ci
npm run check
npm test
npm run build
npx playwright install chromium
npm run test:e2e -- --workers=2
```

Playwright builds and starts one local preview for the pass. It drives desktop and tablet projects. Require the configured port to be free. Do not reuse or stop an operator server. Tests supply signed-in and signed-out API fixtures; the application build retains authentication.

## Feature coverage

Run the full browser suite for skill maintenance. For a focused change, select every affected row. File paths below are relative to `ui/tests/`.

| Feature | Browser tests | Required result |
| --- | --- | --- |
| Authentication and account menu | `auth.spec.ts`, `operator-header.spec.ts` | Signed-out and failed requests cannot reveal the application. Retry and logout work. |
| County selection and coverage | `discovery.spec.ts`, `coverage.spec.ts` | County selection, locked states, zoom, keyboard actions, and the no-WebGL path agree. WebGL is the browser map graphics interface. |
| Parcel discovery and lookup | `parcel-finder.spec.ts`, `parcel-finder-stale.spec.ts`, `viewport-priority.spec.ts` | Lookup selects the intended parcel. Older requests cannot replace current results. Map movement updates bounded reads. |
| Filters and parcel results | `filters.spec.ts`, `discovery-evidence.spec.ts` | Default results, combined filters, clear actions, pages, map counts, and source stamps agree. Invalid input has a visible error. |
| Map layers | `map-layers.spec.ts`, `flood.spec.ts`, `basemap.spec.ts` | Layer visibility stays separate from parcel filters. Stored flood and jurisdiction evidence and local map resources work. |
| Parcel details and preferences | `parcel-evidence.spec.ts`, `detail-preferences.spec.ts`, `parcel-actions.spec.ts` | Selected evidence sections load. Saved detail fields survive reload. Private owner information stays separate. Research links have the expected targets without an external visit. |
| Favorites and Leads | `favorites.spec.ts`, `favorites-discovery.spec.ts`, `crm-conflict-retry.spec.ts`, `crm-delete-freshness.spec.ts`, `leads-refresh.spec.ts` | Favorite actions and the private board agree. State changes, pages, exclusions, removal, conflicts, and retries preserve current confirmed data. Board refresh does not remove mounted cards. |

Inspect test assertions before claiming coverage of an action. A file name alone is not evidence. Test files can cover more than one row.

## Health, evidence, and cleanup

Require UI checks and the build to pass before a browser drive. Playwright's server readiness check must pass. After a failed drive, inspect the browser log and server state. Start a fresh preview or reset the failed browser state before another action. Never continue from an unknown state.

Use [UI evidence](../../../.agents/skills/landman-ui-evidence/SKILL.md) for asserted screenshots or recorded actions. Preserve files under ignored `ui/test-results/` or `artifacts/verify/`. Check their presence before cleanup. Stop only processes this pass started. Playwright removes its preview after the pass. A preview build does not update the operator's API or UI server.

When browser requests or proxy behavior change, also run `verify-landman-api`. Its authentication, parcel API, and private favorites database lanes prove storage behavior that browser fixtures cannot prove. Use the UI origin for browser checks, not API port 8080.

## Boundaries and completion

- Block external county, FEMA, DevNet, map, and authentication requests in fixture tests. FEMA is the Federal Emergency Management Agency.
- Stored tax evidence and research links are current UI paths. Opening an external tax service or fetching operator tax files is not part of this pass.
- Live Google sign-in, full-county performance, and external data accuracy need separate approval and evidence.
- Do not claim that favorites and lead states prove notes, reminders, SMS delivery, or subdivision editing. Verify a concrete source entry point before adding those paths.

Record the Node version, package-lock state, commands, exits, executed tests, skips, screenshots, and API dependencies. State the limits of fixture and software-rendered map evidence. Keep private records, cookies, and tokens out of artifacts.
