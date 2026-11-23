import { test } from 'node:test';
import { deepStrictEqual, throws } from 'node:assert/strict';
import { parseDetailFields } from './detailGroups';

test('server groups use canonical order and preserve intentionally empty selection', () => {
  deepStrictEqual(parseDetailFields({ fields: ['soil_septic', 'mapped_acres'] }), ['mapped_acres', 'soil_septic']);
  deepStrictEqual(parseDetailFields({ fields: [] }), []);
});
test('reject unexpected or duplicate preference groups', () => {
  for (const value of [{}, { fields: null }, { fields: ['secret'] }, { fields: ['soil_septic', 'soil_septic'] }])
    throws(() => parseDetailFields(value));
});
