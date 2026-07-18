// Orderbook Stream Example - Stream Hyperliquid orderbook data via QuickNode gRPC
const grpc = require('@grpc/grpc-js');
const protoLoader = require('@grpc/proto-loader');
const path = require('path');

// Mainnet: your-endpoint.hype-mainnet.quiknode.pro:10000
// Testnet: your-endpoint.hype-testnet.quiknode.pro:10000
const GRPC_ENDPOINT = process.env.GRPC_ENDPOINT || 'your-endpoint.hype-mainnet.quiknode.pro:10000';
const AUTH_TOKEN = process.env.AUTH_TOKEN || process.env.QN_AUTH_TOKEN || 'your-quicknode-token';
const GRPC_PLAINTEXT = process.env.GRPC_PLAINTEXT === '1';
const PROTO_PATH = path.join(__dirname, '..', '..', 'proto', 'orderbook.proto');
const MAX_RECEIVE_BYTES = 100 * 1024 * 1024;
const L4_FLOW_CONTROL_BYTES = 32 * 1024 * 1024;

const packageDefinition = protoLoader.loadSync(PROTO_PATH, {
  keepCase: true,
  longs: String,
  enums: String,
  defaults: true,
  oneofs: true
});
const proto = grpc.loadPackageDefinition(packageDefinition).hyperliquid;

function channelOptions() {
  return {
    'grpc.max_receive_message_length': MAX_RECEIVE_BYTES,
    // BTC L4 snapshots contain tens of thousands of orders. A larger HTTP/2
    // receive window prevents the server from timing out while Node decodes them.
    'grpc-node.flow_control_window': L4_FLOW_CONTROL_BYTES,
    'grpc.keepalive_time_ms': 30000,
    'grpc.keepalive_timeout_ms': 10000
  };
}

function createClient() {
  return new proto.OrderBookStreaming(
    GRPC_ENDPOINT,
    GRPC_PLAINTEXT ? grpc.credentials.createInsecure() : grpc.credentials.createSsl(),
    channelOptions()
  );
}

function createMetadata() {
  const metadata = new grpc.Metadata();
  metadata.add('x-token', AUTH_TOKEN);
  return metadata;
}

function parseArgs() {
  const args = process.argv.slice(2);
  const get = (name, fallback = null) => {
    const arg = args.find(a => a.startsWith(`--${name}=`));
    return arg ? arg.split('=').slice(1).join('=') : fallback;
  };

  const coinArg = get('coin', 'BTC');
  const all = args.includes('--all');
  const coins = all ? [] : coinArg.split(',').map(c => c.trim()).filter(Boolean);
  return {
    mode: get('mode', 'bbo'),
    all,
    coins,
    coin: coins[0] || '',
    levels: parseInt(get('levels', '20'), 10),
    sigFigs: get('sig-figs') ? parseInt(get('sig-figs'), 10) : null,
    mantissa: get('mantissa') ? parseInt(get('mantissa'), 10) : null,
    skipInitialSnapshot: args.includes('--skip-initial-snapshot'),
    maxMessages: get('max-messages') ? parseInt(get('max-messages'), 10) : null
  };
}

function validateArgs(args) {
  if (args.all && (args.mode === 'l2' || args.mode === 'l4')) {
    console.error('--all is only supported for bbo, l2-diff, l4-updates, and tpsl. Use --coin for l2 or l4.');
    process.exit(2);
  }
  if (!args.all && args.coins.length === 0) {
    console.error('--coin must include at least one symbol. Use --all to subscribe to every eligible coin on multi-coin streams.');
    process.exit(2);
  }
}

function coinDisplay(args) {
  if (args.mode === 'l2' || args.mode === 'l4') {
    return { label: 'Coin', value: args.coin };
  }
  return {
    label: 'Coins',
    value: args.coins.length ? args.coins.join(',') : 'all eligible coins'
  };
}

function sleep(ms) {
  return new Promise(resolve => setTimeout(resolve, ms));
}

function levelText(level) {
  if (!level || !level.px) return 'n/a';
  return `${level.px} / ${level.sz} (${level.n})`;
}

function l4SnapshotResetKind(snapshotCount) {
  if (!Number.isInteger(snapshotCount) || snapshotCount < 1) {
    throw new Error('snapshotCount must be a positive integer');
  }
  return snapshotCount === 1 ? 'initial' : 'replacement';
}

function l2Request(args) {
  const request = {
    coin: args.coin,
    n_levels: args.levels
  };
  if (args.sigFigs !== null) request.n_sig_figs = args.sigFigs;
  if (args.mantissa !== null) request.mantissa = args.mantissa;
  return request;
}

function l2DiffRequest(args) {
  const request = {
    coins: args.coins,
    n_levels: args.levels,
    skip_initial_snapshot: args.skipInitialSnapshot
  };
  if (args.sigFigs !== null) request.n_sig_figs = args.sigFigs;
  if (args.mantissa !== null) request.mantissa = args.mantissa;
  return request;
}

async function consumeWithReconnect(label, makeCall, onData, maxMessages = null) {
  const maxRetries = 10;
  const baseDelayMs = 2000;
  let msgCount = 0;
  let dataLossCount = 0;

  while (dataLossCount < maxRetries) {
    if (dataLossCount > 0) {
      const delay = baseDelayMs * Math.pow(2, dataLossCount - 1);
      console.log(`Reconnecting ${label} after DATA_LOSS in ${delay / 1000}s (attempt ${dataLossCount + 1}/${maxRetries})`);
      await sleep(delay);
    }

    const result = await new Promise((resolve, reject) => {
      const call = makeCall();

      call.on('data', (update) => {
        dataLossCount = 0;
        msgCount += 1;
        onData(update, msgCount);

        if (maxMessages && msgCount >= maxMessages) {
          call.cancel();
        }
      });

      call.on('error', (err) => {
        if (err.code === grpc.status.CANCELLED && maxMessages && msgCount >= maxMessages) {
          resolve('done');
        } else if (err.code === grpc.status.DATA_LOSS) {
          resolve('data_loss');
        } else {
          reject(err);
        }
      });

      call.on('end', () => resolve('done'));
    });

    if (result === 'done') return;
    dataLossCount += 1;
  }

  throw new Error(`${label} exceeded max reconnect attempts`);
}

async function streamL2(args) {
  await consumeWithReconnect(
    'StreamL2Book',
    () => createClient().StreamL2Book(l2Request(args), createMetadata()),
    (update, count) => {
      const bestBid = update.bids && update.bids.length ? update.bids[0] : null;
      const bestAsk = update.asks && update.asks.length ? update.asks[0] : null;
      console.log(`[${count}] L2 ${update.coin} block=${update.block_number} bid=${levelText(bestBid)} ask=${levelText(bestAsk)} bids=${update.bids.length} asks=${update.asks.length}`);
    },
    args.maxMessages
  );
}

async function streamL4(args) {
  let snapshotCount = 0;
  await consumeWithReconnect(
    'StreamL4Book',
    () => createClient().StreamL4Book({ coin: args.coin }, createMetadata()),
    (update, count) => {
      if (update.snapshot) {
        snapshotCount += 1;
        const reset = l4SnapshotResetKind(snapshotCount);
        console.log(`[${count}] L4 snapshot ${update.snapshot.coin} height=${update.snapshot.height} reset=${reset} bids=${update.snapshot.bids.length} asks=${update.snapshot.asks.length}`);
        if (reset === 'replacement') {
          console.log('  replace the entire local L4 book with this snapshot');
        }
      } else if (update.diff) {
        let data;
        try {
          data = JSON.parse(update.diff.data || '{}');
        } catch (err) {
          console.warn(`[${count}] L4 diff height=${update.diff.height} invalid JSON: ${err.message}`);
          return;
        }
        console.log(`[${count}] L4 diff height=${update.diff.height} order_statuses=${(data.order_statuses || []).length} book_diffs=${(data.book_diffs || []).length}`);
      }
    },
    args.maxMessages
  );
}

async function streamBbo(args) {
  await consumeWithReconnect(
    'StreamBboBook',
    () => createClient().StreamBboBook({ coins: args.coins }, createMetadata()),
    (update, count) => {
      console.log(`[${count}] BBO ${update.coin} block=${update.block_number} bid=${levelText(update.bid)} ask=${levelText(update.ask)}`);
    },
    args.maxMessages
  );
}

async function streamL2Diff(args) {
  await consumeWithReconnect(
    'StreamL2BookDiff',
    () => createClient().StreamL2BookDiff(l2DiffRequest(args), createMetadata()),
    (update, count) => {
      console.log(`[${count}] L2 diff height=${update.height} snapshot=${update.snapshot} coins=${update.diffs.length}`);
      update.diffs.slice(0, 5).forEach(diff => {
        console.log(`  ${diff.coin} seq=${diff.seq} prev_seq=${diff.prev_seq} snapshot=${diff.snapshot} bid_changes=${diff.bids.length} ask_changes=${diff.asks.length}`);
      });
    },
    args.maxMessages
  );
}

async function streamL4Updates(args) {
  await consumeWithReconnect(
    'StreamL4BookUpdates',
    () => createClient().StreamL4BookUpdates({ coins: args.coins }, createMetadata()),
    (update, count) => {
      console.log(`[${count}] L4 updates height=${update.height} snapshot=${update.snapshot} diffs=${update.diffs.length}`);
      if (update.snapshot) console.log('  clear local L4 order state before applying this update');
      update.diffs.slice(0, 5).forEach(diff => {
        console.log(`  ${diff.diff_type} ${diff.coin} oid=${diff.oid} side=${diff.side || 'n/a'} px=${diff.px || 'n/a'} sz=${diff.sz || 'n/a'}`);
      });
    },
    args.maxMessages
  );
}

async function streamTpsl(args) {
  await consumeWithReconnect(
    'StreamTpslUpdates',
    () => createClient().StreamTpslUpdates({ coins: args.coins }, createMetadata()),
    (update, count) => {
      console.log(`[${count}] TP/SL height=${update.height} snapshot=${update.snapshot} diffs=${update.diffs.length}`);
      update.diffs.slice(0, 5).forEach(diff => {
        console.log(`  ${diff.diff_type} ${diff.coin} oid=${diff.oid} trigger=${diff.trigger_px || 'n/a'} limit=${diff.limit_px || 'n/a'} sz=${diff.sz || 'n/a'} reason=${diff.reason || 'n/a'}`);
      });
    },
    args.maxMessages
  );
}

async function main() {
  const args = parseArgs();
  validateArgs(args);

  console.log('Hyperliquid Orderbook Stream Example');
  console.log(`Endpoint: ${GRPC_ENDPOINT}`);
  console.log(`Mode: ${args.mode}`);
  const display = coinDisplay(args);
  console.log(`${display.label}: ${display.value}`);

  if (AUTH_TOKEN === 'your-quicknode-token') {
    console.error('Set AUTH_TOKEN to your QuickNode token before running this example.');
    process.exit(1);
  }

  switch (args.mode) {
    case 'l2':
      return streamL2(args);
    case 'l4':
      return streamL4(args);
    case 'bbo':
      return streamBbo(args);
    case 'l2-diff':
      return streamL2Diff(args);
    case 'l4-updates':
      return streamL4Updates(args);
    case 'tpsl':
      return streamTpsl(args);
    default:
      console.error('Invalid mode. Use --mode=l2, l4, bbo, l2-diff, l4-updates, or tpsl.');
      process.exit(1);
  }
}

if (require.main === module) {
  main().catch(err => {
    console.error('Stream failed:', err.message);
    process.exit(1);
  });
}

module.exports = { channelOptions, l4SnapshotResetKind };
