// Filtering Example - Stream only trades for specific coins
const grpc = require('@grpc/grpc-js');
const protoLoader = require('@grpc/proto-loader');
const path = require('path');
const zstd = require('@mongodb-js/zstd');

const GRPC_ENDPOINT = 'your-endpoint.hype-mainnet.quiknode.pro:10000';
const AUTH_TOKEN = 'your-auth-token';
const PROTO_PATH = path.join(__dirname, '..', 'proto', 'hyperliquid.proto');

const packageDefinition = protoLoader.loadSync(PROTO_PATH, {
  keepCase: true,
  longs: String,
  enums: String,
  defaults: true,
  oneofs: true
});
const proto = grpc.loadPackageDefinition(packageDefinition).hyperliquid;

async function decompress(data) {
  if (!Buffer.isBuffer(data) || data.length < 4) return data;
  if (data[0] === 0x28 && data[1] === 0xB5 && data[2] === 0x2F && data[3] === 0xFD) {
    return (await zstd.decompress(data)).toString('utf8');
  }
  return data.toString('utf8');
}

function createClient() {
  return new proto.Streaming(
    GRPC_ENDPOINT,
    grpc.credentials.createSsl(),
    { 'grpc.max_receive_message_length': 100 * 1024 * 1024 }
  );
}

async function streamWithFilter() {
  const client = createClient();
  const metadata = new grpc.Metadata();
  metadata.add('x-token', AUTH_TOKEN);

  const call = client.StreamData(metadata);

  // Subscribe to TRADES with filters
  call.write({
    subscribe: {
      stream_type: 'TRADES',
      start_block: 0,
      // Filter for specific coins only
      filters: {
        coin: { values: ['ETH', 'BTC'] }
      },
      filter_name: 'eth-btc-trades'
    }
  });

  console.log('Streaming TRADES filtered by coin: ETH, BTC\n');

  call.on('data', async (response) => {
    if (response.data) {
      const decompressed = await decompress(response.data.data);
      try {
        const parsed = JSON.parse(decompressed);
        console.log(`Block ${response.data.block_number}:`);
        console.log(JSON.stringify(parsed, null, 2));
      } catch {
        console.log(decompressed);
      }
    }
  });

  call.on('error', (err) => console.error('Error:', err.message));

  // Keep-alive
  setInterval(() => call.write({ ping: { timestamp: Date.now() } }), 30000);
}

streamWithFilter();
