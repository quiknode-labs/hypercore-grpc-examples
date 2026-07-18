const assert = require('node:assert/strict');
const test = require('node:test');

const { addServerFilter } = require('./watch_priority_mempool');

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
