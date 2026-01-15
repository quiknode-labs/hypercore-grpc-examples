const grpc = require('@grpc/grpc-js');
const protoLoader = require('@grpc/proto-loader');
const path = require('path');
const zstd = require('@mongodb-js/zstd');

// Configuration
const GRPC_ENDPOINT = 'your-endpoint.hype-mainnet.quiknode.pro:10000';
const AUTH_TOKEN = 'your-auth-token';
const PROTO_PATH = path.join(__dirname, '..', 'proto', 'hyperliquid.proto');

// Load proto
const packageDefinition = protoLoader.loadSync(PROTO_PATH, {
  keepCase: true,
  longs: String,
  enums: String,
  defaults: true,
  oneofs: true
});
const proto = grpc.loadPackageDefinition(packageDefinition).hyperliquid;

// Decompress zstd data
async function decompress(data) {
  if (!Buffer.isBuffer(data) || data.length < 4) return data;

  // Check zstd magic number: 0x28 0xB5 0x2F 0xFD
  if (data[0] === 0x28 && data[1] === 0xB5 && data[2] === 0x2F && data[3] === 0xFD) {
    const decompressed = await zstd.decompress(data);
    return decompressed.toString('utf8');
  }
  return data.toString('utf8');
}

// Create gRPC client
function createClient() {
  const credentials = grpc.credentials.createSsl();
  const options = {
    'grpc.max_receive_message_length': 100 * 1024 * 1024,
    'grpc.keepalive_time_ms': 30000,
    'grpc.keepalive_timeout_ms': 10000,
  };
  return new proto.Streaming(GRPC_ENDPOINT, credentials, options);
}

// Stream with optional filters
async function streamData(streamType, filters = {}) {
  const client = createClient();
  const metadata = new grpc.Metadata();
  metadata.add('x-token', AUTH_TOKEN);

  const call = client.StreamData(metadata);

  // Build subscription request
  const subscribeRequest = {
    subscribe: {
      stream_type: streamType,
      start_block: 0
    }
  };

  // Add filters if provided
  if (Object.keys(filters).length > 0) {
    subscribeRequest.subscribe.filters = {};
    for (const [field, values] of Object.entries(filters)) {
      subscribeRequest.subscribe.filters[field] = { values: Array.isArray(values) ? values : [values] };
    }
    console.log(`Filters applied: ${JSON.stringify(filters)}`);
  }

  call.write(subscribeRequest);
  console.log(`Streaming ${streamType}...`);

  call.on('data', async (response) => {
    if (response.data) {
      const { block_number, timestamp, data } = response.data;
      const decompressed = await decompress(data);

      try {
        const parsed = JSON.parse(decompressed);
        console.log(`\nBlock ${block_number} | Timestamp ${timestamp}`);
        console.log(JSON.stringify(parsed, null, 2));
      } catch {
        console.log(`Block ${block_number}: ${decompressed}`);
      }
    } else if (response.pong) {
      console.log(`Pong: ${response.pong.timestamp}`);
    }
  });

  call.on('error', (err) => console.error('Error:', err.message));
  call.on('end', () => console.log('Stream ended'));

  // Keep-alive ping every 30s
  setInterval(() => {
    call.write({ ping: { timestamp: Date.now() } });
  }, 30000);
}

// Main
const streamType = process.argv[2] || 'TRADES';
const filters = {};

// Parse --filter args: --filter coin=ETH,BTC
process.argv.forEach((arg, i) => {
  if (arg === '--filter' && process.argv[i + 1]) {
    const [field, values] = process.argv[i + 1].split('=');
    if (field && values) {
      filters[field] = values.split(',');
    }
  }
});

streamData(streamType, filters);
