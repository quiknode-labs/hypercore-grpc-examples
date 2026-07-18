#!/usr/bin/env node

// Filter raw Hyperliquid mempool transactions by coin name.
//
// MEMPOOL_TXS payloads contain numeric asset IDs rather than a top-level coin
// field. The gRPC service resolves `coin=BTC` dynamically and applies it to
// every order-touching action. The original raw tuple/object is returned
// unchanged when any action in the transaction touches the requested coin.

const grpc = require('@grpc/grpc-js');
const protoLoader = require('@grpc/proto-loader');
const zstd = require('@mongodb-js/zstd');
const path = require('path');

const DEFAULT_GRPC_ENDPOINT = 'your-endpoint.hype-mainnet.quiknode.pro:10000';
const DEFAULT_AUTH_TOKEN = 'YOUR_QUICKNODE_TOKEN';
const GRPC_ENDPOINT = process.env.GRPC_ENDPOINT || DEFAULT_GRPC_ENDPOINT;
const AUTH_TOKEN = process.env.AUTH_TOKEN || process.env.QN_AUTH_TOKEN || DEFAULT_AUTH_TOKEN;
const GRPC_PLAINTEXT = process.env.GRPC_PLAINTEXT === '1';
const PROTO_PATH = path.join(__dirname, '..', '..', 'proto', 'hyperliquid.proto');
const ZSTD_MAGIC = Buffer.from([0x28, 0xb5, 0x2f, 0xfd]);

const packageDefinition = protoLoader.loadSync(PROTO_PATH, {
  keepCase: true,
  longs: String,
  enums: String,
  defaults: true,
  oneofs: true
});
const proto = grpc.loadPackageDefinition(packageDefinition).hyperliquid;

function option(name, fallback) {
  const prefix = `--${name}=`;
  const value = process.argv.slice(2).find(arg => arg.startsWith(prefix));
  return value ? value.slice(prefix.length) : fallback;
}

function hasFlag(name) {
  return process.argv.slice(2).includes(`--${name}`);
}

function parseAssetIds(value) {
  const ids = value
    .split(',')
    .map(part => part.trim())
    .filter(Boolean);
  if (!ids.length || ids.some(id => !/^\d+$/.test(id))) {
    throw new Error('--asset-ids must be a comma-separated list of non-negative integers');
  }
  return [...new Set(ids)];
}

function parseValues(value, optionName) {
  const values = value
    .split(',')
    .map(part => part.trim())
    .filter(Boolean);
  if (!values.length) {
    throw new Error(`--${optionName} must contain at least one value`);
  }
  return [...new Set(values)];
}

async function decompress(data) {
  if (typeof data === 'string') {
    const raw = Buffer.from(data, 'latin1');
    if (raw.length >= 4 && raw.subarray(0, 4).equals(ZSTD_MAGIC)) {
      return (await zstd.decompress(raw)).toString('utf8');
    }
    return data;
  }
  if (!Buffer.isBuffer(data)) return String(data);
  if (data.length >= 4 && data.subarray(0, 4).equals(ZSTD_MAGIC)) {
    return (await zstd.decompress(data)).toString('utf8');
  }
  return data.toString('utf8');
}

function signedActions(value) {
  const tx = Array.isArray(value) ? value[1] : value;
  if (!tx || typeof tx !== 'object') return [];
  return Array.isArray(tx.signed_actions) ? tx.signed_actions : [];
}

function assetId(value) {
  if (Number.isInteger(value) && value >= 0) return String(value);
  if (typeof value === 'string' && /^\d+$/.test(value)) return value;
  return null;
}

function addAsset(target, value) {
  const id = assetId(value);
  if (id !== null) target.push(id);
}

function addDirectAssets(target, value) {
  if (!value || typeof value !== 'object') return;
  addAsset(target, value.a);
  addAsset(target, value.asset);
}

function orderTouchingActions(value) {
  const matches = [];
  for (const signedAction of signedActions(value)) {
    const action = signedAction?.action;
    const type = action?.type;
    if (!action || typeof type !== 'string') continue;

    const assets = [];
    if (type === 'order') {
      for (const order of Array.isArray(action.orders) ? action.orders : []) {
        addDirectAssets(assets, order);
      }
    } else if (type === 'cancel' || type === 'cancelByCloid') {
      for (const cancel of Array.isArray(action.cancels) ? action.cancels : []) {
        addDirectAssets(assets, cancel);
      }
    } else if (type === 'batchModify') {
      for (const modify of Array.isArray(action.modifies) ? action.modifies : []) {
        addDirectAssets(assets, modify?.order);
        addDirectAssets(assets, modify);
      }
    } else if (type === 'modify') {
      addDirectAssets(assets, action.order);
      addDirectAssets(assets, action);
    } else if (type === 'twapOrder') {
      addDirectAssets(assets, action.twap);
    } else if (type === 'twapCancel') {
      addDirectAssets(assets, action);
    }

    if (assets.length) {
      matches.push({ type, assetIds: [...new Set(assets)] });
    }
  }
  return matches;
}

function actionTypes(value) {
  return signedActions(value)
    .map(signedAction => signedAction?.action?.type)
    .filter(type => typeof type === 'string' && type.length > 0);
}

function orderTouchingAssetIds(value) {
  return orderTouchingActions(value).flatMap(action => action.assetIds);
}

function normalizedSummary(value) {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return '';
  const fields = ['source', 'type', 'coin', 'asset_id', 'tif', 'side', 'vault', 'user'];
  return fields
    .filter(field => value[field] !== undefined && value[field] !== null)
    .map(field => `${field}=${value[field]}`)
    .join(' ');
}

async function main() {
  if (GRPC_ENDPOINT === DEFAULT_GRPC_ENDPOINT || AUTH_TOKEN === DEFAULT_AUTH_TOKEN) {
    throw new Error('Set GRPC_ENDPOINT and AUTH_TOKEN (or QN_AUTH_TOKEN) before running');
  }

  const expectedAssetIds = parseAssetIds(option('expected-asset-ids', option('asset-ids', '0')));
  const filterField = option('filter-field', 'coin');
  const filterValues = parseValues(option('filter-values', 'BTC'), 'filter-values');
  const maxMessages = Number.parseInt(option('max-messages', '5'), 10);
  const timeoutSeconds = Number.parseInt(option('timeout-seconds', '60'), 10);
  const streamType = option('stream-type', 'MEMPOOL_TXS');
  if (!Number.isInteger(maxMessages) || maxMessages <= 0) {
    throw new Error('--max-messages must be a positive integer');
  }
  if (!Number.isInteger(timeoutSeconds) || timeoutSeconds <= 0) {
    throw new Error('--timeout-seconds must be a positive integer');
  }

  const unfiltered = hasFlag('unfiltered');
  const expectNoMatch = hasFlag('expect-no-match');
  const printRaw = hasFlag('print-raw');
  if (unfiltered && expectNoMatch) {
    throw new Error('--unfiltered and --expect-no-match cannot be used together');
  }

  const client = new proto.Streaming(
    GRPC_ENDPOINT,
    GRPC_PLAINTEXT ? grpc.credentials.createInsecure() : grpc.credentials.createSsl(),
    {
      'grpc.max_receive_message_length': 100 * 1024 * 1024,
      'grpc.keepalive_time_ms': 30000,
      'grpc.keepalive_timeout_ms': 10000
    }
  );
  const metadata = new grpc.Metadata();
  metadata.add('x-token', AUTH_TOKEN);
  const call = client.StreamData(metadata);
  const expectedAssets = new Set(expectedAssetIds);
  let received = 0;
  let finished = false;
  let processing = Promise.resolve();
  let pingTimer;
  let timeoutTimer;

  function finish(exitCode, message) {
    if (finished) return;
    finished = true;
    clearInterval(pingTimer);
    clearTimeout(timeoutTimer);
    call.end();
    client.close();
    if (message) console.log(message);
    process.exitCode = exitCode;
  }

  const subscribe = {
    stream_type: streamType,
    start_block: 0,
    filter_name: unfiltered ? 'mempool-unfiltered-sample' : 'mempool-coin-filter'
  };
  if (!unfiltered) {
    subscribe.filters = {
      [filterField]: { values: filterValues }
    };
  }

  call.write({
    subscribe: {
      ...subscribe
    }
  });

  if (unfiltered) {
    console.log(`Sampling unfiltered ${streamType}`);
  } else {
    console.log(`Filtering ${streamType} by ${filterField} in [${filterValues.join(', ')}]`);
  }
  console.log(`Endpoint: ${GRPC_ENDPOINT}`);

  call.on('data', response => {
    if (finished || !response.data) return;
    const payload = response.data.data;
    processing = processing.then(async () => {
      if (finished) return;
      const text = await decompress(payload);
      if (finished) return;
      const value = JSON.parse(text);
      const observed = orderTouchingAssetIds(value);
      const touchingActions = orderTouchingActions(value);
      const types = actionTypes(value);
      const summary = normalizedSummary(value);
      const actionSummary = touchingActions
        .map(action => `${action.type}:${action.assetIds.join('|')}`)
        .join(', ');

      if (printRaw) console.log(text);

      if (unfiltered) {
        received += 1;
        console.log(
          `sample ${received}/${maxMessages}: ` +
          `action_types=[${[...new Set(types)].join(', ')}] ` +
          `order_touching=[${actionSummary}] ` +
          `all_order_assets=[${[...new Set(observed)].join(', ')}] ` +
          `${summary ? `${summary} ` : ''}` +
          `bytes=${Buffer.byteLength(text)}`
        );
        if (received >= maxMessages) {
          finish(0, `PASS: received ${received} unfiltered mempool message(s)`);
        }
        return;
      }

      if (expectNoMatch) {
        finish(1, `FAILED: expected no matches, but server returned action_types=[${types.join(', ')}] observed=[${observed.join(', ')}]`);
        return;
      }

      const matches = observed.filter(id => expectedAssets.has(id));
      if (!matches.length) {
        finish(1, `FAILED: server returned a raw transaction without an expected asset; action_types=[${types.join(', ')}] observed=[${observed.join(', ')}]`);
        return;
      }

      received += 1;
      console.log(
        `match ${received}/${maxMessages}: ` +
        `expected_asset_matches=[${[...new Set(matches)].join(', ')}] ` +
        `action_types=[${[...new Set(types)].join(', ')}] ` +
        `order_touching=[${actionSummary}] ` +
        `all_order_assets=[${[...new Set(observed)].join(', ')}] ` +
        `${summary ? `${summary} ` : ''}` +
        `bytes=${Buffer.byteLength(text)}`
      );
      if (received >= maxMessages) {
        finish(0, `PASS: received ${received} server-filtered mempool message(s)`);
      }
    }).catch(error => finish(1, `FAILED: ${error.message}`));
  });

  call.on('error', error => {
    if (finished && error.code === grpc.status.CANCELLED) return;
    finish(1, `FAILED: gRPC ${error.code}: ${error.details || error.message}`);
  });

  pingTimer = setInterval(() => {
    if (!finished) call.write({ ping: { timestamp: Date.now() } });
  }, 30000);
  timeoutTimer = setTimeout(() => {
    if (expectNoMatch) {
      finish(0, `PASS: no ${streamType} messages matched ${filterField} in [${filterValues.join(', ')}] within ${timeoutSeconds}s`);
      return;
    }

    finish(1, `FAILED: no ${maxMessages}-message sample within ${timeoutSeconds}s (received ${received})`);
  }, timeoutSeconds * 1000);
}

if (require.main === module) {
  main().catch(error => {
    console.error(`FAILED: ${error.message}`);
    process.exitCode = 1;
  });
}

module.exports = {
  actionTypes,
  decompress,
  orderTouchingActions,
  orderTouchingAssetIds,
  parseAssetIds,
  signedActions
};
