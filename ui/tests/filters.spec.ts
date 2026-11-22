import { test, expect } from '@playwright/test';

test.beforeEach(async ({ page }) => {
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await page.route('**/api/me', (route) => route.fulfill({ json: { id: 'sample-user', email: 'buyer@example.test', name: 'Buyer' } }));
  await page.goto('/?county=37179');
  await expect(page.getByRole('heading', { name: 'Filter Union parcels' })).toBeVisible();
});

test('mock filter sections use data-specific controls and explain unavailable soil choices', async ({ page }) => {
  const panel = page.getByRole('complementary', { name: 'Parcel filters' });
  await expect(panel.getByText(/mock|preview/i)).toHaveCount(0);
  await expect(panel.getByText('Parcel data is not connected yet. Selections do not run a search.')).toBeVisible();
  for (const name of ['Owner', 'Soil', 'Tax', 'Parcel']) {
    await expect(panel.getByRole('button', { name: `${name} filters`, exact: true })).toBeVisible();
  }
  await expect(panel.getByRole('checkbox', { name: 'Out-of-state owner' })).toBeEnabled();
  await expect(panel.getByText('Owner mailing address is outside North Carolina.')).toBeVisible();
  await panel.getByRole('button', { name: 'Soil filters', exact: true }).click();
  await expect(panel.getByRole('combobox', { name: 'Soil type', exact: true })).toBeDisabled();
  await expect(panel.getByRole('option', { name: 'No soil types loaded' })).toHaveCount(1);
  await expect(panel.getByRole('searchbox')).toHaveCount(0);
  await page.screenshot({ path: `test-results/${test.info().project.name}-filter-sections.png` });
});

test('preview counts and removal controls update without querying parcel data', async ({ page }) => {
  const dataRequests: string[] = [];
  page.on('request', (request) => { if (request.url().includes('/api/')) dataRequests.push(request.url()); });
  const panel = page.getByRole('complementary', { name: 'Parcel filters' });
  await panel.getByRole('checkbox', { name: 'Business-name match' }).check();
  await panel.getByRole('checkbox', { name: 'Out-of-state owner' }).check();
  await panel.getByRole('checkbox', { name: 'Reported delinquent taxes' }).check();
  await expect(panel.getByRole('button', { name: 'Owner filters', exact: true })).toContainText('2');
  await expect(panel.getByRole('button', { name: 'Tax filters', exact: true })).toContainText('1');
  await expect(panel.getByText('3 filters selected', { exact: true })).toBeVisible();
  await panel.getByRole('button', { name: 'Owner filters', exact: true }).focus();
  await page.keyboard.press('Enter');
  await expect(panel.getByRole('checkbox', { name: 'Out-of-state owner' })).not.toBeVisible();
  await page.keyboard.press('Enter');
  await expect(panel.getByRole('checkbox', { name: 'Out-of-state owner' })).toBeChecked();
  await panel.getByRole('button', { name: 'Remove Business-name match', exact: true }).click();
  await expect(panel.getByRole('checkbox', { name: 'Business-name match' })).not.toBeChecked();
  await expect(panel.getByRole('checkbox', { name: 'Out-of-state owner' })).toBeChecked();
  await expect(panel.getByRole('button', { name: 'Remove Out-of-state owner', exact: true })).toBeFocused();
  await expect(panel.getByText('2 filters selected', { exact: true })).toBeVisible();
  await panel.getByRole('button', { name: 'Clear all', exact: true }).click();
  await expect(panel.getByText('0 filters selected', { exact: true })).toBeVisible();
  await expect(panel.getByRole('checkbox', { name: 'Out-of-state owner' })).not.toBeChecked();
  await expect(panel.getByRole('checkbox', { name: 'Reported delinquent taxes' })).not.toBeChecked();
  await expect(page.getByRole('heading', { name: 'Parcel results unavailable' })).toBeVisible();
  expect(dataRequests).toEqual([]);
});

test('preview filters survive hidden panels and regional zoom without changing parcel results', async ({ page }) => {
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
  await expect(page.getByRole('region', { name: 'Matching parcels' })).toContainText('Filter selections do not run a search.');
  await panel.getByRole('button', { name: 'Remove At least 0 acres', exact: true }).click();
  await expect(panel.getByRole('spinbutton', { name: 'Minimum acres' })).toHaveValue('');
  await expect(panel.getByText('1 filter selected', { exact: true })).toBeVisible();
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
