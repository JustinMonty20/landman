import { test, expect } from '@playwright/test';

test.beforeEach(async ({ page }) => {
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await page.route('**/api/me', (route) => route.fulfill({ json: { id: 'sample-user', email: 'buyer@example.test', name: 'Buyer' } }));
await page.route('**/api/v1/parcels/soil-types', route => route.fulfill({ json: { items: [] } }));
await page.route('**/api/v1/parcels?*', route => route.fulfill({ json: { items: [], count: 0, source_stamp: 'fixture' } }));
await page.route('**/api/v1/parcels/map?*', route => route.fulfill({ json: { mode: 'grid', cells: [], match_count: 0, context_count: 0, source_stamp: 'fixture' } }));
  await page.goto('/?county=37179');
  await expect(page.getByRole('heading', { name: 'Filter Union parcels' })).toBeVisible();
});

test('filter sections expose data-specific controls and bounded soil choices', async ({ page }) => {
  const panel = page.getByRole('complementary', { name: 'Parcel filters' });
  await expect(panel.getByText(/mock|preview/i)).toHaveCount(0);
  await expect(panel.getByText('Choose filters to narrow county parcels.')).toBeVisible();
  for (const name of ['Owner', 'Soil', 'Tax', 'Parcel']) {
    await expect(panel.getByRole('button', { name: `${name} filters`, exact: true })).toBeVisible();
  }
  await expect(panel.getByRole('checkbox', { name: 'Out-of-state mailing' })).toBeEnabled();
  await expect(panel.getByText('Owner mailing address is outside North Carolina.')).toBeVisible();
  await panel.getByRole('button', { name: 'Soil filters', exact: true }).click();
  await expect(panel.getByRole('combobox', { name: 'Soil type', exact: true })).toBeEnabled();
  await expect(panel.getByRole('option', { name: 'Any soil unit' })).toHaveCount(1);
  await expect(panel.getByRole('searchbox')).toHaveCount(0);
  await page.screenshot({ path: `test-results/${test.info().project.name}-filter-sections.png` });
});

test('intended use requires an explicit published rating', async ({ page }, testInfo) => {
const panel = page.getByRole('complementary', { name: 'Parcel filters' });
await panel.getByRole('button', { name: 'Soil filters', exact: true }).click();
const use = panel.getByRole('combobox', { name: 'Intended use', exact: true });
const rating = panel.getByRole('combobox', { name: /Published rating/ });
await expect(rating).toBeDisabled();
await use.selectOption('conventional-septic');
await expect(rating).toBeEnabled();
await expect(rating).toHaveValue('');
await expect(rating).toHaveAttribute('required', '');
await expect(rating).toHaveAttribute('aria-invalid', 'true');
await expect(panel.getByRole('alert')).toHaveText('Select a published rating to apply this intended use.');
await panel.getByRole('alert').scrollIntoViewIfNeeded();
await page.screenshot({ path: testInfo.outputPath('rating-required.png') });
const paths: string[] = [];
page.on('request', request => { if (request.url().includes('/api/v1/parcels')) paths.push(request.url()); });
await rating.selectOption('Not limited');
await expect(panel.getByRole('alert')).toHaveCount(0);
await expect(rating).toHaveAttribute('aria-invalid', 'false');
for (const endpoint of ['/api/v1/parcels?', '/api/v1/parcels/map?']) {
await expect.poll(() => paths.some(path => path.includes(endpoint) && new URL(path).searchParams.get('soil_use') === 'conventional-septic' && new URL(path).searchParams.get('soil_rating') === 'Not limited')).toBe(true);
}
await page.screenshot({ path: testInfo.outputPath('rating-selected.png') });
await use.selectOption('dwelling-with-basement');
await expect(rating).toHaveValue('');
await expect(panel.getByRole('alert')).toBeVisible();
await use.selectOption('');
await expect(rating).toBeDisabled();
await expect(panel.getByRole('alert')).toHaveCount(0);
});
test('filter counts and removal controls update with parcel requests', async ({ page }) => {
  const dataRequests: string[] = [];
  page.on('request', (request) => { if (request.url().includes('/api/')) dataRequests.push(request.url()); });
  const panel = page.getByRole('complementary', { name: 'Parcel filters' });
  await panel.getByRole('checkbox', { name: 'Entity-name signal' }).check();
  await panel.getByRole('checkbox', { name: 'Out-of-state mailing' }).check();
  await panel.getByRole('checkbox', { name: 'Reported delinquent taxes' }).check();
  await expect(panel.getByRole('button', { name: 'Owner filters', exact: true })).toContainText('2');
  await expect(panel.getByRole('button', { name: 'Tax filters', exact: true })).toContainText('1');
  await expect(panel.getByText('3 filters selected', { exact: true })).toBeVisible();
  await panel.getByRole('button', { name: 'Owner filters', exact: true }).focus();
  await page.keyboard.press('Enter');
  await expect(panel.getByRole('checkbox', { name: 'Out-of-state mailing' })).not.toBeVisible();
  await page.keyboard.press('Enter');
  await expect(panel.getByRole('checkbox', { name: 'Out-of-state mailing' })).toBeChecked();
  await panel.getByRole('button', { name: 'Remove Entity-name signal', exact: true }).click();
  await expect(panel.getByRole('checkbox', { name: 'Entity-name signal' })).not.toBeChecked();
  await expect(panel.getByRole('checkbox', { name: 'Out-of-state mailing' })).toBeChecked();
  await expect(panel.getByRole('button', { name: 'Remove Out-of-state mailing', exact: true })).toBeFocused();
  await expect(panel.getByText('2 filters selected', { exact: true })).toBeVisible();
  await panel.getByRole('button', { name: 'Clear all', exact: true }).click();
  await expect(panel.getByText('0 filters selected', { exact: true })).toBeVisible();
  await expect(panel.getByRole('checkbox', { name: 'Out-of-state mailing' })).not.toBeChecked();
  await expect(panel.getByRole('checkbox', { name: 'Reported delinquent taxes' })).not.toBeChecked();
  await expect(page.getByRole('heading', { name: 'Matching parcels' })).toBeVisible();
  expect(dataRequests.some(path => path.includes('/api/v1/parcels?'))).toBe(true);
});

test('filters survive hidden panels and regional zoom with matching results', async ({ page }) => {
  const panel = page.getByRole('complementary', { name: 'Parcel filters' });
  await panel.getByRole('checkbox', { name: 'Foreclosure record' }).check();
  await panel.getByRole('button', { name: 'Parcel filters', exact: true }).click();
  await panel.getByRole('spinbutton', { name: 'Minimum acres' }).fill('0');
  await expect(panel.getByText('2 filters selected', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Hide map panel' }).click();
  await page.getByRole('button', { name: 'Show all counties', exact: true }).click();
  await expect(panel).not.toBeVisible();
  await page.getByRole('button', { name: 'Show selected filters', exact: true }).click();
  await expect(panel.getByRole('checkbox', { name: 'Foreclosure record' })).toBeChecked();
  await expect(panel.getByRole('spinbutton', { name: 'Minimum acres' })).toHaveValue('0');
  await expect(page.getByRole('region', { name: 'Matching parcels' })).toContainText('No matching parcels.');
  await panel.getByRole('button', { name: 'Remove At least 0 acres', exact: true }).click();
  await expect(panel.getByRole('spinbutton', { name: 'Minimum acres' })).toHaveValue('');
  await expect(panel.getByText('1 filter selected', { exact: true })).toBeVisible();
});

test('all vacancy tiers request the same list and map filter', async ({ page }, testInfo) => {
  const panel = page.getByRole('complementary', { name: 'Parcel filters' });
  await panel.getByRole('button', { name: 'Parcel filters', exact: true }).click();
  const tier = panel.getByRole('combobox', { name: 'Vacancy tier' });
  const paths: string[] = [];
  page.on('request', request => {
    if (request.url().includes('/api/v1/parcels')) paths.push(request.url());
  });
  for (const [value, label] of [['strong_candidate', 'Strong candidate'], ['review', 'Review'], ['conflict', 'Conflict'], ['unknown', 'Unknown']]) {
    await tier.selectOption(value);
    await expect(panel.getByRole('button', { name: `Remove Vacancy: ${label.toLowerCase()}` })).toBeVisible();
    await expect.poll(() => paths.some(path => path.includes('/api/v1/parcels?') && path.includes(`vacancy_tier=${value}`))).toBe(true);
    await expect.poll(() => paths.some(path => path.includes('/api/v1/parcels/map?') && path.includes(`vacancy_tier=${value}`))).toBe(true);
    if (value === 'unknown') {
      await tier.scrollIntoViewIfNeeded();
      await expect(tier).toHaveValue('unknown');
      await page.screenshot({ path: testInfo.outputPath('unknown-vacancy-tier.png') });
    }
  }
  await panel.getByRole('button', { name: 'Remove Vacancy: unknown' }).click();
  await expect(tier).toHaveValue('');
});

test('expanded filter sections scroll inside the panel and keep the footer reachable', async ({ page }) => {
  await page.setViewportSize({ width: 768, height: 640 });
  const panel = page.getByRole('complementary', { name: 'Parcel filters' });
  await panel.getByRole('button', { name: 'Soil filters', exact: true }).click();
  await panel.getByRole('button', { name: 'Parcel filters', exact: true }).click();
  const scroll = panel.getByRole('region', { name: 'Filter sections', exact: true });
  expect(await scroll.evaluate((element) => element.scrollHeight > element.clientHeight)).toBe(true);
  await panel.getByRole('spinbutton', { name: 'Minimum acres' }).fill('20');
  await panel.getByRole('spinbutton', { name: 'Maximum acres' }).fill('10');
  await expect(panel.getByRole('alert')).toHaveText('Minimum acreage must not exceed maximum acreage.');
  await expect(panel.getByRole('button', { name: 'Clear all', exact: true })).toBeVisible();
  await panel.getByRole('button', { name: 'Clear all', exact: true }).click();
  await expect(panel.getByRole('alert')).toHaveCount(0);
  const box = await panel.boundingBox();
  expect(box!.y + box!.height).toBeLessThanOrEqual(640);
});
