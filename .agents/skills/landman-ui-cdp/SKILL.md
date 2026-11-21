---
name: landman-ui-cdp
description: Debug Landman's Svelte UI with the Chrome DevTools Protocol (CDP). Use for browser errors, stale Svelte state, or slow map and panel actions that need a browser profile.
---

# Debug and profile the Landman UI

CDP is the Chrome DevTools Protocol. Use Playwright to open a CDP session in a fresh local Chromium browser. Use the `ui/` dependencies and the fixtures in `ui/tests/`. Do not connect to a personal browser or open a remote debugging port. Use fixture data unless the operator approves a different target.

## A. Reproduce the symptom

1. Select the closest test in `ui/tests/`. Record the route, viewport, initial state, and exact action. Use a fixture for `/api/me` and for any other API requests the action makes. Keep these fixtures in Playwright, not in the application.
2. For source debugging, use `npm run dev` from `ui/`. For performance comparisons, use the production build served by `npm run preview`. The Playwright configuration already builds and serves preview. Keep browser version, viewport, fixtures, graphics backend, and warm-up actions the same between runs.
3. Wait for a visible result or a specific response that marks completion. Do not use a fixed delay or general network idle in place of this check.

## B. Debug state and rendering

Use `await page.context().newCDPSession(page)` with the test's `page`. Subscribe before navigation or the action. Then enable the required domains:

- **B1. Errors:** Use `Runtime.enable` with `Runtime.exceptionThrown` and `Log.enable` with `Log.entryAdded`. Record the first relevant error and its source location. Remove sensitive values before reporting.
- **B2. Data:** Use `Network.enable`. Match `Network.responseReceived` and `Network.loadingFailed` to `Network.requestWillBeSent` by request ID. Record method, path without query values, status, and timing. Exclude headers, cookies, and bodies from reports.
- **B3. Svelte:** Reproduce in the development build. Use the first application stack frame to find the route or component in `ui/src/`. Check the input, the event handler, the `$state` update, and any `$derived` or `$effect` that uses that state. Confirm the resulting DOM with a Playwright assertion. CDP does not expose Svelte state as a separate protocol domain.

Detach the session after inspection. Report which observable result was wrong and the first layer where it became wrong. If the evidence does not establish that layer, state the next check.

## C. Profile one action

Create a temporary `ui/tests/profile-action.spec.ts` based on the closest existing test. Put the suspect action and its completion assertion inside `profileAction`. This example measures the transition to county coverage. Replace the action and assertion for another symptom.

```ts
import { expect, test } from '@playwright/test';
import { profileAction } from '../../.agents/skills/landman-ui-cdp/scripts/profile.mjs';

test('profile county coverage', async ({ page }) => {
  const origin = 'http://127.0.0.1:5174'; // The preview origin in the Playwright configuration.
  await page.route('**/*', (route) => {
    const url = new URL(route.request().url());
    if (url.origin !== origin || /^\/(api|auth)(\/|$)/.test(url.pathname)) return route.abort();
    return route.continue();
  });
  await page.route('**/api/me', (route) => route.fulfill({
    json: { id: 'profile-fixture', email: 'buyer@example.test', name: 'Buyer' },
  }));
  await page.goto('/');
  const link = page.getByRole('link', { name: 'County coverage', exact: true });
  await expect(link).toBeVisible();
  const dir = await profileAction(page, async () => {
    await link.click();
    await expect(page.getByRole('heading', { name: 'County coverage', exact: true })).toBeVisible();
  });
  console.log(dir);
});
```

Run from `ui/`:

```sh
npm run test:e2e -- profile-action.spec.ts --project=desktop --workers=1 --repeat-each=3
```

- **C1. JavaScript:** Open `action.cpuprofile` in the Chrome DevTools Performance panel. Check heavy call stacks. `metrics.json` contains before and after values. Compare changes in `ScriptDuration`, `LayoutDuration`, and `TaskDuration` within each run. Some durations overlap. Do not add them to calculate total time.
- **C2. Rendering:** If map movement is slow but the JavaScript profile does not explain it, record a Performance trace for the same action. Check frame, paint, and GPU tracks. A page CPU profile does not measure GPU work or MapLibre worker CPU. Mark those stages unmeasured unless the trace measures them.
- **C3. Report:** Give the median action duration, browser settings, and the largest measured costs. Separate browser work from request waits. Fixture results do not measure the live Go API, and a software-rendered map does not measure hardware GPU performance. If an action finishes too quickly to give useful samples, use the same stated number of repetitions in each run.

Keep capture files private. CPU profiles can contain source URLs and function names. Remove the temporary test and the generated profile directories after use. Keep normal browser verification separate from profiling.
