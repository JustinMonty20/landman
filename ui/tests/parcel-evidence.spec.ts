import { test, expect } from '@playwright/test';

const first = 'a0000000-0000-4000-8000-000000000001';
const second = 'a0000000-0000-4000-8000-000000000002';
const origin = `http://127.0.0.1:${process.env.PLAYWRIGHT_PORT ?? '5174'}`;
test('selected result keeps saved evidence when map layers change', async ({ page }, testInfo) => {
 await page.emulateMedia({ reducedMotion: 'reduce' });
 await page.route('**/*', route => {
  const url = new URL(route.request().url());
  if (url.origin !== origin || /^\/(api|auth)(\/|$)/.test(url.pathname)) return route.abort();
  return route.continue();
 });
 await page.route('**/api/me', route => route.fulfill({ json: { id: first, email: 'fixture@example.test', name: 'Fixture' } }));
 await page.route('**/api/me/parcel-detail-fields', route => route.fulfill({ json: { fields: ['mapped_acres', 'soil_septic', 'delinquent_total', 'zoning_district', 'road_distance', 'flood_unknown'] } }));
 const requested: string[] = [];
 const externalRequests: string[] = [];
 page.on('request', request => { if (['property.spatialest.com', 'unionnc.devnetwedge.com', 'www.google.com'].includes(new URL(request.url()).hostname)) externalRequests.push(request.url()); });
 await page.route('**/api/v1/**', route => {
  const url = new URL(route.request().url()); requested.push(url.pathname);
  if (url.pathname === '/api/v1/parcels') return route.fulfill({ json: { items: [{ id: first, parcel_number: '00123450', address: 'Fixture Way' }, { id: second, parcel_number: 'SECOND', address: null }], count: 2, source_stamp: 'fixture' } });
  if (url.pathname === '/api/v1/parcels/map') {
   const [west, south, east, north] = (url.searchParams.get('bbox') ?? '').split(',').map(Number);
   const atFirst = Number(url.searchParams.get('zoom')) >= 16 && west < -80.7 && east > -80.7 && south < 34.89 && north > 34.89;
   const ring = [[-80.701, 34.889], [-80.699, 34.889], [-80.699, 34.891], [-80.701, 34.891], [-80.701, 34.889]];
   return route.fulfill({ json: { features: atFirst ? [{ id: first, geometry: { type: 'Polygon', coordinates: [ring] }, match: true }] : [], match_count: 2, source_stamp: 'fixture' } });
  }
  if (url.pathname.endsWith('/soil')) return route.fulfill({ json: { available: false, reason: 'no published assessment' } });
  if (url.pathname.endsWith('/tax')) return route.fulfill({ json: { bills: [], delinquent_balances: [], reported_delinquent_total: null, pipeline: [], foreclosure_cases: [], note: 'Missing evidence is not a verified zero or a payoff quote' } });
  if (url.pathname.endsWith('/vacancy')) return route.fulfill({ json: { available: false, tier: 'unknown', reason: 'validated_evidence_not_published' } });
  if (url.pathname.endsWith('/zoning')) return route.fulfill({ json: { available: false, freshness: { stale: true, reasons: ['source_unavailable'], source_fetched_at: null, refresh_failed_at: null } } });
  if (url.pathname.endsWith('/roads')) return route.fulfill({ status: 503, body: 'Unavailable' });
  if (url.pathname.endsWith('/flood')) return route.fulfill({ json: { status: 'no_release', release: null, parcel_number: 'FIRST' } });
  if (url.pathname.match(/\/parcels\/[a-f0-9-]+$/)) return route.fulfill({ json: { location: url.pathname.endsWith(first) ? { type: 'Point', coordinates: [-80.7, 34.89] } : null, parcel: { parcel_number: url.pathname.endsWith(first) ? '00123450' : null, address: 'Fixture Way', mapped_acres: '2.50', likely_vacant: true, updated_at: '2026-01-01' }, source: 'union_county_gis', source_parcel_id: 'fixture-id', owner_labels: null, sales: [] } });
  if (url.pathname === '/api/v1/qualification/map') return route.fulfill({ json: { type: 'FeatureCollection', features: [], generation: null } });
  if (url.pathname === '/api/v1/flood/map') return route.fulfill({ json: { type: 'FeatureCollection', features: [], release: null } });
  return route.abort();
 });
 await page.goto('/');
 await page.getByRole('button', { name: 'Inspect Union, NC', exact: true }).click();
 await page.getByRole('tab', { name: 'Filters', exact: true }).click();
 await page.getByRole('combobox', { name: 'Flood category' }).selectOption('sfha');
 await page.getByRole('tab', { name: 'Layers', exact: true }).click();
 await page.getByRole('checkbox', { name: 'FEMA flood zones' }).uncheck();
 const moved = page.waitForRequest(request => {
  const url = new URL(request.url());
  if (url.pathname !== '/api/v1/parcels/map' || Number(url.searchParams.get('zoom')) < 16) return false;
  const [west, south, east, north] = (url.searchParams.get('bbox') ?? '').split(',').map(Number);
  return west < -80.7 && east > -80.7 && south < 34.89 && north > 34.89 && url.searchParams.get('flood_category') === 'sfha';
 });
 await page.getByRole('button', { name: /00123450 · Fixture Way/ }).click();
 await moved;
 const panel = page.getByRole('region', { name: 'Parcel evidence' });
 await expect(panel).toContainText('00123450 · Site address: Fixture Way');
 await expect(panel.getByRole('link', { name: 'Open satellite view' })).toHaveCount(0);
 const evidenceRequests = requested.filter(path => path.startsWith('/api/v1/parcels/') && path.includes(first)).length;
 await panel.getByRole('tab', { name: 'Research links' }).focus();
 await page.keyboard.press('Enter');
 const sites = panel.getByRole('list', { name: 'External research sites' });
 await expect(sites.getByRole('listitem')).toHaveCount(3);
 await expect(panel).toContainText('County parcel number: 00123450');
 for (const [name, url] of [['Union County property records', 'https://property.spatialest.com/nc/union#/property/00123450'], ['Union County tax records', 'https://unionnc.devnetwedge.com/parcel/view/00123450/2026']] as const) {
  const link = sites.getByRole('link', { name });
  await expect(link).toHaveAttribute('href', url);
  await expect(link).toHaveAttribute('target', '_blank');
  await expect(link).toHaveAttribute('rel', 'noopener noreferrer');
 }
 await expect(sites.getByRole('link', { name: 'Open satellite view' })).toHaveAttribute('href', /center=34\.89%2C-80\.7.*basemap=satellite/);
 expect(requested.filter(path => path.startsWith('/api/v1/parcels/') && path.includes(first))).toHaveLength(evidenceRequests);
 expect(externalRequests).toEqual([]);
 await page.screenshot({ path: testInfo.outputPath('research-links.png'), fullPage: true });
 for (const [name, url, pattern] of [
   ['Union County property records', 'https://property.spatialest.com/nc/union#/property/00123450', 'https://property.spatialest.com/**'],
   ['Union County tax records', 'https://unionnc.devnetwedge.com/parcel/view/00123450/2026', 'https://unionnc.devnetwedge.com/**']
 ] as const) {
   await page.context().route(pattern, route => route.fulfill({ contentType: 'text/html', body: '<title>County record fixture</title>' }));
   const newTab = page.waitForEvent('popup');
   await sites.getByRole('link', { name }).click();
   const recordTab = await newTab;
   await expect(recordTab).toHaveURL(url);
   await recordTab.close();
 }
 const canvas = page.locator('canvas.maplibregl-canvas');
 const bounds = (await canvas.boundingBox())!;
 const center = { x: bounds.x + bounds.width / 2, y: bounds.y + bounds.height / 2 };
 await expect(async () => {
  await page.mouse.move(center.x, center.y);
  await expect(canvas).toHaveCSS('cursor', 'pointer', { timeout: 250 });
 }).toPass({ timeout: 5000 });
 await page.mouse.click(center.x, center.y);
 await expect(panel).toContainText('County parcel number: 00123450');
 await expect(sites.getByRole('link', { name: 'Union County property records' })).toHaveAttribute('href', 'https://property.spatialest.com/nc/union#/property/00123450');
 await expect(sites.getByRole('link', { name: 'Open satellite view' })).toHaveAttribute('href', /center=34\.89%2C-80\.7.*basemap=satellite/);
 expect(requested.filter(path => path.startsWith('/api/v1/parcels/') && path.includes(first))).toHaveLength(evidenceRequests);
 await page.screenshot({ path: testInfo.outputPath('research-map-reselected.png'), fullPage: true });
 await panel.getByRole('tab', { name: 'Evidence', exact: true }).click();
 await expect(panel.getByRole('link', { name: 'Open satellite view' })).toHaveCount(0);
 await expect(panel).toContainText('No published soil assessment');
 await expect(panel).toContainText('Not recorded');
 await expect(panel).toContainText('No FEMA release stored');
 expect(requested.some(path => path.endsWith('/flood'))).toBe(true);
 expect(requested.some(path => path.endsWith('/zoning'))).toBe(true);
 await page.screenshot({ path: testInfo.outputPath('parcel-core.png'), fullPage: true });
 await page.getByRole('checkbox', { name: 'Zoning and jurisdiction' }).check();
 await page.getByRole('checkbox', { name: 'Roads' }).check();
 await page.getByRole('checkbox', { name: 'FEMA flood zones' }).check();
 await expect(panel).toContainText('No published zoning assessment');
 await expect(panel).toContainText('Evidence temporarily unavailable');
 await expect(panel).toContainText('No FEMA release stored');
 await page.screenshot({ path: testInfo.outputPath('parcel-layers.png'), fullPage: true });
 await panel.getByRole('region', { name: 'Mapped roads' }).scrollIntoViewIfNeeded();
 await expect(panel.getByRole('region', { name: 'Mapped roads' })).toBeVisible();
 await page.screenshot({ path: testInfo.outputPath('parcel-layers-bottom.png'), fullPage: true });
 await page.getByRole('checkbox', { name: 'FEMA flood zones' }).uncheck();
 await expect(panel.getByRole('region', { name: 'FEMA flood mapping' })).toBeVisible();
 await page.getByRole('button', { name: 'Close parcel details' }).click();
 await expect(page.getByRole('region', { name: 'Matching parcels' })).toContainText('00123450');
 await page.getByRole('button', { name: /SECOND/ }).click();
 await expect(page.getByRole('region', { name: 'Parcel evidence' })).toContainText('Parcel');
 await panel.getByRole('tab', { name: 'Research links' }).click();
 await expect(panel).toContainText('County parcel number: Not recorded');
 await expect(panel.getByRole('link', { name: 'Union County property records' })).toHaveCount(0);
 await expect(panel.getByRole('link', { name: 'Union County tax records' })).toHaveCount(0);
 await expect(panel).toContainText('A county parcel number is required');
 await expect(panel.getByRole('link', { name: 'Open satellite view' })).toHaveCount(0);
 await expect(panel).not.toContainText('00123450');
 await page.screenshot({ path: testInfo.outputPath('research-missing.png'), fullPage: true });
 await panel.getByRole('tab', { name: 'Evidence', exact: true }).click();
 await expect(panel.getByRole('status').filter({ hasText: 'Parcel location is unavailable.' })).toBeVisible();
});
