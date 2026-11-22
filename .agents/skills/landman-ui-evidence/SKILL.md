---
name: landman-ui-evidence
description: Gather QA evidence for Landman's Svelte UI with Playwright. Use for asserted UI screenshots, recorded user actions, or fixture-based browser verification.
---
# Gather Landman UI evidence

Use Playwright in a fresh local Chromium browser. Use `ui/` dependencies and fixtures in `ui/tests/`. Capture only fixture data unless the operator approves another target.

Evidence proves the stated browser result. It does not prove a live API result, a hardware GPU result, or an unasserted visual property.

## A. Define the check

1. Select the closest test in `ui/tests/`. Record the route, viewport, initial state, action, expected visible result, and required API fixtures.
2. Use an existing test when it already asserts the result. Add a temporary `ui/tests/evidence-action.spec.ts` only when no existing test has the required action.
3. Route `/api/me` and every request the action makes. Abort other API and auth requests. Keep fixtures in Playwright. Do not change application data for evidence capture.
4. Use a Playwright assertion for each captured state. Wait for that assertion. Do not use a fixed delay or `networkidle`.

## B. Capture screenshots and video

Capture a screenshot after each asserted state. Record video only when the action has important motion or more than one state. Use the configured desktop or tablet project. Keep the viewport, reduced-motion setting, fixtures, and graphics backend fixed for repeated evidence.

Create a temporary test like this. Replace the route, fixtures, action, and assertions with the selected check.

```ts
import { expect, test } from '@playwright/test';

test.use({ video: 'on' });

test('evidence: county coverage', async ({ page }, testInfo) => {
  const origin = 'http://127.0.0.1:5174';
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await page.route('**/*', (route) => {
    const url = new URL(route.request().url());
    if (url.origin !== origin || /^\/(api|auth)(\/|$)/.test(url.pathname)) return route.abort();
    return route.continue();
  });
  await page.route('**/api/me', (route) => route.fulfill({
    json: { id: 'evidence-fixture', email: 'buyer@example.test', name: 'Buyer' },
  }));

  await page.goto('/');
  const link = page.getByRole('link', { name: 'County coverage', exact: true });
  await expect(link).toBeVisible();
  await page.screenshot({ path: testInfo.outputPath('01-map.png'), fullPage: true });

  await link.click();
  await expect(page.getByRole('heading', { name: 'County coverage', exact: true })).toBeVisible();
  await page.screenshot({ path: testInfo.outputPath('02-coverage.png'), fullPage: true });
});
```

Run from `ui/`. This command stores screenshots, video, and test artifacts outside Git.

```sh
npm run test:e2e -- evidence-action.spec.ts --project=desktop --workers=1 --output=../artifacts/verify/ui-evidence/county-coverage
```

Use `--project=tablet` for a tablet check. Add `--trace=on` only when a failed check needs action detail. Delete the temporary test after the capture unless it becomes a maintained browser test.

## C. Verify and report

1. Confirm the test passes. Confirm each screenshot shows the asserted state. Confirm the video covers the stated action when video was requested.
2. Report the route, project and viewport, fixtures, action, expected result, test result, and relative evidence directory. Name each file that supports the result.
3. State the limit of the evidence. Fixture captures do not verify live Go API data. Software-rendered maps do not verify hardware GPU behavior. A screenshot does not verify hidden state or accessibility beyond its assertions.
4. Keep capture files private. Do not put source records, cookies, headers, request bodies, or personal browser data in screenshots, video, traces, or reports. Delete evidence after review unless the operator requests retention.

Use `landman-ui-cdp` for browser errors, stale Svelte state, and action profiling. Use this skill for normal UI QA evidence.
