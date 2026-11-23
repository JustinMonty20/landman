import { expect, test } from '@playwright/test';

const origin = `http://127.0.0.1:${process.env.PLAYWRIGHT_PORT ?? '5174'}`;

test('superseded lookup cannot show results from the old query', async ({ page }) => {
  await page.route('**/*', route => {
    const url = new URL(route.request().url());
    if (url.origin !== origin || /^\/(api|auth)(\/|$)/.test(url.pathname)) return route.abort();
    return route.continue();
  });
  await page.route('**/api/me', route => route.fulfill({ json: { id: 'fixture', email: 'fixture@example.test', name: 'Fixture' } }));
  await page.route('**/api/me/parcel-detail-fields', route => route.fulfill({ json: { fields: [] } }));
  let releaseOld!: () => void;
  const oldPending = new Promise<void>(resolve => { releaseOld = resolve; });
  let oldStarted!: () => void;
  const oldRequested = new Promise<void>(resolve => { oldStarted = resolve; });
  await page.route('**/api/v1/**', async route => {
    const url = new URL(route.request().url());
    if (url.pathname === '/api/v1/parcels/lookup') {
      if (url.searchParams.get('q') === 'OLD') {
        oldStarted();
        await oldPending;
      }
      return route.fulfill({ json: { items: [{ id: url.searchParams.get('q'), parcel_number: url.searchParams.get('q'), address: 'Fixture Road' }] } }).catch(() => {});
    }
    if (url.pathname === '/api/v1/parcels/map') return route.fulfill({ json: { features: [], match_count: 0, source_stamp: 'fixture' } });
    if (url.pathname === '/api/v1/parcels') return route.fulfill({ json: { items: [], count: 0, source_stamp: 'fixture' } });
    return route.abort();
  });
  await page.goto('/');
  await page.getByRole('button', { name: 'Inspect Union, NC', exact: true }).click();
  const finder = page.getByRole('combobox', { name: 'Find parcel ID or address' });
  await finder.fill('OLD');
  await oldRequested;
  await finder.fill('NEW');
  await expect(page.getByRole('option', { name: 'Fixture Road · NEW' })).toBeVisible();
  releaseOld();
  await expect(page.getByRole('option', { name: 'Fixture Road · OLD' })).toHaveCount(0);
  await expect(page.getByRole('option', { name: 'Fixture Road · NEW' })).toBeVisible();
});
