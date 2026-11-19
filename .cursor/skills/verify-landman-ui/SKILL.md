---
name: verify-landman-ui
description: Verify the Landman SvelteKit browser application, MapLibre prototype, and browser authentication behavior. Use for ui/, Svelte components, Vite, MapLibre, browser routing, or Playwright changes.
---
# Verify Landman UI

Verify the browser application from `ui/`. Read `ui/README.md` and `plans/browser-parcel-ui.md` before selecting checks. The current UI uses fictional parcel data. It does not prove county-scale query behavior or private lead workflows.

## Checks

Run from `ui/`:

```sh
npm ci
npm run check
npm test
npm run build
npx playwright install chromium
npm run test:e2e
```

Use Playwright fixtures for signed-in and signed-out behavior. Verify the fixed panel layout, filter and map agreement, keyboard selection, reduced-motion behavior, authentication failure states, and logout. Do not bypass authentication in the application build.

When the change affects API calls or browser proxy behavior, also run `verify-landman-api`. Start the API only with disposable settings. Use the UI origin, not port 8080, for browser checks.

## Boundaries

- Do not treat fictional browser data as proof of production parcel, tax, soil, or qualification evidence.
- Do not make county, FEMA, or DevNet requests from UI tests.
- Do not run a live Google sign-in without operator approval.
- Keep screenshots in ignored `ui/test-results/`. Do not commit them.

## Completion

Record the Node version, package-lock state, each npm command result, browser test result, and any API dependency. State that county-scale performance, private lead storage, tax requests, and reminders remain out of scope until implemented.
