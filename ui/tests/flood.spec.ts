import { test, expect } from '@playwright/test';

test('shows stored flood zones and evidence for a visible parcel', async ({ page }, testInfo) => {
  const id = 'a0000000-0000-4000-8000-000000000001';
  const release = { id, issued_at: '2026-09-14T00:00:00Z', observed_at: '2026-09-15T00:00:00Z' };
  const geometry = { type: 'Polygon', coordinates: [[[-80.6, 35], [-80.59, 35], [-80.59, 35.01], [-80.6, 35]]] };
  const requests = new Set<string>();
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await page.route('**/*', (route) => {
    const url = new URL(route.request().url());
    if (url.origin !== testInfo.project.use.baseURL || /^\/(api|auth)(\/|$)/.test(url.pathname)) return route.abort();
    return route.continue();
  });
  page.on('request', (request) => {
    const url = new URL(request.url());
    if (url.searchParams.get('flood_category') === 'sfha') requests.add(url.pathname);
  });
  await page.route('**/api/me', (route) => route.fulfill({ json: { id, email: 'buyer@example.test', name: 'Buyer' } }));
  await page.route('**/api/me/parcel-detail-groups', (route) => route.fulfill({ json: { groups: ['flood'] } }));
  await page.route('**/api/v1/parcels?*', (route) => route.fulfill({ json: { items: [{ id, parcel_number: 'FLOOD-TEST', address: null }], count: 1, source_stamp: 'fixture' } }));
  await page.route('**/api/v1/flood/map?*', (route) => route.fulfill({ json: {
    type: 'FeatureCollection', release, features: [{ type: 'Feature', geometry, properties: { category: 'sfha', code: 'AE', subtype: '', sfha: 'T' } }]
  } }));
  await page.route('**/api/v1/parcels/map?*', (route) => route.fulfill({ json: { features: [{ id, parcel_number: 'FLOOD-TEST', address: null, geometry }] } }));
  await page.route(`**/api/v1/parcels/${id}/flood`, (route) => route.fulfill({ json: {
    parcel_number: 'FLOOD-TEST', release, status: 'available', acres: { parcel: 10, unknown: 3, floodway: 1 },
    zones: [{ code: 'AE', subtype: '', sfha: 'T', acres: 7 }], panels: [{ number: '3710542500J', effective_date: '2008-10-16' }]
  } }));
  await page.goto('/');
  await page.getByRole('button', { name: 'Inspect Union, NC', exact: true }).click();
  await page.getByRole('tab', { name: 'Layers', exact: true }).click();
  await page.getByRole('checkbox', { name: 'FEMA flood zones' }).check();
  await expect(page.getByText('Archive issue date: 2026-09-14', { exact: true })).toBeVisible();
  for (let i = 0; i < 6; i++) {
    if (await page.getByRole('button', { name: 'FLOOD-TEST' }).isVisible()) break;
    await page.getByRole('button', { name: 'Zoom in', exact: true }).click();
  }
  await page.getByRole('button', { name: 'FLOOD-TEST' }).click();
  const details = page.getByRole('region', { name: 'Parcel evidence' });
  await expect(details).toContainText('Zone AE: 7.00 acres');
  await expect(details).toContainText('Unknown coverage: 3.00 acres');
  await expect(details).toContainText('3710542500J · 2008-10-16');
  await page.getByRole('button', { name: 'Close parcel details' }).click();
  const legendFilter = page.getByRole('button', { name: 'Special flood hazard area (1% annual chance)', exact: true });
  await legendFilter.click();
  await expect(legendFilter).toHaveAttribute('aria-pressed', 'true');
  await expect(page.getByRole('region', { name: 'Matching parcels' })).toContainText('1 match');
  await expect.poll(() => [...requests].sort()).toEqual(['/api/v1/flood/map', '/api/v1/parcels', '/api/v1/parcels/map']);
  await page.screenshot({ path: testInfo.outputPath('flood-selection.png'), fullPage: true });
  await page.getByRole('tab', { name: 'Filters', exact: true }).click();
  await expect(page.getByRole('combobox', { name: 'Flood category' })).toHaveValue('sfha');
  await page.getByRole('tab', { name: 'Layers', exact: true }).click();
  await legendFilter.press('Space');
  await expect(legendFilter).toHaveAttribute('aria-pressed', 'false');
  await expect(page.getByRole('region', { name: 'Matching parcels' })).toContainText('1 match');
  await expect(page.getByRole('button', { name: 'FLOOD-TEST' })).toBeVisible();
});
