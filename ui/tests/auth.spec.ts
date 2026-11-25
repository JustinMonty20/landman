import { test, expect } from '@playwright/test';

test('signed-out users see Google sign-in, not the discovery application', async ({ page }) => {
  await page.route('**/api/me', (route) => route.fulfill({ status: 401, body: 'Sign-in required' }));
  await page.goto('/');
  await expect(page.getByRole('link', { name: 'Sign in with Google' })).toHaveAttribute('href', '/auth/google/start');
  const artwork = page.getByRole('img', { name: 'Sign in with Google' });
  await expect(artwork).toBeVisible();
  await expect(artwork).toHaveAttribute('src', '/brand/google-signin-dark.png');
  await expect.poll(() => artwork.evaluate((image) => (image as HTMLImageElement).naturalWidth)).toBe(360);
  await page.screenshot({ path: `test-results/${test.info().project.name}-google-login.png` });
  await expect(page.getByRole('main', { name: 'Parcel discovery' })).toHaveCount(0);
  await expect(page.locator('canvas')).toHaveCount(0);
});

test('provider errors are safe text and do not expose unapproved users', async ({ page }) => {
  await page.route('**/api/me', (route) => route.fulfill({ status: 401 }));
  await page.goto('/?auth_error=not_allowed');
  await expect(page.getByRole('alert')).toHaveText('This Google account is not approved for the private alpha.');
  await expect(page).toHaveURL('/');
  await expect(page.getByRole('link', { name: 'Sign in with Google' })).toBeVisible();
});

test('backend failure cannot bypass sign-in and retry can recover', async ({ page }) => {
  await page.route('**/api/me', (route) => route.fulfill({ status: 503 }));
  await page.goto('/');
  await expect(page.getByRole('alert')).toContainText('sign-in service is unavailable');
  await expect(page.locator('canvas')).toHaveCount(0);
  await page.route('**/api/me', (route) => route.fulfill({ status: 401 }));
  await page.getByRole('button', { name: 'Try again' }).click();
  await expect(page.getByRole('link', { name: 'Sign in with Google' })).toBeVisible();
});

test('logout removes discovery and returns to Google sign-in', async ({ page }) => {
  await page.route('**/api/me', (route) => route.fulfill({ json: { id: 'user-1', email: 'buyer@example.test', name: 'Buyer' } }));
  await page.route('**/auth/logout', async (route) => {
    expect(route.request().method()).toBe('POST');
    await route.fulfill({ status: 204 });
  });
  await page.goto('/');
  await expect(page.getByRole('link', { name: 'County coverage', exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Account menu' }).click();
await page.getByRole('menuitem', { name: 'Sign out', exact: true }).click();
  await expect(page.getByRole('link', { name: 'Sign in with Google' })).toBeVisible();
  await expect(page.locator('canvas')).toHaveCount(0);
});
