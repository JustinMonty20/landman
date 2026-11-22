import { test, expect } from '@playwright/test';

test('Filters and Layers tabs retain independent selections across tabs and zoom levels', async ({ page }, testInfo) => {
  const requested: string[] = [];
  const generation = { id: 'test-generation', fetched_at: '2026-09-15T00:00:00Z', refresh_failed_at: null };
  const release = { id: 'test-release', issued_at: '2026-09-14T00:00:00Z', observed_at: generation.fetched_at };
  const district = [[-80.63, 34.96], [-80.57, 34.96], [-80.57, 35.03], [-80.63, 35.03], [-80.63, 34.96]];
  const road = [[-80.78, 34.98], [-80.35, 35.04]];
  const zone = [[-80.63, 34.97], [-80.39, 34.98], [-80.39, 34.992], [-80.63, 34.983], [-80.63, 34.97]];
  let roadsRejected = 0;
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await page.route('**/*', (route) => {
    const url = new URL(route.request().url());
    if (url.origin !== testInfo.project.use.baseURL || /^\/(api|auth)(\/|$)/.test(url.pathname)) return route.abort();
    return route.continue();
  });
  await page.route('**/api/me', (route) => route.fulfill({ json: { id: 'buyer', email: 'buyer@example.test', name: 'Buyer' } }));
  await page.route('**/api/v1/parcels/map?*', (route) => {
    if (new URL(route.request().url()).searchParams.get('jurisdiction') === 'etj:Test Town') {
      requested.push('etj:map');
      return route.fulfill({ json: { features: [], match_count: 1, source_stamp: 'fixture' } });
    }
    return route.fulfill({ json: { features: [] } });
  });
  await page.route('**/api/v1/qualification/map?*', (route) => {
    const { searchParams } = new URL(route.request().url());
    const layer = searchParams.get('layer');
    const overview = searchParams.get('overview') === 'true';
    if (searchParams.get('jurisdiction') === 'etj:Test Town') {
      requested.push('etj:overlay');
      return route.fulfill({ json: { type: 'FeatureCollection', generation, overview, features: [
        { type: 'Feature', geometry: { type: 'MultiLineString', coordinates: [district] }, properties: { kind: 'jurisdiction' } }
      ] } });
    }
    requested.push(`${layer}:${overview ? 'overview' : 'detail'}`);
    if (layer === 'roads' && !overview) {
      roadsRejected++;
      return route.fulfill({ status: 413 });
    }
    const features = layer === 'roads'
      ? [
          { type: 'Feature', geometry: { type: 'MultiLineString', coordinates: [road] }, properties: { kind: 'roads' } },
          { type: 'Feature', geometry: { type: 'Point', coordinates: [-80.43, 35.0288] }, properties: { kind: 'road_label', name: 'Test Highway' } }
        ]
      : [
          { type: 'Feature', geometry: { type: 'MultiPolygon', coordinates: [[district]] }, properties: { kind: 'zoning' } },
          { type: 'Feature', geometry: { type: 'MultiLineString', coordinates: [district] }, properties: { kind: 'jurisdiction' } },
          { type: 'Feature', geometry: { type: 'Point', coordinates: [-80.6, 34.995] }, properties: { kind: 'municipality_label', name: 'Test Town' } },
          { type: 'Feature', geometry: { type: 'Point', coordinates: [-80.47, 34.95] }, properties: { kind: 'etj_label', name: 'Test Town' } }
        ];
    return route.fulfill({ json: { type: 'FeatureCollection', features, generation, overview } });
  });
  await page.route('**/api/v1/flood/map?*', (route) => {
    const overview = new URL(route.request().url()).searchParams.get('overview') === 'true';
    requested.push(`flood:${overview ? 'overview' : 'detail'}`);
    const features = [
      { code: 'X', subtype: 'AREA OF MINIMAL FLOOD HAZARD', sfha: 'F', offset: -.025 },
      { code: 'X', subtype: '0.2 PCT ANNUAL CHANCE FLOOD HAZARD', sfha: 'F', offset: -.013 },
      { code: 'AE', subtype: '', sfha: 'T', offset: 0 }
    ].map(({ offset, ...properties }) => ({
      type: 'Feature', geometry: { type: 'MultiPolygon', coordinates: [[zone.map(([lon, lat]) => [lon, lat + offset])]] }, properties
    }));
    return route.fulfill({ json: { type: 'FeatureCollection', release, overview, features } });
  });
  await page.route('**/api/v1/qualification/zoning?*', (route) => route.fulfill({ json: {
    generation_id: generation.id,
    districts: [
      { code: 'CZ', jurisdiction: 'Test Town', description: 'Test district', case_reference: 'TEST-24-01', source_date: '2026-09-01' },
      { code: 'R-20', jurisdiction: 'Test County', description: '', case_reference: '', source_date: '' }
    ]
  } }));
  await page.route('**/api/v1/parcels?*', (route) => {
    if (new URL(route.request().url()).searchParams.get('jurisdiction') !== 'etj:Test Town') return route.abort();
    requested.push('etj:list');
    return route.fulfill({ json: { items: [{ id: 'test-match', parcel_number: 'ETJ-TEST' }], count: 1, source_stamp: 'fixture' } });
  });
  await page.goto('/');
  const layers = page.getByRole('group', { name: 'Map layers' });
  await expect(layers).toHaveCount(0);
  await page.getByRole('button', { name: 'Inspect Union, NC', exact: true }).click();
  const filtersTab = page.getByRole('tab', { name: 'Filters', exact: true });
  const layersTab = page.getByRole('tab', { name: 'Layers', exact: true });
  const businessName = page.getByRole('checkbox', { name: 'Business-name match' });
  await expect(filtersTab).toHaveAttribute('aria-selected', 'true');
  await expect(layersTab).toBeVisible();
  await expect(layers).toBeHidden();
  await expect(businessName).toBeVisible();
  await page.screenshot({ path: testInfo.outputPath('01-filters-default.png') });
  await businessName.check();
  await layersTab.click();
  await expect(layersTab).toHaveAttribute('aria-selected', 'true');
  await expect(businessName).toBeHidden();
  const zoning = layers.getByRole('checkbox', { name: 'Zoning and jurisdiction' });
  const roads = layers.getByRole('checkbox', { name: 'Roads', exact: true });
  const flood = layers.getByRole('checkbox', { name: 'FEMA flood zones' });
  for (const toggle of [zoning, roads, flood]) {
    await expect(toggle).toBeVisible();
    await expect(toggle).not.toBeChecked();
  }
  expect(requested).toEqual([]);
  await page.screenshot({ path: testInfo.outputPath('02-layers-off.png') });
  await zoning.focus();
  await zoning.press('Space');
  await expect(zoning).toBeChecked();
  await roads.check();
  const roadsStatus = page.getByText('Overview: main roads only.');
  const zoningStatus = page.getByText('Overview: larger zoning areas only.');
  await expect(roadsStatus).toBeVisible();
  await expect(zoningStatus).toBeVisible();
  await expect(flood).not.toBeChecked();
  await flood.check();
  const floodStatus = page.getByText('Overview: simplified flood boundaries.');
  await expect(floodStatus).toBeVisible();
  expect(requested).toEqual(['zoning:overview', 'roads:overview', 'flood:overview']);
  const zoningLegend = page.getByRole('list', { name: 'Zoning and jurisdiction legend' });
  await expect(zoningLegend.getByText('Zoning area', { exact: true })).toBeVisible();
  await expect(zoningLegend.getByText('Municipality or ETJ boundary', { exact: true })).toBeVisible();
  const mapArea = page.getByRole('region', { name: 'County map' });
  const townLabel = mapArea.getByText('Test Town', { exact: true });
  await expect(townLabel).toBeVisible();
  await expect(mapArea.getByText('Test Town ETJ', { exact: true })).toBeVisible();
  await expect(page.getByText('Test Highway', { exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Zone X — 0.2% annual chance', exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Zone X — minimal flood hazard', exact: true })).toBeVisible();
  for (const toggle of [zoning, roads, flood]) await expect(toggle).toBeChecked();
  await page.getByRole('button', { name: 'Hide matches' }).click();
  await page.screenshot({ path: testInfo.outputPath('03-layers-on.png') });
  await page.getByRole('button', { name: 'Hide map panel' }).click();
  await page.screenshot({ path: testInfo.outputPath('06-map-labels.png') });
  const labelBounds = await townLabel.boundingBox();
  if (!labelBounds) throw new Error('Town label is not displayed');
  await page.mouse.click(labelBounds.x + labelBounds.width / 2, labelBounds.y + labelBounds.height / 2);
  const districts = page.getByRole('list', { name: 'Zoning districts at selected location' });
  const withCase = districts.getByRole('listitem').filter({ hasText: 'CZ · Test Town' });
  await expect(withCase).toContainText('Source description: Test district');
  await expect(withCase).toContainText('Case reference: TEST-24-01');
  await expect(withCase).toContainText('Source record date: 2026-09-01');
  const withoutCase = districts.getByRole('listitem').filter({ hasText: 'R-20 · Test County' });
  await expect(withoutCase).toBeVisible();
  await expect(withoutCase).not.toContainText(/Case reference|Source record date|Source description/);
  await expect(page.getByText('Check the local zoning ordinance', { exact: false })).toBeVisible();
  await page.screenshot({ path: testInfo.outputPath('05-zoning-selection.png') });
  const etj = page.getByRole('button', { name: 'Test Town ETJ', exact: true });
  await etj.click();
  await expect(etj).toHaveAttribute('aria-pressed', 'true');
  await expect(page.getByRole('button', { name: 'Test Town', exact: true })).toHaveAttribute('aria-pressed', 'false');
  await expect(page.getByRole('button', { name: 'Show matches' })).toBeVisible();
  await page.getByRole('button', { name: 'Show matches' }).click();
  await expect(page.getByRole('button', { name: 'ETJ-TEST' })).toBeVisible();
  await expect.poll(() => ['etj:overlay', 'etj:list', 'etj:map'].every((request) => requested.includes(request))).toBe(true);
  await page.screenshot({ path: testInfo.outputPath('07-jurisdiction-filter.png') });
  await etj.press('Space');
  await expect(etj).toHaveAttribute('aria-pressed', 'false');
  await flood.uncheck();
  await expect(floodStatus).toBeHidden();
  await expect(zoning).toBeChecked();
  await expect(roads).toBeChecked();
  for (let i = 0; i < 6 && !roadsRejected; i++) {
    await page.getByRole('button', { name: 'Zoom in', exact: true }).click();
  }
  await expect.poll(() => roadsRejected).toBeGreaterThan(0);
  await expect(roadsStatus).toBeVisible();
  await expect(zoningStatus).toBeHidden();
  await zoning.uncheck();
  await expect(zoningLegend).toBeHidden();
  await page.getByRole('button', { name: 'Show selected filters', exact: true }).click();
  await expect(filtersTab).toHaveAttribute('aria-selected', 'true');
  await expect(businessName).toBeChecked();
  await page.screenshot({ path: testInfo.outputPath('04-filter-preserved.png') });
  await layersTab.click();
  await expect(roads).toBeChecked();
  await page.getByRole('button', { name: 'Show all counties', exact: true }).click();
  await expect(layers).toHaveCount(0);
  await page.getByRole('button', { name: 'Locate selected county', exact: true }).click();
  await expect(roads).toBeChecked();
  await expect(zoning).not.toBeChecked();
  await expect(roadsStatus).toBeVisible();
  await roads.uncheck();
  await expect(roadsStatus).toBeHidden();
});
