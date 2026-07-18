// Watch live priority-fee mempool events from a QuickNode Hyperliquid gRPC endpoint.
const grpc = require('@grpc/grpc-js');
const protoLoader = require('@grpc/proto-loader');
const zstd = require('@mongodb-js/zstd');
const path = require('path');

const DEFAULT_GRPC_ENDPOINT = 'your-endpoint.hype-mainnet.quiknode.pro:10000';
const DEFAULT_AUTH_TOKEN = 'YOUR_QUICKNODE_TOKEN';
const GRPC_ENDPOINT = process.env.GRPC_ENDPOINT || DEFAULT_GRPC_ENDPOINT;
const AUTH_TOKEN = process.env.AUTH_TOKEN || process.env.QN_AUTH_TOKEN || DEFAULT_AUTH_TOKEN;
const PROTO_PATH = path.join(__dirname, '..', '..', 'proto', 'hyperliquid.proto');

const packageDefinition = protoLoader.loadSync(PROTO_PATH, {
  keepCase: true,
  longs: String,
  enums: String,
  defaults: true,
  oneofs: true
});
const proto = grpc.loadPackageDefinition(packageDefinition).hyperliquid;

function parseArgs() {
  const args = process.argv.slice(2);
  const get = (name) => {
    const arg = args.find(a => a.startsWith(`--${name}=`));
    return arg ? arg.split('=').slice(1).join('=') : null;
  };

  const contains = [];
  const serverFilters = {};
  args.forEach((arg, i) => {
    if (arg === '--contains' && args[i + 1]) contains.push(args[i + 1]);
    if (arg.startsWith('--contains=')) contains.push(arg.split('=').slice(1).join('='));
    if (arg === '--filter' && args[i + 1]) addServerFilter(serverFilters, args[i + 1]);
    if (arg.startsWith('--filter=')) addServerFilter(serverFilters, arg.split('=').slice(1).join('='));
  });

  return {
    startBlock: parseInt(get('start-block') || '0', 10),
    contains,
    serverFilters,
    includeConfirmed: args.includes('--include-confirmed'),
    rawMempool: args.includes('--raw-mempool'),
    allMempool: args.includes('--all-mempool'),
    compact: args.includes('--compact'),
    maxMessages: get('max-messages') ? parseInt(get('max-messages'), 10) : null
  };
}

function addServerFilter(filters, expression) {
  const index = expression.indexOf('=');
  if (index <= 0 || index === expression.length - 1) {
    throw new Error(`Invalid --filter ${expression}; expected field=value1,value2`);
  }

  const field = expression.slice(0, index).trim();
  const values = expression
    .slice(index + 1)
    .split(',')
    .map(value => value.trim())
    .filter(Boolean);
  if (!field || values.length === 0) {
    throw new Error(`Invalid --filter ${expression}; expected field=value1,value2`);
  }
  filters[field] = { values };
}

async function decompress(data) {
  if (typeof data === 'string') {
    if (
      data.length >= 4 &&
      data.charCodeAt(0) === 0x28 &&
      data.charCodeAt(1) === 0xB5 &&
      data.charCodeAt(2) === 0x2F &&
      data.charCodeAt(3) === 0xFD
    ) {
      const out = await zstd.decompress(Buffer.from(data, 'latin1'));
      return out.toString('utf8');
    }
    return data;
  }
  if (!Buffer.isBuffer(data)) return String(data);
  if (data.length >= 4 && data[0] === 0x28 && data[1] === 0xB5 && data[2] === 0x2F && data[3] === 0xFD) {
    const out = await zstd.decompress(data);
    return out.toString('utf8');
  }
  return data.toString('utf8');
}

function priorityFees(value) {
  const fees = [];
  if (Array.isArray(value)) {
    value.forEach(item => fees.push(...priorityFees(item)));
  } else if (value && typeof value === 'object') {
    if (value.source && value.type === 'order' && value.p !== undefined) {
      fees.push(String(value.p));
    }
    if (value.grouping && typeof value.grouping === 'object' && value.grouping.p !== undefined) {
      fees.push(String(value.grouping.p));
    }
    Object.values(value).forEach(item => fees.push(...priorityFees(item)));
  }
  return fees;
}

function matchesTextFilters(text, contains) {
  return contains.length === 0 || contains.some(needle => text.includes(needle));
}

function createClient() {
  return new proto.Streaming(
    GRPC_ENDPOINT,
    grpc.credentials.createSsl(),
    {
      'grpc.max_receive_message_length': 100 * 1024 * 1024,
      'grpc.keepalive_time_ms': 30000,
      'grpc.keepalive_timeout_ms': 10000
    }
  );
}

function main() {
  const args = parseArgs();
  const rawMempool = args.rawMempool || args.allMempool;

  if (GRPC_ENDPOINT === DEFAULT_GRPC_ENDPOINT) {
    console.error('Set GRPC_ENDPOINT to your QuickNode Hyperliquid mainnet or testnet gRPC endpoint.');
    process.exit(2);
  }
  if (AUTH_TOKEN === DEFAULT_AUTH_TOKEN) {
    console.error('Set AUTH_TOKEN to your QuickNode token.');
    process.exit(2);
  }

  if (rawMempool) {
    console.log('Watching raw MEMPOOL_TXS');
  } else if (args.includeConfirmed) {
    console.log('Watching ORDER_PRIORITY events from mempool_txs and replica_cmds');
  } else {
    console.log('Watching pre-consensus ORDER_PRIORITY mempool events');
  }
  console.log(`Endpoint: ${GRPC_ENDPOINT}`);
  if (!rawMempool && !args.includeConfirmed) {
    console.log('Server filter: source=mempool_txs (not finalized)');
  } else if (rawMempool && !args.allMempool) {
    console.log('Local filter: priority grouping only');
  }
  if (Object.keys(args.serverFilters).length) {
    console.log(`Server filters: ${JSON.stringify(args.serverFilters)}`);
  }
  if (args.contains.length) console.log(`Text filters: ${JSON.stringify(args.contains)}`);

  const client = createClient();
  const metadata = new grpc.Metadata();
  metadata.add('x-token', AUTH_TOKEN);
  const call = client.StreamData(metadata);
  let printed = 0;
  let stopping = false;
  let processing = Promise.resolve();
  const filters = !rawMempool && !args.includeConfirmed
    ? { source: { values: ['mempool_txs'] }, ...args.serverFilters }
    : args.serverFilters;

  call.write({
    subscribe: {
      stream_type: rawMempool ? 'MEMPOOL_TXS' : 'ORDER_PRIORITY',
      start_block: args.startBlock,
      filters
    }
  });

  const ping = setInterval(() => {
    call.write({ ping: { timestamp: Date.now() } });
  }, 30000);

  async function handleData(response) {
    if (stopping || !response.data) return;

    let text;
    try {
      text = await decompress(response.data.data);
    } catch (err) {
      console.warn(`Decompress error at block ${response.data.block_number}: ${err.message}`);
      return;
    }
    if (!matchesTextFilters(text, args.contains)) return;

    let parsed = null;
    try {
      parsed = JSON.parse(text);
    } catch {}

    const fees = parsed ? priorityFees(parsed) : [];
    if (rawMempool && !args.allMempool && fees.length === 0) return;
    if (args.maxMessages && printed >= args.maxMessages) return;

    printed += 1;
    console.log(`\nBlock ${response.data.block_number} | Timestamp ${response.data.timestamp}`);
    if (fees.length) console.log(`Priority p: ${fees.join(', ')}`);
    if (args.compact) {
      console.log(text.slice(0, 1000));
    } else if (parsed) {
      console.log(JSON.stringify(parsed, null, 2));
    } else {
      console.log(text);
    }

    if (args.maxMessages && printed >= args.maxMessages) {
      stopping = true;
      clearInterval(ping);
      call.end();
    }
  }

  call.on('data', (response) => {
    processing = processing
      .then(() => handleData(response))
      .catch((err) => {
        console.error(`Handler error: ${err.message}`);
      });
  });

  call.on('error', (err) => {
    clearInterval(ping);
    console.error(`gRPC error: ${err.message}`);
    process.exit(1);
  });

  call.on('end', () => {
    clearInterval(ping);
  });
}

main();
