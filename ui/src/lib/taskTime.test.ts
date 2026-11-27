import { test } from 'node:test';
import { strict as assert } from 'node:assert';
import { dueCandidates, smsCapacity } from './taskTime';
test('DST skipped and ambiguous hours require a real offset',()=>{
 assert.equal(dueCandidates('2027-03-14T02:30','America/New_York').length,0);
 const offsets=dueCandidates('2027-11-07T01:30','America/New_York');
 assert.deepEqual(offsets.map(o=>o.label),['UTC-05:00','UTC-04:00']);
 assert.notEqual(Date.parse(offsets[0].due_at),Date.parse(offsets[1].due_at));
 assert.equal(dueCandidates('2027-02-09T14:00','America/New_York').length,1);
});
test('single segment limits use GSM septets and UTF-16 units',()=>{
 assert.equal(smsCapacity('^'.repeat(80)).remaining,0);
 assert.equal(smsCapacity('^'.repeat(81)).remaining,-2);
 assert.equal(smsCapacity('😀'.repeat(35)).remaining,0);
 assert.equal(smsCapacity('😀'.repeat(36)).remaining,-2);
 assert.equal(smsCapacity('A^').remaining,157);
 assert.equal(smsCapacity('A😀').remaining,67);
});
