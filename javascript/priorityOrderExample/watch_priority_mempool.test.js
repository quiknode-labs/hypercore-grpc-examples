const assert = require('node:assert/strict');
const test = require('node:test');

const { addServerFilter, subscriptionFilters } = require('./watch_priority_mempool');

test('merges and deduplicates repeated values for the same filter field', () => {
  const filters = {};
  addServerFilter(filters, 'coin=BTC,ETH');
  addServerFilter(filters, 'coin=ETH,SOL');

  assert.deepEqual(filters, {
    coin: { values: ['BTC', 'ETH', 'SOL'] }
  });
});

test('keeps independently named filter fields', () => {
  const filters = {};
  addServerFilter(filters, 'coin=BTC');
  addServerFilter(filters, 'source=mempool_txs');

  assert.deepEqual(filters, {
    coin: { values: ['BTC'] },
    source: { values: ['mempool_txs'] }
  });
});

test('enforces source=mempool_txs in default priority mode', () => {
  const serverFilters = { coin: { values: ['BTC'] } };

  assert.deepEqual(subscriptionFilters({
    serverFilters,
    includeConfirmed: false,
    rawMempool: false,
    allMempool: false
  }), {
    coin: { values: ['BTC'] },
    source: { values: ['mempool_txs'] }
  });
  assert.deepEqual(serverFilters, { coin: { values: ['BTC'] } });
});

test('rejects an incompatible source in default priority mode', () => {
  assert.throws(() => subscriptionFilters({
    serverFilters: { source: { values: ['replica_cmds'] } },
    includeConfirmed: false,
    rawMempool: false,
    allMempool: false
  }), /use --include-confirmed/);
});

test('preserves explicit source filters when confirmed events are enabled', () => {
  assert.deepEqual(subscriptionFilters({
    serverFilters: { source: { values: ['replica_cmds'] } },
    includeConfirmed: true,
    rawMempool: false,
    allMempool: false
  }), {
    source: { values: ['replica_cmds'] }
  });
});
