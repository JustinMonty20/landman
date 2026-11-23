import { expect, test } from '@playwright/test';

// A synthetic vector tile with a park, water and a primary road.
const tile = Buffer.from('GiEKBHBhcmsSFBgDIhAJ2ATYBBq4FwAAgBm3FwAPKIAgeAIaIgoFd2F0ZXISFBgDIhAJvAWoFBrQDwAAoAbPDwAPKIAgeAIaOQoOdHJhbnNwb3J0YXRpb24SEBICAAAYAiIICQCwIgqAQAAaBWNsYXNzIgkKB3ByaW1hcnkogCB4Ag==', 'base64');
const id = '00000000-0000-0000-0000-000000000001';
const parcel = { id, parcel_number: 'P-1', address: 'Fixture Road' };
function centerParcel(bbox: string) {
  const [west, south, east, north] = bbox.split(',').map(Number);
  const x = (west + east) / 2;
  const y = (south + north) / 2;
  const dx = (east - west) / 18;
  const dy = (north - south) / 8;
  const corner = [x - dx, y - dy];
  return { type: 'Polygon', coordinates: [[corner, [x + dx, y - dy], [x + dx, y + dy], [x - dx, y + dy], corner]] };
}
const provider = 'https://tiles.openfreemap.org';

for (const blocked of [false, true]) {
  test(`parcel browsing with the basemap ${blocked ? 'blocked' : 'available'}`, async ({ page }, info) => {
    let tileRequests = 0;
    let metadataRequests = 0;
    await page.emulateMedia({ reducedMotion: 'reduce' });
    await page.route('**/*', async route => {
      const url = new URL(route.request().url());
      if (url.origin === provider) {
        if (url.pathname === '/planet') {
          metadataRequests++;
          if (blocked) return route.abort();
          return route.fulfill({ json: { tilejson: '3.0.0', tiles: [`${provider}/fixture/{z}/{x}/{y}.pbf`], minzoom: 0, maxzoom: 14 } });
        }
        if (url.pathname.startsWith('/fixture/')) {
          tileRequests++;
          return route.fulfill({ body: tile, contentType: 'application/vnd.mapbox-vector-tile' });
        }
        return route.abort();
      }
      if (url.origin !== info.project.use.baseURL) return route.abort();
      if (!/^\/(api|auth)(\/|$)/.test(url.pathname)) return route.continue();
      if (url.pathname === '/api/me') return route.fulfill({ json: { id, email: 'buyer@example.test', name: 'Buyer' } });
      if (url.pathname === '/api/me/parcel-detail-groups') return route.fulfill({ json: { groups: ['acreage'] } });
      if (url.pathname === '/api/v1/parcels/soil-types') return route.fulfill({ json: { items: [] } });
      if (url.pathname === '/api/v1/parcels/map') {
        const geometry = centerParcel(url.searchParams.get('bbox')!);
        const features = Number(url.searchParams.get('zoom')) < 13 ? [] : [{ ...parcel, geometry, match: url.searchParams.has('foreclosure') }];
        return route.fulfill({ json: { mode: features.length ? 'features' : 'grid', features, match_count: 1, source_stamp: 'fixture' } });
      }
      if (url.pathname === '/api/v1/parcels') return route.fulfill({ json: { items: [parcel], count: 1, source_stamp: 'fixture' } });
      if (url.pathname === `/api/v1/parcels/${id}`) return route.fulfill({ json: { parcel, sales: [] } });
      return route.abort();
    });

    await page.goto('/?county=37179');
    const result = page.getByRole('button', { name: 'P-1 · Fixture Road', exact: true });
    await expect(result).toBeVisible();
    await expect(page.getByRole('link', { name: '© OpenStreetMap contributors' })).toBeInViewport();
    if (blocked) {
      await expect(page.getByText('Some basemap details are unavailable.')).toBeVisible();
    } else {
      await expect.poll(() => tileRequests).toBeGreaterThan(0);
      await expect(page.getByText('Some basemap details are unavailable.')).toHaveCount(0);
    }
    const locate = page.getByRole('button', { name: 'Locate selected county' });
    await expect(locate).toBeEnabled();
    const closeView = page.waitForResponse(response => response.url().includes('/parcels/map?') && Number(new URL(response.url()).searchParams.get('zoom')) >= 13);
    for (let i = 0; i < 5; i++) await page.getByRole('button', { name: 'Zoom in', exact: true }).click();
    await closeView;
    const canvas = page.locator('canvas.maplibregl-canvas');
    const bounds = (await canvas.boundingBox())!;
    const x = bounds.x + bounds.width / 2;
    await expect(async () => {
      await page.mouse.move(x, bounds.y + bounds.height * 0.8);
      await page.mouse.move(x, bounds.y + bounds.height / 2);
      await expect(canvas).toHaveCSS('cursor', 'pointer', { timeout: 250 });
    }).toPass({ timeout: 5000 });
    await page.screenshot({ path: info.outputPath('01-unfiltered.png') });

    const mapRequest = page.waitForResponse(response => response.url().includes('/parcels/map?') && new URL(response.url()).searchParams.has('foreclosure'));
    await page.getByRole('checkbox', { name: 'Foreclosure record' }).check();
    await mapRequest;
    await expect(result).toBeVisible();
    await page.screenshot({ path: info.outputPath('02-filtered.png') });
    await result.click();
    await expect(page.getByRole('heading', { name: 'Parcel evidence', exact: true })).toBeVisible();
    await expect(page.getByRole('region', { name: 'Parcel identity' })).toContainText('P-1 · Fixture Road');
    await expect(page.getByRole('heading', { name: 'Acreage', exact: true })).toBeVisible();
    await expect(locate).toBeEnabled();
    await expect(page.getByText('Map unavailable.', { exact: false })).toHaveCount(0);
    expect(metadataRequests).toBe(1);
    await page.screenshot({ path: info.outputPath('03-selected.png') });
  });
}
