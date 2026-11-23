import { test, expect } from '@playwright/test';

test('default, combined and cleared county discovery', async ({ page }, info) => {
 const origin = `http://127.0.0.1:${process.env.PLAYWRIGHT_PORT ?? '5174'}`;
 await page.emulateMedia({ reducedMotion: 'reduce' });
 await page.route('**/*', async route => {
  const url = new URL(route.request().url());
  if (url.origin !== origin) return route.abort();
  if (!/^\/(api|auth)(\/|$)/.test(url.pathname)) return route.continue();
  if (url.pathname === '/api/me') return route.fulfill({ json: { id: 'fixture', email: 'buyer@example.test', name: 'Buyer' } });
  if (url.pathname === '/api/v1/parcels/soil-types') return route.fulfill({ json: { items: [{ key: 'fixture-unit', name: 'Fixture named map unit' }] } });
  if (url.pathname === '/api/v1/parcels') {
   const combined = url.searchParams.has('foreclosure');
   return route.fulfill({ json: { items: combined ? [{ id: '00000000-0000-0000-0000-000000000001', parcel_number: 'P-1', address: 'Fixture Road' }] : [{ id: '00000000-0000-0000-0000-000000000001', parcel_number: 'P-1', address: 'Fixture Road' }, { id: '00000000-0000-0000-0000-000000000002', parcel_number: 'P-2', address: 'Other Road' }], count: combined ? 1 : 2, source_stamp: 'fixture' } });
  }
  if (url.pathname === '/api/v1/parcels/map') {
   if (Number(url.searchParams.get('zoom')) >= 13) return route.fulfill({ status: 413 });
   return route.fulfill({ json: { mode: 'grid', cells: [], match_count: url.searchParams.has('foreclosure') ? 1 : 2, context_count: 2, source_stamp: 'fixture' } });
  }
  return route.abort();
 });
 await page.goto('/?county=37179');
 const panel = page.getByRole('region', { name: 'Matching parcels' });
 await expect(panel).toContainText('2 matches');
 await page.screenshot({ path: info.outputPath('01-default.png'), fullPage: true });
 const filters = page.getByRole('complementary', { name: 'Parcel filters' });
 await filters.getByRole('checkbox', { name: 'Foreclosure record' }).check();
 await filters.getByRole('button', { name: 'Soil filters' }).click();
 await filters.getByRole('combobox', { name: 'Soil type' }).selectOption('fixture-unit');
 await filters.getByRole('combobox', { name: 'Intended use' }).selectOption('conventional-septic');
 await filters.getByRole('combobox', { name: 'Published rating' }).selectOption('Not limited');
 await expect(panel).toContainText('1 match');
 await expect(panel).not.toContainText('P-2');
 await page.screenshot({ path: info.outputPath('02-combined.png'), fullPage: true });
 await filters.getByRole('button', { name: 'Clear all' }).click();
 await expect(panel).toContainText('2 matches');
 await expect(panel).toContainText('P-2');
 await page.screenshot({ path: info.outputPath('03-cleared.png'), fullPage: true });
 const outlineLimit = panel.getByText('Too many parcels to draw. Zoom in or apply a filter.');
 for (let n = 0; n < 6 && !(await outlineLimit.isVisible()); n++) {
  await page.getByRole('button', { name: 'Zoom in', exact: true }).click();
 }
 await expect(outlineLimit).toBeVisible();
 await expect(panel).toContainText('2 in map view');
 await page.screenshot({ path: info.outputPath('04-outline-limit.png'), fullPage: true });
});
