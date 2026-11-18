import { test, expect } from '@playwright/test';

test.beforeEach(async ({ page }) => {
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await page.route('**/api/me', (route) => route.fulfill({ json: { id: 'sample-user', email: 'buyer@example.test', name: 'Buyer' } }));
});

test('regional map has clickable counties without a dropdown, legend, or empty filter count', async ({ page }) => {
  await page.goto('/');
  await expect(page.locator('.county-map-label')).toHaveCount(9);
  await expect(page.getByRole('combobox')).toHaveCount(0);
  await expect(page.getByText('0 active filters', { exact: true })).toHaveCount(0);
  await expect(page.getByRole('searchbox')).toHaveCount(0);
  await expect(page.getByRole('complementary', { name: 'Parcel filters' })).not.toBeVisible();
  await expect(page.getByRole('region', { name: 'Matching parcels' })).toHaveCount(0);
  await expect(page.getByText(/ingestion configured/i)).toHaveCount(0);
  await expect(page.getByText(/^(Regional|County|Parcel) view$/)).toHaveCount(0);
  await expect(page.getByRole('link', { name: 'County coverage', exact: true })).toBeVisible();
  await expect(page.getByLabel('Map legend', { exact: true })).toHaveCount(0);
  await expect(page.getByText('Boundaries: U.S. Census Bureau · Display only', { exact: true })).toBeVisible();
  const map = await page.getByRole('region', { name: 'County map', exact: true }).boundingBox();
  expect(map!.height).toBeGreaterThan(page.viewportSize()!.height * 0.9);
});

test('clicking Union opens filters and preserves panel preferences across zoom levels', async ({ page }) => {
  await page.goto('/');
  await page.getByRole('button', { name: 'Inspect Union, NC', exact: true }).click();
  await expect(page.getByRole('searchbox', { name: 'Find parcel ID or address' })).toBeDisabled();
  await expect(page.getByRole('heading', { name: 'Filter Union parcels' })).toBeVisible();
  const filters = await page.getByRole('complementary', { name: 'Parcel filters' }).boundingBox();
  expect(filters!.y).toBe(164);
  await page.getByRole('button', { name: 'Parcel filters', exact: true }).click();
  await expect(page.getByRole('spinbutton', { name: 'Minimum acres' })).toBeEnabled();
  await expect(page.getByRole('heading', { name: 'Parcel results unavailable' })).toBeVisible();
  await expect(page.getByRole('region', { name: 'Matching parcels' })).not.toContainText('Ingestion');
  await page.screenshot({ path: `test-results/${test.info().project.name}-union-county-view.png` });
  await page.getByRole('button', { name: 'Hide filters' }).click();
  await page.getByRole('button', { name: 'Show all counties', exact: true }).click();
  await expect(page.getByRole('searchbox')).toHaveCount(0);
  await expect(page.getByRole('region', { name: 'Matching parcels' })).toHaveCount(0);
  await page.getByRole('button', { name: 'Inspect Union, NC', exact: true }).click();
  await expect(page.getByRole('button', { name: 'Show filters' })).toBeVisible();
  await page.getByRole('button', { name: 'Show filters' }).click();
  await expect(page.getByRole('heading', { name: 'Filter Union parcels' })).toBeVisible();
});

test('map navigation remains clickable with both county panels open', async ({ page }) => {
  await page.goto('/');
  await page.getByRole('button', { name: 'Inspect Union, NC', exact: true }).click();
  await expect(page.getByRole('complementary', { name: 'Parcel filters' })).toBeVisible();
  await expect(page.getByRole('region', { name: 'Matching parcels' })).toBeVisible();
  await page.getByRole('button', { name: 'Locate selected county', exact: true }).click();
  await page.getByRole('button', { name: 'Show all counties', exact: true }).click();
  await expect(page.getByRole('searchbox')).toHaveCount(0);
  await expect(page.locator('.county-map-label:visible')).toHaveCount(9);
  await page.getByRole('button', { name: 'Locate selected county', exact: true }).click();
  await expect(page.getByRole('complementary', { name: 'Parcel filters' })).toBeVisible();
  await expect(page.getByRole('region', { name: 'Matching parcels' })).toBeVisible();
  await page.screenshot({ path: `test-results/${test.info().project.name}-open-panel-navigation.png` });
});

test('zoom and map movement change detail without changing the selected county', async ({ page }) => {
  await page.goto('/');
  await page.getByRole('button', { name: 'Inspect Union, NC', exact: true }).click();
  await page.getByRole('button', { name: 'Hide filters' }).click();
  await page.getByRole('button', { name: 'Hide matches' }).click();
  for (let i = 0; i < 6; i++) await page.getByRole('button', { name: 'Zoom in', exact: true }).click();
  await expect(page.locator('.county-map-label:visible')).toHaveCount(0);
  const canvas = await page.locator('.maplibregl-canvas').boundingBox();
  await page.mouse.move(canvas!.x + canvas!.width / 2, canvas!.y + canvas!.height / 2);
  await page.mouse.down();
  await page.mouse.move(canvas!.x + canvas!.width / 2 + 150, canvas!.y + canvas!.height / 2 + 100, { steps: 8 });
  await page.mouse.up();
  await expect(page.getByRole('link', { name: 'County coverage', exact: true })).toHaveAttribute('href', '/coverage?county=37179');
  for (let i = 0; i < 9; i++) await page.getByRole('button', { name: 'Zoom out', exact: true }).click();
  await expect(page.getByRole('searchbox')).toHaveCount(0);
  await page.getByRole('button', { name: 'Show all counties', exact: true }).click();
  await expect(page.locator('.county-map-label:visible')).toHaveCount(9);
  await expect(page.getByRole('link', { name: 'County coverage', exact: true })).toHaveAttribute('href', '/coverage?county=37179');
});

test('locked county labels show a short notice and keyboard selection can return to Union', async ({ page }) => {
  await page.goto('/');
  for (const name of ['Anson, NC', 'Lancaster, SC', 'Mecklenburg, NC', 'Cabarrus, NC', 'York, SC', 'Gaston, NC', 'Chesterfield, SC', 'Stanly, NC']) {
    await page.getByRole('button', { name: 'Show all counties', exact: true }).click();
    await page.getByRole('button', { name: `Inspect ${name}: Locked`, exact: true }).click();
    await expect(page.getByRole('status')).toContainText('is locked. Parcel discovery is not available.');
    await expect(page.getByRole('searchbox')).toHaveCount(0);
    await expect(page.getByRole('complementary', { name: 'Parcel filters' })).not.toBeVisible();
  }
  for (let i = 0; i < 6; i++) await page.getByRole('button', { name: 'Zoom in', exact: true }).click();
  await expect(page.locator('.county-map-label:visible')).toHaveCount(0);
  await expect(page.getByRole('region', { name: 'Matching parcels' })).toHaveCount(0);
  await page.getByRole('button', { name: 'Show all counties', exact: true }).click();
  await page.getByRole('button', { name: 'Inspect Union, NC', exact: true }).focus();
  await page.keyboard.press('Enter');
  await expect(page.getByRole('heading', { name: 'Filter Union parcels' })).toBeVisible();
});

test('map labels select counties and all regional outlines fit the map', async ({ page }) => {
  const external: string[] = [];
  page.on('request', (request) => {
    if (/^https?:/.test(request.url()) && !request.url().startsWith('http://127.0.0.1:5174')) external.push(request.url());
  });
  await page.goto('/');
  await page.getByRole('button', { name: 'Inspect York, SC: Locked', exact: true }).click();
  await expect(page.getByRole('status')).toContainText('York County is locked.');
  await page.getByRole('button', { name: 'Show all counties', exact: true }).click();
  for (const label of await page.locator('.county-map-label').all()) {
    const box = await label.boundingBox();
    expect(box!.x).toBeGreaterThanOrEqual(0);
    expect(box!.x + box!.width).toBeLessThan(page.viewportSize()!.width);
    expect(box!.y).toBeGreaterThan(52);
    expect(box!.y + box!.height).toBeLessThan(page.viewportSize()!.height - 90);
  }
  expect(external).toEqual([]);
  await page.screenshot({ path: `test-results/${test.info().project.name}-county-coverage.png` });
});

test('coverage page provides county selection without WebGL', async ({ page }) => {
  await page.addInitScript(() => {
    const original = HTMLCanvasElement.prototype.getContext;
    HTMLCanvasElement.prototype.getContext = function (this: HTMLCanvasElement, type: string, ...args: unknown[]) {
      if (type.includes('webgl')) return null;
      return original.apply(this, [type, ...args] as Parameters<typeof original>);
    } as typeof original;
  });
  await page.goto('/');
  await expect(page.getByRole('status')).toContainText('Map unavailable.');
  await page.getByRole('link', { name: 'County coverage', exact: true }).click();
  await page.getByRole('link', { name: 'Show Anson County on map' }).click();
  await expect(page.getByText('Anson County is locked. Parcel discovery is not available.')).toBeVisible();
  await page.getByRole('link', { name: 'County coverage', exact: true }).click();
  await page.getByRole('link', { name: 'Show Union County on map' }).click();
  await expect(page.getByRole('heading', { name: 'Filter Union parcels' })).toBeVisible();
});
