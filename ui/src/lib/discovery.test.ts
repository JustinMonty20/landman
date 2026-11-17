import { test } from 'node:test';
import assert from 'node:assert/strict';
import { acreageError, emptyFilters, geometry, hasActiveFilters, matches, parcelSearchLabel, parcels, searchParcels } from './discovery';

test('parcel search suggests partial parcel IDs and addresses without case sensitivity', () => {
  assert.equal(searchParcels(parcels, ' demo-001 ').length, 1);
  assert.equal(searchParcels(parcels, 'SAMPLE OAK').length, 6);
  assert.equal(searchParcels(parcels, '').length, 0);
  assert.equal(parcelSearchLabel(parcels[0]), '100 Sample Oak Lane · DEMO-001');
});
test('no filters produce no matches without removing parcel lookup data', () => {
  assert.equal(hasActiveFilters(emptyFilters()), false);
  assert.deepEqual(parcels.filter((parcel) => matches(parcel, emptyFilters())), []);
  assert.equal(searchParcels(parcels, 'DEMO-001').length, 1);
  assert.equal(geometry(parcels).features.length, 24);
});

test('zero acreage is an active filter and clearing it restores zero matches', () => {
  const filters = { ...emptyFilters(), min: '0' };
  assert.equal(hasActiveFilters(filters), true);
  assert.equal(parcels.filter((parcel) => matches(parcel, filters)).length, 24);
  assert.equal(parcels.filter((parcel) => matches(parcel, emptyFilters())).length, 0);
});

test('all selected filters apply together and acreage bounds are inclusive', () => {
  const filters = { ...emptyFilters(), min: '1.2', max: '1.2', vacant: true, outside: true, llc: true };
  const result = parcels.filter((p) => matches(p, filters));
  assert.equal(result.length, 2);
  assert.ok(result.every((p) => p.acres === 1.2 && p.vacancy === 'yes' && p.mailing === 'SC' && p.llc === 'yes'));
});
test('unknown evidence never passes a positive filter', () => {
  const unknown = { ...parcels[0], vacancy: 'unknown' as const, mailing: null, llc: 'unknown' as const };
  for (const key of ['vacant', 'outside', 'llc']) assert.equal(matches(unknown, { ...emptyFilters(), [key]: true }), false);
});
test('invalid acreage is reported and does not produce matches', () => {
  for (const range of [{ min: '9', max: '2' }, { min: '-1', max: '' }, { min: '', max: 'NaN' }]) {
    const filters = { ...emptyFilters(), ...range };
    assert.ok(acreageError(filters));
    assert.equal(matches(parcels[0], filters), false);
  }
});
test('map geometry uses the same stable IDs as the results', () => {
  const result = parcels.filter((p) => matches(p, { ...emptyFilters(), outside: true }));
  assert.deepEqual(geometry(result).features.map((f) => f.properties?.id), result.map((p) => p.id));
});
