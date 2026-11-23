import { test } from 'node:test';
import { deepStrictEqual, throws } from 'node:assert/strict';
import { parseDetailGroups } from './detailGroups';

test('server groups use canonical order and preserve intentionally empty selection', () => {
  deepStrictEqual(parseDetailGroups({ groups: ['soil', 'acreage'] }), ['acreage', 'soil']);
  deepStrictEqual(parseDetailGroups({ groups: [] }), []);
});
test('reject unexpected or duplicate preference groups', () => {
  for (const value of [{}, { groups: null }, { groups: ['secret'] }, { groups: ['soil', 'soil'] }])
    throws(() => parseDetailGroups(value));
});
