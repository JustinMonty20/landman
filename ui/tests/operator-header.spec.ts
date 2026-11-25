import { test, expect } from '@playwright/test';

const user = { id: 'fixture', email: 'fixture@example.test', name: 'Fixture Buyer' };
for (const photo of ['', 'https://lh3.googleusercontent.com/a/fixture']) {
 test(`shared header and avatar menu ${photo ? 'photo failure' : 'missing photo'}`, async ({ page }, info) => {
  await page.route('**/*', route => {
   const url = new URL(route.request().url());
   if (url.origin !== 'http://127.0.0.1:5174' || /^\/(api|auth)(\/|$)/.test(url.pathname)) return route.abort();
   return route.continue();
  });
  await page.route('**/api/me', route => route.fulfill({ json: { ...user, ...(photo ? { profile_image_url: photo } : {}) } }));
  await page.route('https://lh3.googleusercontent.com/**', route => route.abort());
  await page.route('**/api/me/crm/**', route => route.fulfill({ json: { states: [], default_state_id: '', revision: 1, next: '' } }));
  await page.goto('/leads');
  await expect(page.getByRole('heading', { name: 'Leads', exact: true })).toHaveCount(1);
  const brand = page.getByRole('link', { name: 'Landman home' });
  await expect(brand).toBeVisible();
  const bounds = await brand.boundingBox();
  await expect(page.getByRole('link', { name: 'Parcels', exact: true })).toBeVisible();
  const trigger = page.getByRole('button', { name: 'Account menu' });
  await expect(trigger).toHaveAttribute('aria-expanded', 'false');
  await expect(page.getByText(user.email)).toBeHidden();
  await expect(trigger.locator('span').filter({ hasText: 'FB' })).toBeVisible();
  await page.screenshot({ path: info.outputPath('leads.png') });
  await trigger.click();
  await expect(page.getByText(user.email)).toBeVisible();
  await expect(page.getByRole('menuitem', { name: 'Sign out' })).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(trigger).toBeFocused();
  await expect(trigger).toHaveAttribute('aria-expanded', 'false');
  await page.goto('/');
  await expect(brand).toBeVisible();
  expect(await brand.boundingBox()).toMatchObject({ width: bounds!.width, height: bounds!.height });
  await expect(page.getByRole('link', { name: 'Leads', exact: true })).toBeVisible();
  await page.screenshot({ path: info.outputPath('parcels.png') });
 });
}
test('fixture photo loads without external network and coverage shares brand', async ({ page }, info) => {
 await page.route('**/api/me', route => route.fulfill({ json: { ...user, profile_image_url: 'https://lh3.googleusercontent.com/a/fixture' } }));
 await page.route('https://lh3.googleusercontent.com/**', route => route.fulfill({ contentType: 'image/png', body: Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/4eUAAAAASUVORK5CYII=', 'base64') }));
 await page.goto('/coverage');
 const trigger = page.getByRole('button', { name: 'Account menu' });
 await expect(trigger.locator('img')).toHaveJSProperty('naturalWidth', 1);
 await expect(trigger.locator('img')).toHaveAttribute('referrerpolicy', 'no-referrer');
 await expect(page.getByRole('link', { name: 'Landman home' })).toBeVisible();
 await page.screenshot({ path: info.outputPath('coverage-photo.png') });
});
test('failed sign-out retains account and menu error, retry works', async ({ page }) => {
 await page.route('**/api/me', route => route.fulfill({ json: user }));
 let calls = 0;
 await page.route('**/auth/logout', route => route.fulfill({ status: ++calls === 1 ? 503 : 204 }));
 await page.route('**/api/me/crm/**', route => route.fulfill({ json: { states: [], default_state_id: '', revision: 1, next: '' } }));
 await page.goto('/leads');
 await page.getByRole('button', { name: 'Account menu' }).click();
 await page.getByRole('menuitem', { name: 'Sign out' }).click();
 await expect(page.getByRole('alert').filter({ hasText: 'Sign-out failed' })).toHaveText('Sign-out failed. Try again.');
 await expect(page.getByRole('menuitem', { name: 'Sign out' })).toBeVisible();
 await page.getByRole('menuitem', { name: 'Sign out' }).click();
 await expect(page.getByRole('link', { name: 'Sign in with Google' })).toBeVisible();
});
