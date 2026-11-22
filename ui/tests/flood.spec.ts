import { test, expect } from '@playwright/test';

test('shows stored flood zones and evidence for a visible parcel', async ({ page }) => {
  const id = 'a0000000-0000-4000-8000-000000000001';
  const release = { id, issued_at: '2026-09-14T00:00:00Z', observed_at: '2026-09-15T00:00:00Z' };
  const geometry = { type: 'Polygon', coordinates: [[[-80.6, 35], [-80.59, 35], [-80.59, 35.01], [-80.6, 35]]] };
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await page.route('**/api/me', (route) => route.fulfill({ json: { id, email: 'buyer@example.test', name: 'Buyer' } }));
  await page.route('**/api/v1/flood/map?*', (route) => route.fulfill({ json: {
    type: 'FeatureCollection', release, features: [{ type: 'Feature', geometry, properties: { code: 'AE', subtype: '', sfha: 'T' } }]
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
  const details = page.getByRole('region', { name: 'Parcel flood evidence' });
  await expect(details).toContainText('Zone AE: 7.00 acres');
  await expect(details).toContainText('Unknown coverage: 3.00 acres');
  await expect(details).toContainText('3710542500J · 2008-10-16');
  await page.getByRole('button', { name: 'Close parcel details' }).click();
  await expect(page.getByRole('button', { name: 'FLOOD-TEST' })).toBeVisible();
});
