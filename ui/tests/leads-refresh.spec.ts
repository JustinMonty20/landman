import { expect, test } from '@playwright/test';

const origin = 'http://127.0.0.1:5174';
const states = [
 { id: '11111111-1111-4111-8111-111111111111', name: 'New', position: 0, revision: 1, count: 1 },
 { id: '22222222-2222-4222-8222-222222222222', name: 'Review', position: 1, revision: 1, count: 0 },
];
const card = { parcel_id: '33333333-3333-4333-8333-333333333333', parcel_number: '00123', address: 'Fixture parcel', state_id: states[0].id, revision: 1, excluded: false, exclusion_reason: '', favorited_at: '2026-01-01' };

test.use({ video: 'on' });

for (const failure of ['none', 'board', 'column'] as const) {
 test(`state change keeps board mounted during refresh: ${failure}`, async ({ page }, info) => {
  let moved = false;
  let recovered = false;
  let releaseBoard!: () => void;
  let releaseCards!: () => void;
  const boardGate = new Promise<void>(resolve => releaseBoard = resolve);
  const cardGate = new Promise<void>(resolve => releaseCards = resolve);
  await page.route('**/*', route => {
   const url = new URL(route.request().url());
   if(url.origin===origin && url.pathname==='/api/me/crm/tasks')return route.fulfill({json:{items:[],next:''}});
   return url.origin !== origin || /^\/(api|auth)(\/|$)/.test(url.pathname) ? route.abort() : route.continue();
  });
  await page.route('**/api/me', route => route.fulfill({ json: { id: 'fixture', email: 'buyer@example.test', name: 'Buyer' } }));
  await page.route('**/api/me/crm/board', async route => {
   if (moved) {
    await boardGate;
    if (failure === 'board' && !recovered) return route.fulfill({ status: 503 });
   }
   return route.fulfill({ json: { states: states.map((s, i) => ({ ...s, count: moved ? i : 1 - i })), default_state_id: states[0].id, revision: 1, next: '' } });
  });
  await page.route('**/api/me/crm/board/*', async route => {
   const id = new URL(route.request().url()).pathname.split('/').pop();
   if (moved) {
    await cardGate;
    if (failure === 'column' && !recovered && id === states[0].id) return route.fulfill({ status: 503 });
   }
   const stateId = moved ? states[1].id : states[0].id;
   return route.fulfill({ json: { items: id === stateId ? [{ ...card, state_id: stateId, revision: moved ? 2 : 1 }] : [], next: '' } });
  });
  await page.route(`**/api/me/favorites/${card.parcel_id}/state`, route => {
   moved = true;
   return route.fulfill({ json: {} });
  });
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await page.goto('/leads');
  const canvas = page.getByLabel('Lead states', { exact: true });
  const source = page.getByRole('region', { name: 'New', exact: true });
  const target = page.getByRole('region', { name: 'Review', exact: true });
  await expect(source.locator('article')).toHaveCount(1);
  const originalCanvas = await canvas.elementHandle();
  const originalCard = await source.locator('article').elementHandle();
  const bounds = await canvas.boundingBox();
  await page.screenshot({ path: info.outputPath('01-before.png'), fullPage: true });
  const refreshRequest = page.waitForRequest(request => request.url().endsWith('/api/me/crm/board'));
  await page.getByRole('combobox', { name: 'Change state for 00123' }).selectOption(states[1].id);
  await refreshRequest;
  await expect(canvas).toHaveAttribute('aria-busy', 'true');
  expect(await originalCanvas!.evaluate(node => node.isConnected)).toBe(true);
  expect(await originalCard!.evaluate(node => node.isConnected)).toBe(true);
  expect(await canvas.boundingBox()).toEqual(bounds);
  await expect(page.getByText('Loading your board…')).toHaveCount(0);
  await page.screenshot({ path: info.outputPath('02-refresh.png'), fullPage: true });
  if (failure !== 'board') {
   const cardsRequest = page.waitForRequest(request => request.url().includes(`/crm/board/${states[1].id}?`));
   releaseBoard();
   await cardsRequest;
   expect(await originalCanvas!.evaluate(node => node.isConnected)).toBe(true);
   expect(await originalCard!.evaluate(node => node.isConnected)).toBe(true);
   await expect(source.locator('article')).toHaveCount(1);
  } else releaseBoard();
  releaseCards();
  await expect(canvas).toHaveAttribute('aria-busy', 'false');
  expect(await originalCanvas!.evaluate(node => node.isConnected)).toBe(true);
  if (failure === 'board') {
   await expect(page.getByRole('alert')).toContainText('503');
   await expect(source.locator('article')).toHaveCount(1);
   recovered = true;
   await page.getByRole('button', { name: 'Retry board load' }).click();
   await expect(target.locator('article')).toHaveCount(1);
   await expect(source.locator('article')).toHaveCount(0);
  } else {
   await expect(target.locator('article')).toHaveCount(1);
   if (failure === 'column') {
    await expect(source.getByRole('alert')).toContainText('503');
    recovered = true;
    await source.getByRole('button', { name: 'Retry New cards' }).click();
    await expect(source.getByRole('alert')).toHaveCount(0);
    await expect(source.locator('article')).toHaveCount(0);
   } else await expect(source.locator('article')).toHaveCount(0);
  }
  await page.screenshot({ path: info.outputPath('03-result.png'), fullPage: true });
 });
}
