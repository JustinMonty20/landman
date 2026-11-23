import { expect, test } from '@playwright/test';

const origin = `http://127.0.0.1:${process.env.PLAYWRIGHT_PORT ?? '5174'}`;
const id = 'a0000000-0000-4000-8000-000000000001';
const parcels = Array.from({ length: 9 }, (_, index) => ({
  id: `a0000000-0000-4000-8000-${String(index + 1).padStart(12, '0')}`,
  parcel_number: `PIN-${String(index + 1).padStart(3, '0')}`,
  address: index === 0 ? 'Oak Road' : `Oak Road ${index + 1}`,
}));

test('finder narrows immediately and only selection locates the parcel', async ({ page }, testInfo) => {
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await page.route('**/*', route => {
    const url = new URL(route.request().url());
    if (url.origin !== origin || /^\/(api|auth)(\/|$)/.test(url.pathname)) return route.abort();
    return route.continue();
  });
  await page.route('**/api/me', route => route.fulfill({ json: { id, email: 'fixture@example.test', name: 'Fixture' } }));
  await page.route('**/api/me/parcel-detail-fields', route => route.fulfill({ json: { fields: ['mapped_acres'] } }));
  const requests: string[] = [];
  await page.route('**/api/v1/**', route => {
    const url = new URL(route.request().url());
    requests.push(url.pathname + url.search);
    if (url.pathname === '/api/v1/parcels/lookup') return url.searchParams.get('q') === 'FAIL' ? route.fulfill({ status: 503, body: 'Unavailable' }) : route.fulfill({ json: { items: url.searchParams.get('q') === 'OAK' ? parcels : [] } });
    if (url.pathname === '/api/v1/parcels/map') return route.fulfill({ json: { features: [], match_count: 1, source_stamp: 'fixture' } });
    if (url.pathname === '/api/v1/parcels') return route.fulfill({ json: { items: [], count: 1, source_stamp: 'fixture' } });
    const parcel = parcels.find(item => url.pathname === `/api/v1/parcels/${item.id}`);
    if (parcel) return route.fulfill({ json: { location: { type: 'Point', coordinates: [-80.7, 34.89] }, parcel: { parcel_number: parcel.parcel_number, address: parcel.address, mapped_acres: '2.50', likely_vacant: true, updated_at: '2026-01-01' }, source: 'union_county_gis', source_parcel_id: 'fixture', owner_labels: null, sales: [] } });
    if (url.pathname.endsWith('/soil')) return route.fulfill({ json: { available: false, reason: 'no published assessment' } });
    if (url.pathname.endsWith('/tax')) return route.fulfill({ json: { bills: [], delinquent_balances: [], reported_delinquent_total: null, pipeline: [], foreclosure_cases: [] } });
    if (url.pathname.endsWith('/vacancy')) return route.fulfill({ json: { available: false, tier: 'unknown', reason: 'validated_evidence_not_published' } });
    if (url.pathname.endsWith('/zoning') || url.pathname.endsWith('/roads')) return route.fulfill({ json: { available: false } });
    if (url.pathname.endsWith('/flood')) return route.fulfill({ json: { status: 'no_release', release: null, parcel_number: 'PIN-001' } });
    return route.abort();
  });
  await page.goto('/');
  await page.getByRole('button', { name: 'Inspect Union, NC', exact: true }).click();
  const finder = page.getByRole('combobox', { name: 'Find parcel ID or address' });
  await finder.fill('OA');
  await expect(page.getByRole('status', { name: '' }).filter({ hasText: 'No parcels found.' })).toBeVisible();
  await page.screenshot({ path: testInfo.outputPath('01-no-matches.png'), fullPage: true });
  await finder.fill('FAIL');
  await expect(page.getByRole('status').filter({ hasText: 'Parcel search is unavailable. Try again.' })).toBeVisible();
  await finder.fill('OAK');
  const option = page.getByRole('option', { name: 'Oak Road · PIN-001' });
  await expect(option).toBeVisible();
  await expect(page.getByRole('region', { name: 'Matching parcels' })).toBeVisible();
  expect(requests.some(path => path.includes('/parcels/lookup?q=OAK'))).toBe(true);
  expect(requests.some(path => path.includes(`/parcels/${id}`))).toBe(false);
  await page.screenshot({ path: testInfo.outputPath('02-suggestions.png'), fullPage: true });
  await finder.press('Escape');
  await expect(option).toHaveCount(0);
  await finder.fill('OAK');
  await expect(option).toBeVisible();
  for (let index = 0; index < 8; index++) await finder.press('ArrowDown');
  const active = page.getByRole('option', { name: 'Oak Road 9 · PIN-009' });
  await expect(active).toHaveAttribute('aria-selected', 'true');
  await expect(active).toHaveCSS('background-color', 'rgb(41, 53, 70)');
  await expect(finder).toBeFocused();
  expect(await active.evaluate(element => {
    const item = element.getBoundingClientRect();
    const list = element.parentElement!.getBoundingClientRect();
    return item.top >= list.top && item.bottom <= list.bottom;
  })).toBe(true);
  await page.screenshot({ path: testInfo.outputPath('02-keyboard-active.png'), fullPage: true });
  expect(requests.some(path => path.includes(`/parcels/${parcels[8].id}`))).toBe(false);
  await finder.press('Enter');
  await expect(page.getByRole('region', { name: 'Parcel evidence' })).toContainText('PIN-009 · Site address: Oak Road 9');
  expect(requests.some(path => path.includes(`/parcels/${parcels[8].id}`))).toBe(true);
  await expect.poll(() => requests.some(path => path.startsWith('/api/v1/parcels/map?') && Number(new URLSearchParams(path.split('?')[1]).get('zoom')) >= 16)).toBe(true);
  await page.screenshot({ path: testInfo.outputPath('03-selected.png'), fullPage: true });
});
