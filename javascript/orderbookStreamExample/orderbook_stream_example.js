// Orderbook Stream Example - Stream L2 and L4 orderbook data via gRPC
const grpc = require('@grpc/grpc-js');
const protoLoader = require('@grpc/proto-loader');
const path = require('path');

const GRPC_ENDPOINT = 'your-endpoint.hype-mainnet.quiknode.pro:10000';
const AUTH_TOKEN = 'your-auth-token';
const PROTO_PATH = path.join(__dirname, '..', '..', 'proto', 'orderbook.proto');

const packageDefinition = protoLoader.loadSync(PROTO_PATH, {
  keepCase: true,
  longs: String,
  enums: String,
  defaults: true,
  oneofs: true
});
const proto = grpc.loadPackageDefinition(packageDefinition).hyperliquid;

function createClient() {
  return new proto.OrderBookStreaming(
    GRPC_ENDPOINT,
    grpc.credentials.createSsl(),
    { 'grpc.max_receive_message_length': 100 * 1024 * 1024 }
  );
}

// Stream L2 (aggregated) orderbook
async function streamL2Orderbook(coin, nLevels = 20, autoReconnect = true) {
  console.log('='.repeat(60));
  console.log(`Streaming L2 Orderbook for ${coin}`);
  console.log(`Levels: ${nLevels}`);
  console.log(`Auto-reconnect: ${autoReconnect}`);
  console.log('='.repeat(60) + '\n');

  let retryCount = 0;
  const maxRetries = 10;
  const baseDelay = 2000;

  while (retryCount < maxRetries) {
    const client = createClient();
    const metadata = new grpc.Metadata();
    metadata.add('x-token', AUTH_TOKEN);

    const request = {
      coin: coin,
      n_levels: nLevels
    };

    try {
      if (retryCount > 0) {
        console.log(`\n🔄 Reconnecting (attempt ${retryCount + 1}/${maxRetries})...`);
      } else {
        console.log(`Connecting to ${GRPC_ENDPOINT}...`);
      }

      let msgCount = 0;
      const call = client.StreamL2Book(request, metadata);

      call.on('data', (update) => {
        msgCount++;

        if (msgCount === 1) {
          console.log('✓ First L2 update received!\n');
          retryCount = 0; // Reset on success
        }

        console.log('\n' + '─'.repeat(60));
        console.log(`Block: ${update.block_number} | Time: ${update.time} | Coin: ${update.coin}`);
        console.log('─'.repeat(60));

        // Display asks (reversed for display)
        if (update.asks && update.asks.length > 0) {
          console.log('\n  ASKS:');
          update.asks.slice(0, 10).reverse().forEach(level => {
            console.log(`    ${level.px.padStart(12)} | ${level.sz.padStart(12)} | (${level.n} orders)`);
          });
        }

        // Display spread
        if (update.bids && update.bids.length > 0 && update.asks && update.asks.length > 0) {
          const bestBid = parseFloat(update.bids[0].px);
          const bestAsk = parseFloat(update.asks[0].px);
          const spread = bestAsk - bestBid;
          const spreadBps = (spread / bestBid) * 10000;
          console.log('\n  ' + '─'.repeat(44));
          console.log(`  SPREAD: ${spread.toFixed(2)} (${spreadBps.toFixed(2)} bps)`);
          console.log('  ' + '─'.repeat(44));
        }

        // Display bids
        if (update.bids && update.bids.length > 0) {
          console.log('\n  BIDS:');
          update.bids.slice(0, 10).forEach(level => {
            console.log(`    ${level.px.padStart(12)} | ${level.sz.padStart(12)} | (${level.n} orders)`);
          });
        }

        console.log(`\n  Messages received: ${msgCount}`);
      });

      call.on('error', (err) => {
        if (err.code === grpc.status.DATA_LOSS && autoReconnect) {
          console.log(`\n⚠️  Server reinitialized: ${err.message}`);
          retryCount++;
          if (retryCount < maxRetries) {
            const delay = baseDelay * Math.pow(2, retryCount - 1);
            console.log(`⏳ Waiting ${delay / 1000}s before reconnecting...`);
            setTimeout(() => streamL2Orderbook(coin, nLevels, autoReconnect), delay);
          } else {
            console.log(`\n❌ Max retries (${maxRetries}) reached. Giving up.`);
          }
        } else {
          console.error('\ngRPC error:', err.code, '-', err.message);
        }
      });

      call.on('end', () => {
        console.log('\nStream ended');
      });

      // Wait for stream to complete
      await new Promise((resolve) => {
        call.on('end', resolve);
        call.on('error', resolve);
      });

      break; // Exit retry loop on success

    } catch (err) {
      console.error('Error:', err.message);
      break;
    }
  }
}

// Stream L4 (individual orders) orderbook
async function streamL4Orderbook(coin, maxMessages = null, autoReconnect = true) {
  console.log('='.repeat(60));
  console.log(`Streaming L4 Orderbook for ${coin}`);
  console.log(`Auto-reconnect: ${autoReconnect}`);
  console.log('='.repeat(60) + '\n');

  let retryCount = 0;
  const maxRetries = 10;
  const baseDelay = 2000;
  let totalMsgCount = 0;

  while (retryCount < maxRetries) {
    const client = createClient();
    const metadata = new grpc.Metadata();
    metadata.add('x-token', AUTH_TOKEN);

    const request = { coin: coin };

    try {
      if (retryCount > 0) {
        console.log(`\n🔄 Reconnecting (attempt ${retryCount + 1}/${maxRetries})...`);
      } else {
        console.log(`Connecting to ${GRPC_ENDPOINT}...`);
      }

      let snapshotReceived = false;
      const call = client.StreamL4Book(request, metadata);

      call.on('data', (update) => {
        totalMsgCount++;

        if (update.snapshot) {
          const snapshot = update.snapshot;
          snapshotReceived = true;
          retryCount = 0; // Reset on success

          console.log('\n✓ L4 Snapshot Received!');
          console.log('─'.repeat(60));
          console.log(`Coin: ${snapshot.coin}`);
          console.log(`Height: ${snapshot.height}`);
          console.log(`Time: ${snapshot.time}`);
          console.log(`Bids: ${snapshot.bids.length} orders`);
          console.log(`Asks: ${snapshot.asks.length} orders`);
          console.log('─'.repeat(60));

          // Sample bids
          if (snapshot.bids.length > 0) {
            console.log('\nSample Bids (first 5):');
            snapshot.bids.slice(0, 5).forEach(order => {
              console.log(`  OID: ${order.oid} | Price: ${order.limit_px} | Size: ${order.sz} | User: ${order.user.substring(0, 10)}...`);
            });
          }

          // Sample asks
          if (snapshot.asks.length > 0) {
            console.log('\nSample Asks (first 5):');
            snapshot.asks.slice(0, 5).forEach(order => {
              console.log(`  OID: ${order.oid} | Price: ${order.limit_px} | Size: ${order.sz} | User: ${order.user.substring(0, 10)}...`);
            });
          }

        } else if (update.diff) {
          const diff = update.diff;

          if (!snapshotReceived) {
            console.log('\n⚠ Received diff before snapshot');
          }

          try {
            const diffData = JSON.parse(diff.data);
            const orderStatuses = diffData.order_statuses || [];
            const bookDiffs = diffData.book_diffs || [];

            console.log(`\n[Block ${diff.height}] L4 Diff:`);
            console.log(`  Time: ${diff.time}`);
            console.log(`  Order Statuses: ${orderStatuses.length}`);
            console.log(`  Book Diffs: ${bookDiffs.length}`);

            if (bookDiffs.length > 0 && bookDiffs.length <= 5) {
              console.log(`  Diffs: ${JSON.stringify(bookDiffs, null, 2)}`);
            }
          } catch (e) {
            console.log(`  Error parsing diff: ${e.message}`);
          }
        }

        if (maxMessages && totalMsgCount >= maxMessages) {
          console.log(`\nReached max messages (${maxMessages}), stopping...`);
          call.cancel();
        }
      });

      call.on('error', (err) => {
        if (err.code === grpc.status.DATA_LOSS && autoReconnect) {
          console.log(`\n⚠️  Server reinitialized: ${err.message}`);
          retryCount++;
          if (retryCount < maxRetries) {
            const delay = baseDelay * Math.pow(2, retryCount - 1);
            console.log(`⏳ Waiting ${delay / 1000}s before reconnecting...`);
            setTimeout(() => streamL4Orderbook(coin, maxMessages, autoReconnect), delay);
          } else {
            console.log(`\n❌ Max retries (${maxRetries}) reached. Giving up.`);
          }
        } else if (err.code !== grpc.status.CANCELLED) {
          console.error('\ngRPC error:', err.code, '-', err.message);
        }
      });

      call.on('end', () => {
        console.log('\nStream ended');
      });

      // Wait for stream to complete
      await new Promise((resolve) => {
        call.on('end', resolve);
        call.on('error', resolve);
      });

      break; // Exit retry loop on success

    } catch (err) {
      console.error('Error:', err.message);
      break;
    }
  }
}

// Parse command line args
const args = process.argv.slice(2);
const mode = args.find(a => a.startsWith('--mode='))?.split('=')[1] || 'l2';
const coin = args.find(a => a.startsWith('--coin='))?.split('=')[1] || 'BTC';
const levels = parseInt(args.find(a => a.startsWith('--levels='))?.split('=')[1]) || 20;

console.log('\n' + '='.repeat(60));
console.log('Hyperliquid Orderbook Stream Example');
console.log(`Endpoint: ${GRPC_ENDPOINT}`);
console.log('='.repeat(60));

if (mode === 'l2') {
  streamL2Orderbook(coin, levels);
} else if (mode === 'l4') {
  streamL4Orderbook(coin);
} else {
  console.log('Invalid mode. Use --mode=l2 or --mode=l4');
  process.exit(1);
}
