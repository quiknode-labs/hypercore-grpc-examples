const assert = require('node:assert/strict');
const test = require('node:test');

const { channelOptions, l4SnapshotResetKind } = require('./orderbook_stream_example');

test('allocates enough receive capacity for full BTC L4 snapshots', () => {
  const options = channelOptions();
  assert.equal(options['grpc.max_receive_message_length'], 100 * 1024 * 1024);
  assert.equal(options['grpc-node.flow_control_window'], 32 * 1024 * 1024);
  assert.equal(options['grpc.keepalive_time_ms'], 30000);
  assert.equal(options['grpc.keepalive_timeout_ms'], 10000);
});

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
