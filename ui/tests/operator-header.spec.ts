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
  await page.route('**/api/me/crm/tasks*', route => route.fulfill({ json: { items: [], next: '' } }));
  await page.goto('/leads');
  await expect(page.getByRole('heading', { name: 'Leads', exact: true })).toHaveCount(1);
  const brand = page.getByRole('banner').getByRole('link', { name: 'Landman home' });
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
  await expect.poll(async () => {
   const current = await brand.boundingBox();
   return current && { width: current.width, height: current.height };
  }).toEqual({ width: bounds!.width, height: bounds!.height });
  await expect(page.getByRole('link', { name: 'Leads', exact: true })).toBeVisible();
  await page.screenshot({ path: info.outputPath('parcels.png') });
 });
}
test('header measurement waits for the signed-in brand after a delayed session', async ({ page }) => {
 await page.route('**/*', route => {
  const url = new URL(route.request().url());
  if (url.origin !== 'http://127.0.0.1:5174' || /^\/(api|auth)(\/|$)/.test(url.pathname)) return route.abort();
  return route.continue();
 });
 let reads = 0;
 let releaseSession: (() => void) | undefined;
 await page.route('**/api/me', async route => {
  if (++reads === 2) await new Promise<void>(resolve => { releaseSession = resolve; });
  await route.fulfill({ json: user });
 });
 await page.route('**/api/me/crm/**', route => route.fulfill({ json: { states: [], default_state_id: '', revision: 1, next: '' } }));
 const brand = page.getByRole('banner').getByRole('link', { name: 'Landman home' });
 await page.goto('/leads');
 await expect(brand).toBeVisible();
 const bounds = await brand.boundingBox();
 expect(bounds).not.toBeNull();
 await page.goto('/');
 await expect(page.getByRole('status')).toHaveText('Checking your session…');
 await expect(page.getByRole('link', { name: 'Landman home' })).toBeVisible();
 await expect(brand).toHaveCount(0);
 await expect.poll(() => !!releaseSession).toBe(true);
 releaseSession?.();
 await expect(brand).toBeVisible();
 await expect.poll(async () => {
  const current = await brand.boundingBox();
  return current && { width: current.width, height: current.height };
 }).toEqual({ width: bounds!.width, height: bounds!.height });
});

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
test('login and unavailable use the same home brand bounds as signed-in pages', async ({ page }, info) => {
 let resolveSession: (() => void) | undefined;
 await page.route('**/api/me', async route => {
   await new Promise<void>(resolve => { resolveSession = resolve; });
   await route.fulfill({ status: 401 });
 });
 await page.goto('/');
 const brand = page.getByRole('link', { name: 'Landman home' });
 await expect(page.getByRole('status')).toHaveText('Checking your session…');
 await expect(brand).toHaveCount(1);
 const loading = await brand.boundingBox();
 await expect.poll(() => !!resolveSession).toBe(true);
 resolveSession?.();
 await expect(page.getByRole('link', { name: 'Sign in with Google' })).toBeVisible();
 await expect(brand).toHaveAttribute('href', '/');
 const login = await brand.boundingBox();
 expect(loading).toMatchObject({ width: login!.width, height: login!.height });
 const styles = await brand.evaluate(el => ({ color: getComputedStyle(el).color, font: getComputedStyle(el).fontSize }));
 await page.screenshot({ path: info.outputPath('login-brand.png') });
 await page.route('**/api/me', route => route.fulfill({ status: 503 }));
 await page.reload();
 await expect(page.getByRole('alert')).toContainText('sign-in service is unavailable');
 await expect(brand).toHaveCount(1);
 expect(await brand.boundingBox()).toMatchObject({ width: login!.width, height: login!.height });
 await page.route('**/api/me', route => route.fulfill({ json: user }));
 await page.reload();
 await expect(page.getByRole('button', { name: 'Account menu' })).toBeVisible();
 const signedIn = await brand.boundingBox();
 expect(login).toMatchObject({ width: signedIn!.width, height: signedIn!.height });
 expect(await brand.evaluate(el => ({ color: getComputedStyle(el).color, font: getComputedStyle(el).fontSize }))).toEqual(styles);
});
test('keyboard sign-out stays visible pending, allows Escape, and retries after failure', async ({ page }, info) => {
 await page.route('**/api/me', route => route.fulfill({ json: user }));
 await page.route('**/api/me/crm/**', route => route.fulfill({ json: { states: [], default_state_id: '', revision: 1, next: '' } }));
 let finish: (() => void) | undefined;
 let calls = 0;
 await page.route('**/auth/logout', async route => {
   calls++;
   await new Promise<void>(resolve => { finish = resolve; });
   await route.fulfill({ status: calls === 1 ? 503 : 204 });
 });
 await page.goto('/leads');
 const trigger = page.getByRole('button', { name: 'Account menu' });
 await trigger.click();
 await page.getByRole('menu').focus();
 await page.keyboard.press('End');
 await expect(page.getByRole('menuitem', { name: 'Sign out' })).toHaveAttribute('data-highlighted', '');
 await page.keyboard.press('Enter');
 await expect(page.getByRole('menuitem', { name: 'Signing out…' })).toBeVisible();
 await expect(trigger).toHaveAttribute('aria-expanded', 'true');
 await page.screenshot({ path: info.outputPath('signout-pending.png') });
 await page.keyboard.press('Escape');
 await expect(trigger).toHaveAttribute('aria-expanded', 'false');
 await expect.poll(() => calls).toBe(1);
 finish?.();
 await expect(page.getByRole('alert').filter({ hasText: 'Sign-out failed' })).toBeVisible();
 await page.getByRole('menuitem', { name: 'Sign out' }).click();
 await expect(page.getByRole('menuitem', { name: 'Signing out…' })).toBeVisible();
 await expect(trigger).toHaveAttribute('aria-expanded', 'true');
 await expect.poll(() => calls).toBe(2);
 finish?.();
 await expect(page.getByRole('link', { name: 'Sign in with Google' })).toBeVisible();
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
