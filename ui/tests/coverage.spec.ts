import { test, expect } from '@playwright/test';

test('coverage page requires sign-in', async ({ page }) => {
  await page.route('**/api/me', (route) => route.fulfill({ status: 401 }));
  await page.goto('/coverage');
  await expect(page.getByRole('link', { name: 'Sign in with Google' })).toBeVisible();
  await expect(page.getByRole('main', { name: 'County coverage' })).toHaveCount(0);
});

test('coverage details are separate from the map and link to each county', async ({ page }) => {
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await page.route('**/api/me', (route) => route.fulfill({ json: { id: 'user-1', email: 'buyer@example.test', name: 'Buyer' } }));
  await page.goto('/');
  await page.getByRole('button', { name: 'Inspect York, SC: Locked', exact: true }).click();
  await page.getByRole('link', { name: 'County coverage', exact: true }).click();
  await expect(page).toHaveURL('/coverage?county=45091');
  await expect(page.getByRole('heading', { name: 'County coverage', exact: true })).toBeVisible();
  const list = page.getByRole('list', { name: 'County ingestion status' });
  await expect(list.getByRole('listitem')).toHaveCount(9);
  await expect(list.getByRole('heading', { name: 'Chesterfield County, SC' })).toBeVisible();
  await expect(list.getByRole('heading', { name: 'Stanly County, NC' })).toBeVisible();
  await expect(list.getByText('Ingestion configured', { exact: true })).toHaveCount(1);
  await expect(list.getByText('Locked — ingestion not configured', { exact: true })).toHaveCount(8);
  await expect(page.getByRole('heading', { name: 'Map legend' })).toBeVisible();
  await expect(page.locator('canvas')).toHaveCount(0);
  await expect(page.getByRole('link', { name: 'Return to map' })).toHaveAttribute('href', '/?county=45091');
  await page.getByRole('link', { name: 'Return to map' }).click();
  await expect(page.getByRole('link', { name: 'County coverage', exact: true })).toHaveAttribute('href', '/coverage?county=45091');
  await expect(page.getByRole('status')).toContainText('York County is locked.');
  await page.getByRole('link', { name: 'County coverage', exact: true }).click();
  await page.getByRole('link', { name: 'Show Union County on map' }).click();
  await expect(page.getByRole('heading', { name: 'Filter Union parcels' })).toBeVisible();
  await expect(page.getByText('Ingestion configured', { exact: true })).toHaveCount(0);
});

test('unknown county links use the default map selection', async ({ page }) => {
  await page.route('**/api/me', (route) => route.fulfill({ json: { id: 'user-1', email: 'buyer@example.test', name: 'Buyer' } }));
  await page.goto('/?county=unknown');
  await expect(page.getByRole('link', { name: 'County coverage', exact: true })).toHaveAttribute('href', '/coverage?county=37179');
  await expect(page.getByRole('searchbox')).toHaveCount(0);
});
