import { test, expect } from '@playwright/test';

test('layer selections persist while zoom controls which data loads', async ({ page }) => {
  const requested: string[] = [];
  const generation = { fetched_at: '2026-09-15T00:00:00Z', refresh_failed_at: null };
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await page.route('**/api/me', (route) => route.fulfill({ json: { id: 'buyer', email: 'buyer@example.test', name: 'Buyer' } }));
  await page.route('**/api/v1/parcels/map?*', (route) => route.fulfill({ json: { features: [] } }));
  await page.route('**/api/v1/qualification/map?*', (route) => {
    const layer = new URL(route.request().url()).searchParams.get('layer')!;
    requested.push(layer);
    return route.fulfill({ json: { type: 'FeatureCollection', features: [], generation } });
  });
  await page.goto('/');
  const open = page.getByText('Map layers', { exact: true });
  await expect(open).toHaveCount(0);
  await page.getByRole('button', { name: 'Inspect Union, NC', exact: true }).click();
  await open.click();
  const zoning = page.getByRole('checkbox', { name: 'Zoning and jurisdiction' });
  const roads = page.getByRole('checkbox', { name: 'Roads', exact: true });
  const flood = page.getByRole('checkbox', { name: 'FEMA flood zones' });
  await zoning.check();
  await roads.check();
  await expect(page.getByText('Zoom in to see roads.')).toBeVisible();
  await expect.poll(() => requested.includes('zoning')).toBe(true);
  expect(requested).not.toContain('roads');
  await expect(flood).not.toBeChecked();
  for (let i = 0; i < 6 && !requested.includes('roads'); i++) {
    await page.getByRole('button', { name: 'Zoom in', exact: true }).click();
  }
  await expect.poll(() => requested.includes('roads')).toBe(true);
  await zoning.uncheck();
  await expect(roads).toBeChecked();
  await page.getByRole('button', { name: 'Show all counties', exact: true }).click();
  await expect(open).toHaveCount(0);
  await page.getByRole('button', { name: 'Locate selected county', exact: true }).click();
  await open.click();
  await expect(roads).toBeChecked();
  await expect(zoning).not.toBeChecked();
  await expect(page.getByText('Zoom in to see roads.')).toBeVisible();
});
