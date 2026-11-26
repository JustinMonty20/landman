import { expect, test } from '@playwright/test';

const origin = 'http://127.0.0.1:5174';
const id = '11111111-1111-4111-8111-111111111111';
test.use({ video: 'on' });

for (const colorScheme of ['dark', 'light'] as const) {
 test(`parcel actions share one row and icon size with ${colorScheme} OS preference`, async ({ page }, info) => {
  await page.emulateMedia({ colorScheme, reducedMotion: 'reduce' });
  await page.route('**/*', route => {
   const url = new URL(route.request().url());
   return url.origin !== origin || /^\/(api|auth)(\/|$)/.test(url.pathname) ? route.abort() : route.continue();
  });
  await page.route('**/api/me', route => route.fulfill({ json: { id: 'fixture', email: 'buyer@example.test', name: 'Buyer' } }));
  await page.route('**/api/coverage*', route => route.fulfill({ json: { counties: [] } }));
  await page.route(`**/api/me/favorites/${id}`, route => route.fulfill(route.request().method() === 'GET' ? { status: 404 } : {
   json: { parcel_id: id, favorite: true, revision: 1, state_id: id, excluded: false, exclusion_reason: '' },
  }));
  await page.goto(`/?parcel=${id}`);
  const panel = page.getByRole('region', { name: 'Parcel evidence', exact: true });
  const actions = panel.getByRole('group', { name: 'Parcel actions', exact: true });
  const star = actions.getByRole('button', { name: 'Favorite parcel', exact: true });
  const close = actions.getByRole('button', { name: 'Close parcel details', exact: true });
  await expect(star).toBeEnabled();
  await expect(close).toBeVisible();
  await expect(actions.getByRole('button')).toHaveCount(2);
  const starBox = (await star.boundingBox())!;
  const closeBox = (await close.boundingBox())!;
  expect(starBox.width).toBe(20);
  expect(starBox.height).toBe(20);
  expect(closeBox).toMatchObject({ y: starBox.y, width: starBox.width, height: starBox.height });
  await expect(actions.getByRole('button').first()).toHaveAccessibleName('Close parcel details');
  expect(starBox.x - closeBox.x - closeBox.width).toBe(4);
  const starIcon = (await star.locator('svg').boundingBox())!;
  expect(starIcon.width).toBe(10);
  expect(starIcon.height).toBe(10);
  expect(await close.locator('svg').boundingBox()).toMatchObject({ y: starIcon.y, width: 10, height: 10 });
  await page.screenshot({ path: info.outputPath('01-actions.png'), fullPage: true });
  await star.focus();
  await expect(star).toBeFocused();
  await expect(star).toHaveCSS('outline-style', 'solid');
  await expect(star).toHaveCSS('outline-color', 'rgb(96, 165, 250)');
  await page.keyboard.press('Enter');
  const saved = actions.getByRole('button', { name: 'Remove favorite', exact: true });
  await expect(saved).toHaveAttribute('aria-pressed', 'true');
  await expect(saved).toHaveCSS('color', 'rgb(245, 189, 79)');
  await page.screenshot({ path: info.outputPath('02-saved.png'), fullPage: true });
  await page.keyboard.press('Shift+Tab');
  await expect(close).toBeFocused();
  await expect(close).toHaveCSS('outline-color', 'rgb(96, 165, 250)');
  await page.screenshot({ path: info.outputPath('03-close-focus.png'), fullPage: true });
  await page.keyboard.press('Enter');
  await expect(panel).toHaveCount(0);
  await expect(actions).toHaveCount(0);
 });
}
