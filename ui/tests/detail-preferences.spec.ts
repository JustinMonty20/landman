import { test, expect } from '@playwright/test';
const first = 'a0000000-0000-4000-8000-000000000001';
const second = 'a0000000-0000-4000-8000-000000000002';
const origin = `http://127.0.0.1:${process.env.PLAYWRIGHT_PORT ?? '5174'}`;

test('saved compact groups persist across parcels and reload, including empty selection', async ({ page }, testInfo) => {
  await page.emulateMedia({ reducedMotion: 'reduce' });
  let stored = ['acreage', 'soil'];
  const requests: string[] = [];
  await page.route('**/*', route => {
    const url = new URL(route.request().url());
    if (url.origin !== origin || /^\/(api|auth)(\/|$)/.test(url.pathname)) return route.abort();
    return route.continue();
  });
  await page.route('**/api/me', route => route.fulfill({ json: { id: first, email: 'fixture@example.test', name: 'Fixture' } }));
  await page.route('**/api/me/parcel-detail-groups', route => {
    if (route.request().method() === 'PUT') {
      stored = (route.request().postDataJSON() as { groups: string[] }).groups;
    }
    return route.fulfill({ json: { groups: stored } });
  });
  await page.route('**/api/v1/**', route => {
    const path = new URL(route.request().url()).pathname;
    requests.push(path);
    if (path === '/api/v1/parcels') return route.fulfill({ json: { items: [{ id: first, parcel_number: 'FIRST', address: 'Fixture Way' }, { id: second, parcel_number: 'SECOND', address: 'Second Way' }], count: 2, source_stamp: 'fixture' } });
    if (path === '/api/v1/parcels/map') return route.fulfill({ json: { features: [], match_count: 2, source_stamp: 'fixture' } });
    if (path.endsWith('/soil')) return route.fulfill({ json: { available: false, reason: 'not mapped' } });
    if (path.endsWith('/tax')) return route.fulfill({ json: { bills: [], delinquent_balances: [], reported_delinquent_total: null, pipeline: [], foreclosure_cases: [], note: 'Not a payoff quote.' } });
    if (/\/parcels\/[a-f0-9-]+$/.test(path)) return route.fulfill({ json: { location: { type: 'Point', coordinates: [-80.55, 34.99] }, parcel: { parcel_number: path.endsWith(first) ? 'FIRST' : 'SECOND', address: 'Fixture Way', mapped_acres: '2.50', updated_at: '2026-01-01' }, source: 'county', source_parcel_id: 'fixture', owner_labels: null, sales: [] } });
    if (path === '/api/v1/flood/map') return route.fulfill({ json: { type: 'FeatureCollection', features: [], release: null } });
    return route.abort();
  });
  await page.goto('/?county=37179');
  await page.getByRole('combobox', { name: 'Flood category' }).selectOption('sfha');
  await page.getByRole('button', { name: /FIRST · Fixture Way/ }).click();
  const panel = page.getByRole('region', { name: 'Parcel evidence' });
  await expect(panel).toContainText('Parcel');
  await expect(panel).toContainText('FIRST');
  await expect(panel.getByRole('region', { name: 'Soil assessment' })).toBeVisible();
  expect(requests.filter(path => path.endsWith('/soil'))).toHaveLength(1);
  expect(requests.filter(path => path.endsWith('/tax'))).toHaveLength(0);
  await panel.getByRole('button', { name: 'Choose detail groups' }).click();
  await panel.getByRole('checkbox', { name: 'Soil' }).uncheck();
  await panel.getByRole('checkbox', { name: 'Tax' }).check();
  await panel.getByRole('button', { name: 'Save details' }).click();
  await expect(panel.getByRole('status')).toContainText('saved');
  await expect(panel.getByRole('region', { name: 'Tax evidence' })).toBeVisible();
  await expect(panel.getByRole('region', { name: 'Soil assessment' })).toHaveCount(0);
  await page.screenshot({ path: testInfo.outputPath('compact-detail.png') });
  await panel.getByRole('button', { name: 'Close parcel details' }).click();
  await expect(page.getByRole('region', { name: 'Matching parcels' })).toContainText('SECOND');
  await page.getByRole('button', { name: /SECOND · Second Way/ }).click();
  await expect(panel).toContainText('SECOND');
  await expect(panel.getByRole('region', { name: 'Tax evidence' })).toBeVisible();
  await page.reload();
  await page.getByRole('combobox', { name: 'Flood category' }).selectOption('sfha');
  await page.getByRole('button', { name: /FIRST · Fixture Way/ }).click();
  await expect(panel.getByRole('region', { name: 'Tax evidence' })).toBeVisible();
  await panel.getByRole('button', { name: 'Choose detail groups' }).click();
  await panel.getByRole('checkbox', { name: 'Acreage' }).uncheck();
  await panel.getByRole('checkbox', { name: 'Tax' }).uncheck();
  await panel.getByRole('button', { name: 'Save details' }).click();
  await expect(panel).toContainText('FIRST');
  await expect(panel.getByRole('region', { name: 'Tax evidence' })).toHaveCount(0);
  await expect(panel.getByRole('region', { name: 'Acreage' })).toHaveCount(0);
  await page.screenshot({ path: testInfo.outputPath('empty-detail.png') });
  expect(stored).toEqual([]);
});
