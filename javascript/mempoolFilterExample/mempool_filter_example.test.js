const assert = require('node:assert/strict');
const test = require('node:test');
const zstd = require('@mongodb-js/zstd');

const {
  decompress,
  orderTouchingActions,
  orderTouchingAssetIds,
  pingRequest,
  signedActions
} = require('./mempool_filter_example');

test('builds an application-level ping request', () => {
  assert.deepEqual(pingRequest(123456789), { ping: { timestamp: 123456789 } });
  assert.throws(() => pingRequest(-1), /non-negative safe integer/);
});

test('decompresses zstd payloads represented by protobuf as latin-1 strings', async () => {
  const json = JSON.stringify(fixture());
  const compressed = await zstd.compress(Buffer.from(json));
  const protobufString = compressed.toString('latin1');

  assert.equal(await decompress(protobufString), json);
  assert.equal(await decompress(json), json);
});

function fixture(root = 'tuple') {
  const tx = {
    tx_hash: '0xraw',
    signed_actions: [
      { action: { type: 'order', orders: [{ a: 0 }] } },
      { action: { type: 'cancel', cancels: [{ a: '5' }] } },
      { action: { type: 'cancelByCloid', cancels: [{ asset: 0 }] } },
      { action: { type: 'batchModify', modifies: [{ order: { a: '0' } }] } },
      { action: { type: 'modify', order: { asset: 0 } } },
      { action: { type: 'twapOrder', twap: { a: 0 } } },
      { action: { type: 'twapCancel', asset: 0 } },
      { action: { type: 'noop' } }
    ]
  };
  return root === 'tuple' ? ['2026-07-17T00:00:00Z', tx] : tx;
}

test('extracts assets from every order-touching action without changing the raw tuple', () => {
  const raw = fixture();
  const before = JSON.stringify(raw);
  const actions = orderTouchingActions(raw);

  assert.deepEqual(actions.map(action => action.type), [
    'order',
    'cancel',
    'cancelByCloid',
    'batchModify',
    'modify',
    'twapOrder',
    'twapCancel'
  ]);
  assert.deepEqual([...new Set(orderTouchingAssetIds(raw))].sort(), ['0', '5']);
  assert.equal(JSON.stringify(raw), before);
});

test('supports object-root mempool payloads', () => {
  const raw = fixture('object');
  assert.equal(signedActions(raw).length, 8);
  assert.ok(orderTouchingAssetIds(raw).includes('0'));
});

test('ignores non-order actions and invalid asset values', () => {
  const raw = {
    signed_actions: [
      { action: { type: 'order', orders: [{ a: -1 }, { a: 'BTC' }] } },
      { action: { type: 'noop', a: 0 } }
    ]
  };
  assert.deepEqual(orderTouchingActions(raw), []);
});
