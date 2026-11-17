import { test, expect } from '@playwright/test';

function contrastRatio(first: string, second: string): number {
  function luminance(color: string): number {
    const channels = color.match(/[\d.]+/g)?.slice(0, 3).map(Number);
    if (!channels || channels.length !== 3) throw new Error('Expected an RGB color');
    const linear = channels.map((channel) => {
      const value = channel / 255;
      return value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4;
    });
    return linear[0] * 0.2126 + linear[1] * 0.7152 + linear[2] * 0.0722;
  }
  const values = [luminance(first), luminance(second)].sort((a, b) => b - a);
  return (values[0] + 0.05) / (values[1] + 0.05);
}

// Discovery tests exercise a signed-in browser; authentication has separate tests.
test.beforeEach(async ({ page }) => {
  await page.route('**/api/me', (route) => route.fulfill({ json: { id: 'sample-user', email: 'buyer@example.test', name: 'Sample Buyer' } }));
});

test('map starts with neutral parcel boundaries and zero filter matches', async ({ page }) => {
  await page.goto('/');
  const viewport = page.viewportSize()!;
  const map = (await page.locator('canvas.maplibregl-canvas').boundingBox())!;
  expect(map.width).toBe(viewport.width);
  expect(map.height / viewport.height).toBeGreaterThan(0.9);
  await expect(page.getByRole('heading', { name: '0 matches', exact: true })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Choose filters to find parcels' })).toBeVisible();
  await expect(page.getByText('Matching parcels (0)', { exact: true })).toBeVisible();
  await expect(page.getByText('Parcel boundary', { exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: /DEMO-/ })).toHaveCount(0);
  const results = (await page.getByRole('region', { name: 'Matching parcels' }).boundingBox())!;
  expect(results.x).toBeGreaterThan(viewport.width / 2);
  expect(results.width / viewport.width).toBeLessThan(0.4);
  await expect(page.getByRole('radio')).toHaveCount(0);
  await expect(page.locator('header')).not.toContainText('Union County');
  await expect(page.locator('header')).not.toContainText('/ Discovery');
  await expect(page.getByRole('button', { name: 'Go to parcel' })).toHaveCount(0);
  await page.screenshot({ animations: 'disabled', path: `test-results/${test.info().project.name}-map-first.png` });
  await page.getByRole('button', { name: 'Show filters', exact: true }).click();
  const filters = (await page.getByRole('complementary', { name: 'Parcel filters' }).boundingBox())!;
  expect(filters.x).toBeLessThan(viewport.width / 2);
  expect(filters.x + filters.width).toBeLessThan(results.x);
  await page.screenshot({ animations: 'disabled', path: `test-results/${test.info().project.name}-side.png` });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth && document.documentElement.scrollHeight <= window.innerHeight)).toBe(true);
});

test('search suggestions open a parcel directly without adding filter matches', async ({ page }) => {
  await page.goto('/');
  const finder = page.getByLabel('Find parcel ID or address');
  await finder.fill('Sample Oak Lane');
  await expect(page.getByRole('heading', { name: '0 matches', exact: true })).toBeVisible();
  await expect(page.getByRole('region', { name: 'Parcel details' })).toHaveCount(0);
  await page.getByRole('option', { name: '100 Sample Oak Lane · DEMO-001' }).click();
  await expect(page.getByRole('region', { name: 'Parcel details' })).toContainText('DEMO-001');
  await expect(page.getByText('This parcel is not in the current filter results.')).toBeVisible();
  await expect(page.getByText('Matching parcels (0)', { exact: true })).toBeVisible();
  await expect(finder).toHaveAttribute('aria-expanded', 'false');
  await page.getByRole('button', { name: 'Close parcel details' }).click();
  await expect(finder).toBeFocused();
  await expect(page.getByRole('heading', { name: '0 matches', exact: true })).toBeVisible();
  // A neutral, nonmatching boundary remains selectable on the map.
  const canvas = page.locator('canvas.maplibregl-canvas');
  const box = (await canvas.boundingBox())!;
  await page.waitForTimeout(500);
  await canvas.click({ position: { x: box.width / 2, y: box.height / 2 } });
  await expect(page.getByRole('region', { name: 'Parcel details' })).toContainText('DEMO-001');
  await expect(page.getByText('Matching parcels (0)', { exact: true })).toBeVisible();
  await page.screenshot({ path: `test-results/${test.info().project.name}-neutral-selected.png` });
  await page.getByRole('button', { name: 'Close parcel details' }).click();
  await finder.fill('DEMO-002');
  await finder.press('Enter');
  await expect(page.getByRole('region', { name: 'Parcel details' })).toContainText('DEMO-002');
});

test('details replace results and close restores filtered results', async ({ page }) => {
  const errors: string[] = [];
  const external: string[] = [];
  page.on('pageerror', (e) => errors.push(e.message));
  page.on('request', (r) => { if (/^https?:/.test(r.url()) && !r.url().startsWith('http://127.0.0.1:5174')) external.push(r.url()); });
  await page.goto('/');
  await page.getByRole('button', { name: 'Show filters', exact: true }).click();
  await page.getByLabel('Likely vacant', { exact: true }).check();
  await expect(page.getByRole('heading', { name: '12 matches' })).toBeVisible();
  await expect(page.getByText('Matching parcels (12)', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Next', exact: true }).click();
  await expect(page.getByText('Page 2 of 2')).toBeVisible();
  const resultsBox = (await page.getByRole('region', { name: 'Matching parcels' }).boundingBox())!;
  const row = page.getByRole('button', { name: /DEMO-013/ });
  await row.click();
  const details = page.getByRole('region', { name: 'Parcel details' });
  await expect(details).toContainText('DEMO-013');
  await expect(page.getByRole('region', { name: 'Matching parcels' })).toHaveCount(0);
  const detailsBox = (await details.boundingBox())!;
  expect(detailsBox.x).toBe(resultsBox.x);
  expect(detailsBox.width).toBe(resultsBox.width);
  await expect(page.getByLabel('Likely vacant', { exact: true })).toBeChecked();
  await expect(page.getByText('Matching parcels (12)', { exact: true })).toBeVisible();
  await page.screenshot({ animations: 'disabled', path: `test-results/${test.info().project.name}-details.png` });
  await page.getByRole('button', { name: 'Close parcel details' }).click();
  await expect(page.getByRole('heading', { name: '12 matches' })).toBeVisible();
  await expect(page.getByText('Page 2 of 2')).toBeVisible();
  await expect(row).toBeFocused();
  await page.getByLabel('Mailing outside NC', { exact: true }).check();
  await page.getByLabel('LLC-name match', { exact: true }).check();
  await expect(page.getByRole('heading', { name: '4 matches' })).toBeVisible();
  await expect(page.getByText('Matching parcels (4)', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Clear filters' }).click();
  await expect(page.getByRole('heading', { name: '0 matches', exact: true })).toBeVisible();
  await page.getByLabel('Minimum acres').fill('10');
  await page.getByLabel('Maximum acres').fill('2');
  await expect(page.getByRole('alert')).toHaveText('Minimum acreage must not exceed maximum acreage.');
  await expect(page.getByRole('heading', { name: 'No matching parcels' })).toBeVisible();
  expect(errors).toEqual([]);
  expect(external).toEqual([]);
});

test('result paging supports keyboard use and clamps after filters are cleared', async ({ page }) => {
  await page.goto('/');
  await page.getByRole('button', { name: 'Show filters', exact: true }).click();
  await page.getByLabel('Minimum acres').fill('0');
  await expect(page.getByRole('heading', { name: '24 matches' })).toBeVisible();
  const next = page.getByRole('button', { name: 'Next', exact: true });
  const previous = page.getByRole('button', { name: 'Previous', exact: true });
  await expect(previous).toBeDisabled();
  await next.click();
  await next.click();
  await next.click();
  await expect(page.getByText('Page 4 of 4')).toBeVisible();
  await expect(next).toBeDisabled();
  await previous.focus();
  await page.keyboard.press('Enter');
  await expect(page.getByText('Page 3 of 4')).toBeVisible();
  await page.getByLabel('Maximum acres').fill('1');
  await expect(page.getByRole('heading', { name: 'No matching parcels' })).toBeVisible();
  await expect(page.getByText('Page 1 of 1')).toBeVisible();
  await expect(next).toBeDisabled();
  await expect(previous).toBeDisabled();
  await page.getByRole('button', { name: 'Clear filters' }).click();
  await expect(page.getByRole('heading', { name: '0 matches', exact: true })).toBeVisible();
  await expect(next).toBeDisabled();
  await page.getByLabel('Minimum acres').fill('0');
  await expect(page.getByText('Page 1 of 4')).toBeVisible();
  await expect(next).toBeEnabled();
});

test('keyboard lookup and return work without WebGL or active filters', async ({ page }) => {
  await page.addInitScript(() => {
    const original = HTMLCanvasElement.prototype.getContext;
    HTMLCanvasElement.prototype.getContext = function (this: HTMLCanvasElement, ...args: Parameters<typeof original>) {
      if (String(args[0]).includes('webgl')) return null;
      return original.apply(this, args);
    } as typeof original;
  });
  await page.goto('/');
  await expect(page.getByRole('status')).toContainText('Map unavailable');
  const finder = page.getByLabel('Find parcel ID or address');
  await finder.fill('DEMO-001');
  await finder.press('Enter');
  await expect(page.getByRole('heading', { name: '100 Sample Oak Lane' })).toBeFocused();
  await page.keyboard.press('Tab');
  await expect(page.getByRole('button', { name: 'Close parcel details' })).toBeFocused();
  await page.keyboard.press('Enter');
  await expect(page.getByRole('heading', { name: '0 matches', exact: true })).toBeVisible();
  await expect(finder).toBeFocused();
});

test('nonmatching parcels remain visible and selectable after filtering', async ({ page }) => {
  await page.goto('/');
  await page.getByRole('button', { name: 'Show filters', exact: true }).click();
  await page.getByLabel('Minimum acres').fill('0');
  await page.getByRole('button', { name: 'Next', exact: true }).click();
  await page.getByRole('button', { name: /DEMO-007/ }).click();
  await page.getByLabel('LLC-name match', { exact: true }).check();
  await expect(page.getByText('Matching parcels (8)', { exact: true })).toBeVisible();
  await expect(page.getByRole('region', { name: 'Parcel details' })).toContainText('DEMO-007');
  await page.getByLabel('Likely vacant', { exact: true }).check();
  await expect(page.getByRole('heading', { name: '4 matches' })).toBeVisible();
  const canvas = page.locator('canvas.maplibregl-canvas');
  const box = (await canvas.boundingBox())!;
  await page.waitForTimeout(500);
  await canvas.click({ position: { x: box.width / 2, y: box.height / 2 } });
  await expect(page.getByRole('region', { name: 'Parcel details' })).toContainText('DEMO-007');
  await expect(page.getByText('This parcel is not in the current filter results.')).toBeVisible();
  await expect(page.getByText('Matching parcels (4)', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Clear filters' }).click();
  await expect(page.getByText('Matching parcels (0)', { exact: true })).toBeVisible();
  await expect(page.getByRole('region', { name: 'Parcel details' })).toContainText('DEMO-007');
  await page.getByRole('button', { name: 'Close parcel details' }).click();
  await expect(page.getByRole('heading', { name: '0 matches', exact: true })).toBeVisible();
});

test('matches can be hidden without changing filters, map highlights, or list page', async ({ page }) => {
  await page.goto('/');
  await page.getByRole('button', { name: 'Show filters', exact: true }).click();
  await page.getByLabel('Likely vacant', { exact: true }).check();
  await page.getByRole('button', { name: 'Next', exact: true }).click();
  await expect(page.getByText('Page 2 of 2')).toBeVisible();
  await page.getByRole('button', { name: /Hide matches/ }).click();
  await expect(page.getByRole('region', { name: 'Matching parcels' })).toHaveCount(0);
  await expect(page.getByRole('button', { name: /Show matches/ })).toHaveAttribute('aria-expanded', 'false');
  await expect(page.getByLabel('Likely vacant', { exact: true })).toBeChecked();
  await expect(page.getByText('Matching parcels (12)', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: /Show matches/ }).click();
  await expect(page.getByText('Page 2 of 2')).toBeVisible();
  await page.getByRole('button', { name: /DEMO-013/ }).click();
  await page.getByRole('button', { name: 'Close parcel details' }).click();
  await page.getByRole('button', { name: /Hide matches/ }).click();
  const canvas = page.locator('canvas.maplibregl-canvas');
  const box = (await canvas.boundingBox())!;
  await page.waitForTimeout(500);
  await canvas.click({ position: { x: box.width / 2, y: box.height / 2 } });
  await expect(page.getByRole('region', { name: 'Parcel details' })).toContainText('DEMO-013');
  await page.getByRole('button', { name: 'Close parcel details' }).click();
  await expect(page.getByText('Page 2 of 2')).toBeVisible();
});

test('only edge arrows hide filter and results panels and preserve panel state', async ({ page }) => {
  await page.goto('/');
  await page.getByRole('button', { name: 'Show filters', exact: true }).click();
  await page.getByLabel('Likely vacant', { exact: true }).check();
  await expect(page.getByRole('button', { name: 'Hide filters panel', exact: true })).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Hide results panel', exact: true })).toHaveCount(0);
  await page.getByRole('button', { name: 'Hide filters', exact: true }).click();
  await expect(page.getByRole('button', { name: 'Show filters', exact: true })).toBeFocused();
  await expect(page.locator('#filters')).toBeHidden();
  await page.keyboard.press('Enter');
  await expect(page.getByLabel('Likely vacant', { exact: true })).toBeChecked();
  await page.getByRole('button', { name: 'Hide matches', exact: true }).click();
  await expect(page.getByRole('button', { name: 'Show matches', exact: true })).toBeFocused();
  await expect(page.locator('#parcel-panel')).toBeHidden();
  await page.keyboard.press('Enter');
  await expect(page.getByRole('heading', { name: '12 matches' })).toBeVisible();
  await page.getByRole('button', { name: /DEMO-001/ }).click();
  await page.getByRole('button', { name: 'Hide parcel details panel', exact: true }).click();
  await expect(page.getByRole('region', { name: 'Parcel details' })).toHaveCount(0);
  await page.getByRole('button', { name: 'Show parcel details panel', exact: true }).click();
  await expect(page.getByRole('region', { name: 'Parcel details' })).toContainText('DEMO-001');
  await page.getByRole('button', { name: 'Close parcel details', exact: true }).click();
  await expect(page.getByRole('heading', { name: '12 matches' })).toBeVisible();
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await expect(page.locator('.right-panel')).toHaveCSS('transition-duration', '0s');
  await expect(page.locator('.left-panels')).toHaveCSS('transition-duration', '0s');
  await expect(page.locator('#filter-toggle')).toHaveCSS('transition-duration', '0s');
});

test('active-filter buttons remove individual filters while the panel stays closed', async ({ page }) => {
  await page.goto('/');
  await page.getByRole('button', { name: 'Show filters', exact: true }).click();
  await page.getByLabel('Minimum acres').fill('0');
  await page.getByLabel('Maximum acres').fill('30');
  await page.getByLabel('Likely vacant', { exact: true }).check();
  await page.getByLabel('Mailing outside NC', { exact: true }).check();
  await page.getByLabel('LLC-name match', { exact: true }).check();
  await expect(page.getByRole('heading', { name: '4 matches' })).toBeVisible();
  await page.getByRole('button', { name: 'Hide filters', exact: true }).click();
  await expect.poll(async () => {
    const label = await page.getByRole('button', { name: 'Remove LLC-name match', exact: true }).boundingBox();
    const panel = await page.getByRole('region', { name: 'Matching parcels' }).boundingBox();
    return label !== null && panel !== null && label.y + label.height <= panel.y;
  }).toBe(true);
  await page.screenshot({ animations: 'disabled', path: `test-results/${test.info().project.name}-removable-filters.png` });
  await page.getByRole('button', { name: 'Remove LLC-name match', exact: true }).click();
  await expect(page.getByRole('heading', { name: '6 matches' })).toBeVisible();
  await expect(page.getByText('Matching parcels (6)', { exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Show filters', exact: true })).toHaveAttribute('aria-expanded', 'false');
  await expect(page.getByRole('button', { name: 'Remove Likely vacant', exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Remove Mailing outside NC', exact: true }).click();
  await expect(page.getByRole('heading', { name: '12 matches' })).toBeVisible();
  const vacancy = page.getByRole('button', { name: 'Remove Likely vacant', exact: true });
  await vacancy.focus();
  await page.keyboard.press('Enter');
  await expect(page.getByRole('heading', { name: '24 matches' })).toBeVisible();
  await page.getByRole('button', { name: 'Remove At most 30 acres', exact: true }).click();
  await page.getByRole('button', { name: 'Show filters', exact: true }).click();
  await expect(page.getByLabel('Minimum acres')).toHaveValue('0');
  await expect(page.getByLabel('Maximum acres')).toHaveValue('');
  await expect(page.getByLabel('Likely vacant', { exact: true })).not.toBeChecked();
  await expect(page.getByLabel('Mailing outside NC', { exact: true })).not.toBeChecked();
  await expect(page.getByLabel('LLC-name match', { exact: true })).not.toBeChecked();
  await page.getByRole('button', { name: 'Hide filters', exact: true }).click();
  await page.getByRole('button', { name: 'Remove At least 0 acres', exact: true }).click();
  await expect(page.getByRole('heading', { name: '0 matches', exact: true })).toBeVisible();
  await expect(page.getByText('Matching parcels (0)', { exact: true })).toBeVisible();
  await expect(page.getByText('Parcel boundary', { exact: true })).toBeVisible();
  await expect(page.getByText('Active filters:', { exact: true })).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Show filters', exact: true })).toBeFocused();
  await page.keyboard.press('Enter');
  await expect(page.getByLabel('Minimum acres')).toHaveValue('');
});

test('removing an acreage filter clears its error and preserves keyboard focus', async ({ page }) => {
  await page.goto('/');
  await page.getByRole('button', { name: 'Show filters', exact: true }).click();
  await page.getByLabel('Minimum acres').fill('10');
  await page.getByLabel('Maximum acres').fill('2');
  await expect(page.getByRole('alert')).toBeVisible();
  await page.getByRole('button', { name: 'Hide filters', exact: true }).click();
  await page.getByRole('button', { name: 'Remove At most 2 acres', exact: true }).focus();
  await page.keyboard.press('Enter');
  await expect(page.getByRole('alert')).toHaveCount(0);
  await expect(page.getByRole('heading', { name: '8 matches' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Remove At least 10 acres', exact: true })).toBeFocused();
  await page.keyboard.press('Enter');
  await expect(page.getByRole('heading', { name: '0 matches', exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Show filters', exact: true })).toBeFocused();
});

test('filter controls use blue keyboard focus rather than amber', async ({ page }) => {
  await page.goto('/');
  const arrow = page.getByRole('button', { name: 'Show filters', exact: true });
  await arrow.focus();
  await expect(arrow).toHaveCSS('outline-color', 'rgb(96, 165, 250)');
  await expect(arrow).toHaveCSS('outline-style', 'solid');
  await page.keyboard.press('Enter');
  const acreage = page.getByLabel('Minimum acres');
  await expect(acreage).toBeVisible();
  await acreage.focus();
  await expect(acreage).toBeFocused();
  await expect(acreage).toHaveCSS('outline-color', 'rgb(96, 165, 250)');
  await expect(page.locator('.legend .selection')).toHaveCSS('border-top-color', 'rgb(237, 186, 113)');
});

test('dark mode is fixed under light and dark system preferences', async ({ page }) => {
  for (const colorScheme of ['light', 'dark'] as const) {
    await page.emulateMedia({ colorScheme });
    await page.goto('/');
    await expect(page.locator('html')).toHaveCSS('color-scheme', 'dark');
    await expect(page.locator('html')).toHaveCSS('background-color', 'rgb(36, 39, 43)');
    await expect(page.getByLabel('Find parcel ID or address')).toHaveCSS('background-color', 'rgb(48, 52, 56)');
    await expect(page.locator('.maplibregl-ctrl-group')).toHaveCSS('background-color', 'rgb(48, 52, 56)');
    await expect(page.locator('.legend .context')).toHaveCSS('border-top-color', 'rgb(128, 137, 148)');
    await expect(page.locator('.legend .match')).toHaveCSS('border-top-color', 'rgb(147, 197, 253)');
    await expect(page.locator('.legend .selection')).toHaveCSS('border-top-color', 'rgb(237, 186, 113)');
    const colors = await page.getByRole('region', { name: 'Matching parcels' }).evaluate((panel) => ({
      background: getComputedStyle(panel).backgroundColor,
      text: getComputedStyle(panel).color,
      muted: getComputedStyle(panel.querySelector('p')!).color
    }));
    expect(contrastRatio(colors.text, colors.background)).toBeGreaterThanOrEqual(4.5);
    expect(contrastRatio(colors.muted, colors.background)).toBeGreaterThanOrEqual(4.5);
    expect(contrastRatio('rgb(128, 137, 148)', 'rgb(36, 39, 43)')).toBeGreaterThanOrEqual(3);
  }
});
