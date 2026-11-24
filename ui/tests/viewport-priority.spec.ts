import { expect, test } from '@playwright/test';

const origin = `http://127.0.0.1:${process.env.PLAYWRIGHT_PORT ?? '5174'}`;
const parcel = (n: number, prefix: string) => ({ id: `a0000000-0000-4000-8000-${String(n).padStart(12, '0')}`, parcel_number: `${prefix}-${n}`, address: null });
const outside = Array.from({ length: 60 }, (_, i) => parcel(i + 1, 'OUTSIDE'));
const inside = Array.from({ length: 10 }, (_, i) => parcel(i + 61, 'INSIDE'));

test('viewport matches precede all county pages and reset on map movement', async ({ page }, info) => {
  await page.emulateMedia({ reducedMotion: 'reduce' });
  const listQueries: URLSearchParams[] = [];
  await page.route('**/*', route => {
    const url = new URL(route.request().url());
    if (url.origin !== origin || /^\/(api|auth)(\/|$)/.test(url.pathname)) return route.abort();
    return route.continue();
  });
  await page.route('**/api/me', route => route.fulfill({ json: { id: inside[0].id, email: 'fixture@example.test', name: 'Fixture' } }));
  await page.route('**/api/v1/**', route => {
    const url = new URL(route.request().url());
    if (url.pathname === '/api/v1/parcels/soil-types') return route.fulfill({ json: { items: [] } });
    if (url.pathname === '/api/v1/parcels') {
      listQueries.push(url.searchParams);
      const preferred = url.searchParams.has('viewport_bbox');
      const ordered = preferred ? [...inside, ...outside] : [...outside, ...inside];
      const start = url.searchParams.has('cursor') ? 50 : 0;
      return route.fulfill({ json: { items: ordered.slice(start, start + 50), count: 70, next_cursor: start ? undefined : 'next-fixture-page', source_stamp: 'fixture' } });
    }
    if (url.pathname === '/api/v1/parcels/map') return route.fulfill({ json: { features: [], match_count: 10, source_stamp: 'fixture' } });
    return route.abort();
  });
  await page.goto('/?county=37179');
  const results = page.getByRole('list', { name: 'Matching parcels' });
  await expect(results.getByRole('button').first()).toHaveText('INSIDE-61');
  await expect(results.getByRole('button').nth(9)).toHaveText('INSIDE-70');
  await expect(results.getByRole('button').nth(10)).toHaveText('OUTSIDE-1');
  await expect(results.getByRole('button')).toHaveCount(50);
  expect(listQueries.at(-1)?.get('discovery')).toBe('browse');
  expect(listQueries.at(-1)?.has('bbox')).toBe(false);
  await page.screenshot({ path: info.outputPath('viewport-first.png'), fullPage: true });
  await page.getByRole('button', { name: 'Next', exact: true }).click();
  await expect(results.getByRole('button').first()).toHaveText('OUTSIDE-41');
  await expect(results.getByRole('button')).toHaveCount(20);
  await page.screenshot({ path: info.outputPath('county-second.png'), fullPage: true });
  const previousBounds = listQueries.at(-1)?.get('viewport_bbox');
  await page.getByRole('button', { name: 'Zoom in', exact: true }).click();
  await expect(page.getByRole('navigation', { name: 'Result pages' })).toContainText('Page 1');
  await expect.poll(() => listQueries.at(-1)?.get('viewport_bbox')).not.toBe(previousBounds);
  await expect(results.getByRole('button').first()).toHaveText('INSIDE-61');
  expect(listQueries.at(-1)?.has('cursor')).toBe(false);
  await page.screenshot({ path: info.outputPath('after-zoom.png'), fullPage: true });
  await page.getByRole('checkbox', { name: 'Foreclosure record' }).check();
  await expect.poll(() => listQueries.at(-1)?.get('foreclosure')).toBe('true');
  await expect(results.getByRole('button').first()).toHaveText('INSIDE-61');
  expect(listQueries.at(-1)?.has('viewport_bbox')).toBe(true);
  expect(listQueries.at(-1)?.has('cursor')).toBe(false);
});
