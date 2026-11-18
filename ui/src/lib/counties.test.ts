import { test } from 'node:test';
import assert from 'node:assert/strict';
import { counties, countyBounds, countyGeometry, countyStatus, mapDetail } from './counties';

test('coverage contains exactly the nine requested counties with only Union configured', () => {
  assert.deepEqual(counties.map((county) => `${county.name}, ${county.state}`).sort(), [
    'Anson, NC', 'Cabarrus, NC', 'Chesterfield, SC', 'Gaston, NC', 'Lancaster, SC', 'Mecklenburg, NC', 'Stanly, NC', 'Union, NC', 'York, SC'
  ]);
  assert.deepEqual(counties.filter((county) => county.status === 'configured').map((county) => county.id), ['37179']);
  assert.equal(counties.filter((county) => countyStatus(county).startsWith('Locked')).length, 8);
});

test('each county has one closed boundary with its label inside its display bounds', () => {
  assert.equal(countyGeometry.features.length, 9);
  assert.deepEqual(countyGeometry.features.map((feature) => feature.id).sort(), counties.map((county) => county.id).sort());
  for (const county of counties) {
    const feature = countyGeometry.features.find((feature) => feature.id === county.id)!;
    assert.equal(feature.properties?.status, county.status);
    for (const ring of feature.geometry.coordinates) {
      assert.ok(ring.length >= 4);
      assert.deepEqual(ring[0], ring.at(-1));
      assert.ok(ring.every(([x, y]) => Number.isFinite(x) && Number.isFinite(y) && x >= -82 && x <= -79 && y >= 34 && y <= 36));
    }
    const [[west, south], [east, north]] = countyBounds(county.id);
    assert.ok(county.center[0] >= west && county.center[0] <= east);
    assert.ok(county.center[1] >= south && county.center[1] <= north);
  }
});

test('zoom changes display detail at county and parcel thresholds', () => {
  assert.equal(mapDetail(0), 'region');
  assert.equal(mapDetail(8.99), 'region');
  assert.equal(mapDetail(9), 'county');
  assert.equal(mapDetail(12.99), 'county');
  assert.equal(mapDetail(13), 'parcels');
  assert.equal(mapDetail(20), 'parcels');
});

test('regional bounds include every county', () => {
  const [[west, south], [east, north]] = countyBounds();
  for (const county of counties) {
    const [[w, s], [e, n]] = countyBounds(county.id);
    assert.ok(w >= west && s >= south && e <= east && n <= north);
  }
});
