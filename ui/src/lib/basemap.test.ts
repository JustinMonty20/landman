import { strict as assert } from 'node:assert';
import { test } from 'node:test';
import { basemapMask } from './basemap';
import { counties } from './counties';

function insideRing([x, y]: readonly number[], ring: number[][]): boolean {
  let inside = false;
  for (let i = 0, j = ring.length - 1; i < ring.length; j = i++) {
    const [ax, ay] = ring[i];
    const [bx, by] = ring[j];
    if ((ay > y) !== (by > y) && x < ax + (bx - ax) * (y - ay) / (by - ay)) inside = !inside;
  }
  return inside;
}

test('the basemap mask leaves all nine mapped counties open', () => {
  const mask = basemapMask().geometry.coordinates;
  const covered = (point: readonly number[]) => mask.some(([shell, ...holes]) =>
    insideRing(point, shell) && !holes.some(hole => insideRing(point, hole)));
  for (const county of counties) assert.equal(covered(county.center), false, county.name);
  for (const point of [[-80.32, 35.75], [-80.24, 34.1], [0, 0], [179, 80]]) {
    assert.equal(covered(point), true, point.join(', '));
  }
});
