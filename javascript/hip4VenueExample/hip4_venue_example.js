// HIP-4 Outcome Markets Example - Stream one venue's orders with
// subscription tagging and signer enrichment.
//
// Demonstrates the three HIP-4-era streaming features:
//  1. "venue" filter key: server-side expansion of an outcome venue's name
//     to its full coin set (coins look like "#146870" and churn as outcomes
//     settle - the server tracks that for you).
//  2. subscription_id: a client-chosen tag echoed on every update, plus the
//     stream_type field, so multiplexed subscriptions are distinguishable.
//  3. enrichment.include_signer: each order carries "signer" - the wallet
//     that actually SUBMITTED it (master or approved API wallet), recovered
//     from the action's signature. Unsigned engine events (trigger fires,
//     liquidations, TWAP children) carry "signer": null.
//
// Find active venue names via the info endpoint: {"type":"outcomeMeta"}
// (fields: outcomes[].venue, deployers[].venue).
const grpc = require('@grpc/grpc-js');
const protoLoader = require('@grpc/proto-loader');
const path = require('path');
const zstd = require('@mongodb-js/zstd');

// HIP-4 launches on testnet first; use your testnet endpoint until mainnet
// venues go live.
// Mainnet: 'your-endpoint.hype-mainnet.quiknode.pro:10000'
// Testnet: 'your-endpoint.hype-testnet.quiknode.pro:10000'
const GRPC_ENDPOINT = 'your-endpoint.hype-testnet.quiknode.pro:10000';
const AUTH_TOKEN = 'your-auth-token';
const VENUE_NAME = 'txyz'; // an active venue from {"type":"outcomeMeta"}
const PROTO_PATH = path.join(__dirname, '..', '..', 'proto', 'hyperliquid.proto');

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

async function streamVenueOrders() {
  const client = createClient();
  const metadata = new grpc.Metadata();
  metadata.add('x-token', AUTH_TOKEN);

  const call = client.StreamData(metadata);

  // Subscribe to ORDERS for one outcome venue, tagged and signer-enriched.
  call.write({
    subscribe: {
      stream_type: 'ORDERS',
      start_block: 0,
      filters: {
        // Reserved key: expanded server-side to the venue's coin set.
        // Also accepted: "venues", "deployer", "deployers" (address).
        venue: { values: [VENUE_NAME] }
      },
      filter_name: `hip4-${VENUE_NAME}`,
      // Echoed on every update for this stream type.
      subscription_id: 'hip4-orders-demo',
      // Adds "signer" to each order (requires a server with signer
      // enrichment enabled; testnet has it on).
      enrichment: { include_signer: true }
    }
  });

  console.log(`Streaming ORDERS for venue "${VENUE_NAME}" with signer enrichment\n`);

  call.on('data', async (response) => {
    if (!response.data) return; // pong
    const decompressed = await decompress(response.data.data);

    // Every update says which subscription it belongs to.
    console.log(
      `[block ${response.data.block_number}] ` +
      `streamType=${response.data.stream_type} ` +
      `subscriptionId="${response.data.subscription_id}"`
    );

    try {
      const entries = JSON.parse(decompressed);
      for (const entry of entries) {
        const coin = entry.order?.order?.coin;
        const user = entry.order?.user;
        // "signer" is present because of enrichment above.
        console.log(
          `  coin=${coin} user=${user} signer=${entry.signer} status=${entry.status}`
        );
      }
    } catch {
      console.log(decompressed);
    }
  });

  call.on('error', (err) => console.error('Error:', err.message));

  // Keep-alive
  setInterval(() => call.write({ ping: { timestamp: Date.now() } }), 30000);
}

streamVenueOrders();
