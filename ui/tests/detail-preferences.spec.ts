import { test, expect } from '@playwright/test';
const first = 'a0000000-0000-4000-8000-000000000001';
const second = 'a0000000-0000-4000-8000-000000000002';
const origin = `http://127.0.0.1:${process.env.PLAYWRIGHT_PORT ?? '5174'}`;

test('saved compact groups persist across parcels and reload, including empty selection', async ({ page }, testInfo) => {
  await page.emulateMedia({ reducedMotion: 'reduce' });
  let stored = ['mapped_acres', 'soil_septic'];
  let mapRelease = 'current-release';
  const requests: string[] = [];
  await page.route('**/*', route => {
    const url = new URL(route.request().url());
    if (url.origin !== origin || /^\/(api|auth)(\/|$)/.test(url.pathname)) return route.abort();
    return route.continue();
  });
  await page.route('**/api/me', route => route.fulfill({ json: { id: first, email: 'fixture@example.test', name: 'Fixture' } }));
  await page.route('**/api/me/parcel-detail-fields', route => {
    if (route.request().method() === 'PUT') {
      stored = (route.request().postDataJSON() as { fields: string[] }).fields;
    }
    return route.fulfill({ json: { fields: stored } });
  });
  await page.route('**/api/me/parcel-owner/*', route => { requests.push(new URL(route.request().url()).pathname); return route.fulfill({ json: { current_name_1: 'County Owner', current_name_2: null, mailing_line_1: 'PO Box 9', mailing_city: 'Monroe', mailing_state: 'NC', mailing_postal_code: '28110' } }); });
  await page.route('**/api/v1/**', route => {
    const path = new URL(route.request().url()).pathname;
    requests.push(path);
    if (path === '/api/v1/parcels') return route.fulfill({ json: { items: [{ id: first, parcel_number: 'FIRST', address: 'Fixture Way' }, { id: second, parcel_number: 'SECOND', address: 'Second Way' }], count: 2, source_stamp: 'fixture' } });
    if (path === '/api/v1/parcels/map') return route.fulfill({ json: { features: [], match_count: 2, source_stamp: 'fixture' } });
    if (path.endsWith('/soil')) return route.fulfill({ json: { available: true, total_acres: 2.5, calculated_at: '2026-01-31', uses: [{ use: 'conventional-septic', rated: [{ rating: 'Very limited', area: { acres: 1.5 } }, { rating: 'Somewhat limited', area: { acres: 0.004 } }], unrated: { acres: 0 }, uncovered: { acres: 1 }, ambiguous: { acres: 0 }, next_step: 'Confirm with county health department', map_units: [{ map_unit_key: '1001', interpretation: { fetched_at: '2026-01-30', revision: { survey_saved_on: '2024-05-02' }, components: [{ name: 'Cecil', reason_name: 'Depth to rock', reason: 'Shallow bedrock' }] } }] }, { use: 'dwelling-with-basement', rated: [{ rating: 'Not limited', area: { acres: 2 } }], unrated: { acres: 0 }, uncovered: { acres: 0.5 }, ambiguous: { acres: 0 }, next_step: 'Confirm site' }], freshness: { stale: true, reasons: ['source_age'], refresh_failed_at: '2026-02-01' } } });
    if (path.endsWith('/zoning')) return route.fulfill({ json: { available: true, unmapped_acres: 0.004, zones: [{ jurisdiction: 'Weddington', district_code: 'R-40', description: 'Residential', case_reference: 'CZ-12', area: { acres: 0.004 } }], jurisdictions: [{ name: 'Weddington' }], freshness: { stale: false, reasons: [], source_fetched_at: '2026-02-01', refresh_failed_at: null } } });
if (path.endsWith('/roads')) return route.fulfill({ json: { available: true, state: 'nearby', distance_feet: 12, ties: [{ name: 'Oak Lane', status: 'private' }, { name: 'Pine Lane', status: 'public' }], freshness: { stale: false, reasons: [], source_fetched_at: '2026-02-01', refresh_failed_at: null } } });
if (path.endsWith('/flood')) return route.fulfill({ json: { status: 'available', release: { id: 'old-release', issued_at: '2024-01-01', observed_at: '2024-01-01' }, acres: { parcel: 2.5, unknown: 0.004, floodway: 0 }, zones: [{ code: 'AE', subtype: 'sfha', sfha: 'T', acres: 0.004 }], panels: [{ number: '37179C', effective_date: '2024-04-01' }] } });
if (path.endsWith('/tax')) return route.fulfill({ json: { bills: [{ year: 2025, total_taxes: '0.00', fetched_at: '2026-02-03' }], delinquent_balances: [{ year: 2024, balance_due: '69.12', fetched_at: '2026-02-03' }], reported_delinquent_total: '69.12', pipeline: [], foreclosure_cases: [{ case_number: '25-CV-4', status: 'open', sale_on: '2026-03-01', fetched_at: '2026-02-03' }], note: 'Not a payoff quote.' } });
    if (/\/parcels\/[a-f0-9-]+$/.test(path)) return route.fulfill({ json: { location: { type: 'Point', coordinates: [-80.55, 34.99] }, parcel: { parcel_number: path.endsWith(first) ? 'FIRST' : 'SECOND', address: 'Fixture Way', mapped_acres: '0.004', updated_at: '2026-01-01' }, source: 'county', valuation: { land: '123000.00', improvement: '0.00', total: '123000.00', assessed: null }, source_parcel_id: 'fixture', owner_labels: null, sales: [{ date: '2025-06-12', amount: '0.00', deed_book: '42', deed_page: '19' }] } });
    if (path === '/api/v1/flood/map') return route.fulfill({ json: { type: 'FeatureCollection', features: [], release: { id: mapRelease, issued_at: '2026-01-01', observed_at: '2026-01-01' } } });
    return route.abort();
  });
  await page.goto('/?county=37179');
  await page.getByRole('combobox', { name: 'Flood category' }).selectOption('sfha');
  await page.getByRole('button', { name: /FIRST · Fixture Way/ }).click();
  const panel = page.getByRole('region', { name: 'Parcel evidence' });
  await expect(panel).toContainText('Parcel');
  await expect(panel).toContainText('FIRST');
await panel.getByRole('tab', { name: 'Research links' }).click();
await expect(panel.getByRole('link', { name: 'Open satellite view' })).toHaveAttribute('href', 'https://earth.google.com/web/search/34.99,-80.55');
await panel.getByRole('tab', { name: 'Evidence', exact: true }).click();
  await expect(panel.getByRole('region', { name: 'Soil screening' })).toContainText('Very limited: 1.50 acres');
await expect(panel.getByRole('region', { name: 'Soil screening' })).toContainText('Somewhat limited: <0.01 acres');
await expect(panel.getByText('Stale soil evidence', { exact: false })).toHaveCount(1);
  await expect(panel.getByRole('region', { name: 'Soil screening' })).not.toContainText('Dwelling with basement rating');
  expect(requests.filter(path => path.endsWith('/soil'))).toHaveLength(1);
  expect(requests.filter(path => path.endsWith('/tax'))).toHaveLength(0);
  expect(requests.filter(path => path.includes('/parcel-owner/'))).toHaveLength(0);
  await panel.getByRole('button', { name: 'Choose detail fields' }).click();
  await panel.getByRole('checkbox', { name: 'Owner name' }).check();
await panel.getByRole('checkbox', { name: 'Owner mailing address' }).check();
await panel.getByRole('checkbox', { name: 'Parcel land value' }).check();
  await panel.getByRole('button', { name: 'Save details' }).click();
  await expect(panel.getByRole('region', { name: 'Soil screening' })).toContainText('Very limited: 1.50 acres');
  await expect(panel.getByRole('region', { name: 'Soil screening' })).not.toContainText('Dwelling with basement rating');
  await expect(panel.getByRole('region', { name: 'Ownership and sales' })).toContainText('County Owner');
await expect(panel.getByRole('region', { name: 'Ownership and sales' })).toContainText('PO Box 9');
await expect(panel.getByRole('region', { name: 'Land and value' })).toContainText('$123,000.00');
  await panel.getByRole('button', { name: 'Choose detail fields' }).click();
  await panel.getByRole('checkbox', { name: 'Septic field rating' }).uncheck();
  await panel.getByRole('checkbox', { name: 'Reported delinquent total' }).check();
  await panel.getByRole('button', { name: 'Save details' }).click();
  await expect(panel.getByText('Detail fields saved.')).toBeVisible();
  await expect(panel.getByRole('region', { name: 'Tax and foreclosure' })).toBeVisible();
  await expect(panel.getByRole('region', { name: 'Ownership and sales' })).toContainText('County Owner');
  await expect(panel.getByRole('region', { name: 'Soil screening' })).toHaveCount(0);
  await expect(panel.getByRole('region', { name: 'Ownership and sales' })).not.toContainText('Sales history');
  expect(requests.filter(path => path.endsWith('/tax'))).toHaveLength(1);
  await page.screenshot({ path: testInfo.outputPath('compact-detail.png') });
  await panel.getByRole('button', { name: 'Close parcel details' }).click();
  await expect(page.getByRole('region', { name: 'Matching parcels' })).toContainText('SECOND');
  await page.getByRole('button', { name: /SECOND · Second Way/ }).click();
  await expect(panel).toContainText('SECOND');
  await expect(panel.getByRole('region', { name: 'Tax and foreclosure' })).toBeVisible();
  await page.reload();
  await page.getByRole('combobox', { name: 'Flood category' }).selectOption('sfha');
  await page.getByRole('button', { name: /FIRST · Fixture Way/ }).click();
  await expect(panel.getByRole('region', { name: 'Tax and foreclosure' })).toBeVisible();
  await panel.getByRole('button', { name: 'Choose detail fields' }).click();
  await expect(panel.getByRole('checkbox', { name: 'Owner name' })).toBeChecked();
  await page.screenshot({ path: testInfo.outputPath('individual-field-picker.png') });
  await panel.getByRole('checkbox', { name: 'Mapped acreage', exact: true }).uncheck();
  await panel.getByRole('checkbox', { name: 'Reported delinquent total' }).uncheck();
  await panel.getByRole('checkbox', { name: 'Owner name' }).uncheck();
await panel.getByRole('checkbox', { name: 'Owner mailing address' }).uncheck();
await panel.getByRole('checkbox', { name: 'Parcel land value' }).uncheck();
  await panel.getByRole('button', { name: 'Save details' }).click();
  await expect(panel).toContainText('FIRST');
  await expect(panel.getByRole('region', { name: 'Tax and foreclosure' })).toHaveCount(0);
  await expect(panel.getByRole('region', { name: 'Land and value' })).toHaveCount(0);
  await page.screenshot({ path: testInfo.outputPath('empty-detail.png') });
  expect(stored).toEqual([]);
  await panel.getByRole('button', { name: 'Choose detail fields' }).click();
  await panel.getByRole('checkbox', { name: 'Mapped acreage', exact: true }).check();
  await panel.getByRole('button', { name: 'Save details' }).click();
  await expect(panel.getByRole('region', { name: 'Land and value' })).toContainText('<0.01 acres');
  expect(stored).toEqual(['mapped_acres']);
  await page.screenshot({ path: testInfo.outputPath('added-field.png') });
  await panel.getByRole('button', { name: 'Choose detail fields' }).click();
  for (const field of ['Parcel improvement value', 'Parcel assessed value', 'Sales history', 'Reported delinquent total', 'Tax bills', 'Foreclosure records', 'Septic field rating', 'Zoning district', 'Jurisdiction', 'Unmapped acreage', 'Nearest road name', 'Road status', 'Unknown coverage acreage', 'Mapped flood zones']) {
    await panel.getByRole('checkbox', { name: field, exact: true }).check();
  }
  await panel.getByRole('button', { name: 'Save details' }).click();
  await expect(panel.getByRole('region', { name: 'Land and value' })).toContainText('$0.00');
  await expect(panel.getByRole('region', { name: 'Land and value' })).toContainText('Not recorded');
  await expect(panel.getByRole('region', { name: 'Ownership and sales' })).toContainText('2025-06-12');
  await expect(panel.getByRole('region', { name: 'Tax and foreclosure' })).toContainText('absent years do not prove payment');
  await expect(panel.getByRole('region', { name: 'Tax and foreclosure' })).toContainText('1 stored feed records');
  await expect(panel.getByRole('region', { name: 'Zoning' })).toContainText('Residential');
  await expect(panel.getByRole('region', { name: 'Zoning' })).toContainText('<0.01');
  await expect(panel.getByRole('region', { name: 'FEMA flood mapping' })).toContainText('Refresh map to compare releases');
  await expect(panel).not.toContainText('AE sfha');
  await expect(panel).not.toContainText('FEMA archive 2024');
  await panel.getByText('Measured geometry, limits and next site check').click();
  await expect(panel).toContainText('Shallow bedrock');
  await panel.getByText('Stored sale records (1)').click();
  await panel.getByText('Case and pipeline details').click();
  await page.screenshot({ path: testInfo.outputPath('populated-evidence.png'), fullPage: true });
  mapRelease = 'old-release';
  await page.reload();
  await page.getByRole('combobox', { name: 'Flood category' }).selectOption('sfha');
  await page.getByRole('button', { name: /FIRST · Fixture Way/ }).click();
  await expect(panel.getByRole('region', { name: 'FEMA flood mapping' })).toContainText('AE sfha');
  await expect(panel).toContainText('FEMA archive 2024-01-01');
  await expect(panel.getByRole('region', { name: 'FEMA flood mapping' })).toContainText('<0.01 acres');
  await page.screenshot({ path: testInfo.outputPath('matching-flood-release.png'), fullPage: true });

async function only(label: string) {
  await panel.getByRole('button', { name: 'Choose detail fields' }).click();
  const checks = panel.getByRole('checkbox');
  for (let i = 0; i < await checks.count(); i++) {
    const check = checks.nth(i);
    if (await check.isChecked()) await check.uncheck();
  }
  if (label) await panel.getByRole('checkbox', { name: label, exact: true }).check();
  await panel.getByRole('button', { name: 'Save details' }).click();
}
await only('Zoning district');
await expect(panel.getByRole('region', { name: 'Zoning' })).toContainText('R-40');
await expect(panel.getByRole('region', { name: 'Zoning' })).not.toContainText('Jurisdiction');
await only('Nearest road name');
await panel.getByText('Road status, ties and source').click();
await expect(panel.getByRole('region', { name: 'Mapped roads' })).toContainText('Oak Lane');
await expect(panel.getByRole('region', { name: 'Mapped roads' })).not.toContainText('private');
await only('Road status');
await panel.getByText('Road status, ties and source').click();
await expect(panel.getByRole('region', { name: 'Mapped roads' })).toContainText('private');
await expect(panel.getByRole('region', { name: 'Mapped roads' })).not.toContainText('Oak Lane');
await only('Mapped flood zones');
await panel.getByText('FEMA source and zero-area checks').click();
await expect(panel.getByRole('region', { name: 'FEMA flood mapping' })).toContainText('AE sfha');
await expect(panel.getByRole('region', { name: 'FEMA flood mapping' })).not.toContainText('Coverage gap');
await expect(panel.getByRole('region', { name: 'FEMA flood mapping' })).not.toContainText('Floodway');
await only('');
for (const name of ['Land and value', 'Ownership and sales', 'Tax and foreclosure', 'Vacancy screening', 'Soil screening', 'Zoning', 'Mapped roads', 'FEMA flood mapping']) {
  await expect(panel.getByRole('region', { name })).toHaveCount(0);
}
await only('Parcel total value');
await expect(panel.getByRole('region', { name: 'Land and value' })).toContainText('$123,000.00');
await panel.getByRole('region', { name: 'Land and value' }).scrollIntoViewIfNeeded();
await page.screenshot({ path: testInfo.outputPath('valuation-visible.png') });
await panel.getByRole('button', { name: 'Choose detail fields' }).click();
for (const label of ['Mapped acreage', 'Parcel land value', 'Parcel improvement value', 'Parcel assessed value']) await panel.getByRole('checkbox', { name: label, exact: true }).check();
await panel.getByRole('button', { name: 'Save details' }).click();
const valueGroup = panel.getByRole('region', { name: 'Land and value' });
for (const label of ['Mapped acreage', 'Parcel land value', 'Parcel improvement value', 'Parcel total value', 'Parcel assessed value']) await expect(valueGroup).toContainText(label);
await valueGroup.scrollIntoViewIfNeeded();
await page.screenshot({ path: testInfo.outputPath('grouped-valuation-visible.png') });

});
