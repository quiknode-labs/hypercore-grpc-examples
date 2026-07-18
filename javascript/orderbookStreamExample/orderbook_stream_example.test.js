const assert = require('node:assert/strict');
const test = require('node:test');

const { l4SnapshotResetKind } = require('./orderbook_stream_example');

test('classifies the first L4 snapshot as the initial reset', () => {
  assert.equal(l4SnapshotResetKind(1), 'initial');
});

test('classifies every later L4 snapshot as an authoritative replacement', () => {
  assert.equal(l4SnapshotResetKind(2), 'replacement');
  assert.equal(l4SnapshotResetKind(10), 'replacement');
});

test('rejects invalid snapshot counters', () => {
  assert.throws(() => l4SnapshotResetKind(0), /positive integer/);
  assert.throws(() => l4SnapshotResetKind(1.5), /positive integer/);
});
