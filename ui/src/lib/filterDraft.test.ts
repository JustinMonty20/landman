import { test } from 'node:test';
import assert from 'node:assert/strict';
import { emptyFilterDraft, filterSelections, removeDraftFilter } from './filterDraft';

test('a new filter preview has no selections', () => {
  assert.deepEqual(filterSelections(emptyFilterDraft()), []);
});

test('preview selections keep owner, tax, and parcel fields in their groups', () => {
  const filters = { ...emptyFilterDraft(), businessName: true, outOfState: true, delinquent: true, foreclosure: true, vacancyTier: 'unknown' as const, minAcres: 0, maxAcres: 20 };
  const selections = filterSelections(filters);
  assert.equal(selections.length, 7);
  assert.deepEqual(selections.filter((selection) => selection.group === 'Owner').map((selection) => selection.key), ['businessName', 'outOfState']);
  assert.deepEqual(selections.filter((selection) => selection.group === 'Tax').map((selection) => selection.key), ['delinquent', 'foreclosure']);
  assert.deepEqual(selections.filter((selection) => selection.group === 'Parcel').map((selection) => selection.key), ['minAcres', 'maxAcres', 'vacancyTier']);
  assert.equal(selections.find((selection) => selection.key === 'minAcres')?.label, 'At least 0 acres');
});

test('removing a preview selection preserves all other fields', () => {
  const original = { ...emptyFilterDraft(), businessName: true, outOfState: true, minAcres: 0, maxAcres: 20 };
  const ownerRemoved = removeDraftFilter(original, 'businessName');
  assert.equal(ownerRemoved.businessName, false);
  assert.equal(ownerRemoved.outOfState, true);
  assert.equal(original.businessName, true);
  const acreageRemoved = removeDraftFilter(ownerRemoved, 'minAcres');
  assert.equal(acreageRemoved.minAcres, undefined);
  assert.equal(acreageRemoved.maxAcres, 20);
  assert.equal(filterSelections(acreageRemoved).length, 2);
});
