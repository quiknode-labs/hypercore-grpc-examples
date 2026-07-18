# Hyperliquid gRPC Examples

gRPC streaming examples for Hyperliquid with zstd compression support.

## Languages

- **JavaScript** - Node.js with `@grpc/grpc-js`
- **Python** - `grpcio` with `zstandard`
- **Go** - `google.golang.org/grpc` with `klauspost/compress`
- **Rust** - `tonic` with `zstd`

## Proto Files

The raw stream proto definition is in `proto/hyperliquid.proto`.

The dedicated orderbook streaming proto definition is in `proto/orderbook.proto`. See [Orderbook Streaming](docs/orderbook-streaming.md) for runnable examples using your QuickNode endpoint.

## Networks

Both **mainnet** and **testnet** are supported. Set your `GRPC_ENDPOINT` accordingly:

| Network | Endpoint Format |
|---------|----------------|
| Mainnet | `your-endpoint.hype-mainnet.quiknode.pro:10000` |
| Testnet | `your-endpoint.hype-testnet.quiknode.pro:10000` |

All stream types below are available on mainnet and testnet. The endpoint determines which network's node data you receive.

## Stream Types

| Type | Description | Networks |
|------|-------------|----------|
| `TRADES` | Trade executions | Mainnet, Testnet |
| `ORDERS` | Order updates | Mainnet, Testnet |
| `EVENTS` | General events | Mainnet, Testnet |
| `BOOK_UPDATES` | Order book changes | Mainnet, Testnet |
| `TWAP` | TWAP orders | Mainnet, Testnet |
| `BLOCKS` | Raw blocks | Mainnet, Testnet |
| `WRITER_ACTIONS` | Writer actions | Mainnet, Testnet |
| `MEMPOOL_TXS` | Raw pre-consensus mempool transactions | Mainnet, Testnet |
| `ORDER_PRIORITY` | Filterable order/write priority actions with `grouping.p > 0` | Mainnet, Testnet |
| `GOSSIP_PRIORITY` | Filterable gossip/read priority auction bid actions | Mainnet, Testnet |

## Orderbook Streaming Methods

The `hyperliquid.OrderBookStreaming` service exposes full-book streams plus lower-bandwidth derived streams:

| Method | Description |
|--------|-------------|
| `StreamL2Book` | Existing full aggregated L2 snapshots for one coin |
| `StreamL4Book` | Existing full L4 snapshot for one coin, then raw JSON diffs |
| `StreamBboBook` | New best bid/offer updates for one or more coins |
| `StreamL2BookDiff` | New incremental L2 price-level changes |
| `StreamL4BookUpdates` | New typed L4 order-level changes |
| `StreamTpslUpdates` | New typed TP/SL trigger-order lifecycle changes |

L4 streams keep books correctly ordered when ALO priority fees change queue
placement. The public response schema is unchanged: `StreamL4Book` emits
an authoritative replacement snapshot, and `StreamL4BookUpdates` emits an
update with `snapshot=true`. Clients must clear and rebuild local state whenever
either stream sends a snapshot, not only on initial subscription. See
[L4 ALO Queue Priority](docs/orderbook-streaming.md#l4-alo-queue-priority).

Run BBO with your hosted QuickNode endpoint:

```bash
cd javascript/orderbookStreamExample
npm install

export GRPC_ENDPOINT="your-endpoint.hype-mainnet.quiknode.pro:10000"
export AUTH_TOKEN="YOUR_QUICKNODE_TOKEN"

node orderbook_stream_example.js --mode=bbo --coin=BTC --max-messages=5
```

## Priority Streams

`ORDER_PRIORITY` and `GOSSIP_PRIORITY` are available on mainnet and testnet. `ORDER_PRIORITY` lets customers filter priority orders by fields such as `cloid`, `user`, `tx_hash`, `coin`, and `source`. `GOSSIP_PRIORITY` lets customers filter bid actions by `user`, `tx_hash`, `ip`, `slot_id`, and `source`.

The examples use `source=mempool_txs` by default, so their output is pre-consensus and not finalized. For IOC orders, this is the view of an order trying to move ahead of otherwise similar-time orders in mempool sorting. `GOSSIP_PRIORITY` surfaces the `gossipPriorityBid` action; it does not measure whether a connection received data faster.

Use `--include-confirmed` to also receive confirmed `source=replica_cmds` order-priority events, or `--raw-mempool` to inspect raw `MEMPOOL_TXS` payloads. See [Priority Streaming](docs/priority-streaming.md) for searchable fields, payloads, and network-specific fee behavior.

```bash
cd python/priorityOrderExample
pip install -r requirements.txt
python -m grpc_tools.protoc -I../../proto --python_out=. --grpc_python_out=. ../../proto/hyperliquid.proto

export GRPC_ENDPOINT="your-endpoint.hype-mainnet.quiknode.pro:10000"
export AUTH_TOKEN="YOUR_QUICKNODE_TOKEN"

python watch_priority_mempool.py --max-messages 5
```

Priority examples are available in:

- `javascript/priorityOrderExample/`
- `python/priorityOrderExample/`
- `golang/priorityOrderExample/`
- `rust/src/priorityOrderExample/`

## Raw Mempool Coin Filtering

`MEMPOOL_TXS` supports the virtual server-side fields `coin` and `coins`. Raw
mempool payloads contain numeric asset IDs, so the server resolves current coin
names dynamically and matches the transaction when any order-touching action
uses a requested asset. The response remains the original raw JSON tuple or
object; it is not replaced by a normalized event.

The dedicated examples default to `coin=BTC` and independently verify asset ID
`0` in each returned raw transaction:

```bash
export GRPC_ENDPOINT="your-endpoint.hype-mainnet.quiknode.pro:10000"
export AUTH_TOKEN="YOUR_QUICKNODE_TOKEN"

# JavaScript
node javascript/mempoolFilterExample/mempool_filter_example.js --max-messages=5

# Python
python3 python/mempoolFilterExample/mempool_filter_example.py --max-messages 5

# Go
(cd golang && go run ./mempoolFilterExample -max-messages 5)

# Rust
(cd rust && cargo run --bin mempool_filter_example -- --max-messages 5)
```

The filter spans `order`, `cancel`, `cancelByCloid`, `batchModify`, `modify`,
`twapOrder`, and `twapCancel`. See [Mempool Filtering](docs/mempool-filtering.md)
for semantics, visible 30-second Ping/Pong heartbeats, non-matching controls,
raw output, and unit-test commands.

## Filtering

Each language includes a dedicated `filter_example` file demonstrating server-side filtering.

### Filter Example Files

```bash
# JavaScript
node filter_example.js

# Python
python filter_example.py

# Go
go run filter_example.go

# Rust
cargo run --bin filter_example
```

### How Filters Work

Filters are applied server-side via the `filters` field in the subscription request:

```javascript
// JavaScript example
call.write({
  subscribe: {
    stream_type: 'TRADES',
    filters: {
      coin: { values: ['ETH', 'BTC'] },
      user: { values: ['0x123...'] }
    },
    filter_name: 'my-filter'
  }
});
```

### CLI Filtering (main examples)

The main examples also support filtering via command line:

```bash
# JavaScript
node index.js TRADES --filter coin=ETH,BTC --filter user=0x123

# Python
python main.py TRADES --filter coin=ETH,BTC

# Go
go run main.go -stream TRADES -filter "coin=ETH,BTC;user=0x123"

# Rust
cargo run --bin main -- -s TRADES -f coin=ETH,BTC -f user=0x123
```

## Quick Start

### JavaScript

```bash
cd javascript
npm install
# Edit index.js to set GRPC_ENDPOINT and AUTH_TOKEN
node index.js TRADES
```

### Python

```bash
cd python
pip install -r requirements.txt
./generate_proto.sh
# Edit main.py to set GRPC_ENDPOINT and AUTH_TOKEN
python main.py TRADES
```

### Go

```bash
cd golang
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
./generate_proto.sh
go mod tidy
# Edit main.go to set grpcEndpoint and authToken
go run main.go -stream TRADES
```

### Rust

```bash
cd rust
# Edit src/main.rs to set GRPC_ENDPOINT and AUTH_TOKEN
cargo run -- -s TRADES
```

## Configuration

Each example requires:

1. **GRPC_ENDPOINT** - Your QuickNode endpoint (e.g., `your-endpoint.hype-mainnet.quiknode.pro:10000` or `your-endpoint.hype-testnet.quiknode.pro:10000`)
2. **AUTH_TOKEN** - Your authentication token

### Connection Requirements

- **Port**: `10000` (gRPC streaming port)
- **TLS**: Required - all connections must use TLS/SSL
- **Authentication**: Pass your token via the `x-token` metadata header

## Zstd Compression

All examples automatically detect and decompress zstd-compressed data by checking for the magic number `0x28 0xB5 0x2F 0xFD`.

## Connection Management

gRPC streams are long-lived connections that can disconnect due to network issues, server restarts, or idle timeouts. Production systems should implement proper connection management.

### Keep-Alive Pings

The server expects periodic pings to keep the connection alive. The examples send pings every 30 seconds:

```javascript
// JavaScript
setInterval(() => {
  call.write({ ping: { timestamp: Date.now() } });
}, 30000);
```

```python
# Python
yield pb.SubscribeRequest(ping=pb.Ping(timestamp=int(time.time() * 1000)))
```

The server responds with a `pong` message. If you stop receiving pongs, the connection may be dead.

### Reconnection Strategy

When a disconnect occurs, implement exponential backoff:

```
Attempt 1: Wait 1s
Attempt 2: Wait 2s
Attempt 3: Wait 4s
Attempt 4: Wait 8s
...
Max backoff: 60s
```

### Handling Missed Blocks

When your connection drops, you'll miss blocks. On reconnect:

1. Track the last `block_number` you received
2. Reconnect with `start_block` set to resume from where you left off
3. For the `BLOCKS` stream specifically, historical data isn't available via gRPC - see the `replicaCmdsOnS3Example` for backfilling from the Hyperliquid Foundation S3 bucket

### Example Reconnect Flow

```
1. Connect to gRPC stream
2. Subscribe with start_block=0 (or last known block)
3. Process incoming data, track last block_number
4. Send pings every 30s
5. On disconnect:
   - Log last block_number received
   - Wait with exponential backoff
   - Reconnect and subscribe with start_block=last_block_number
6. Repeat
```

**Note:** These examples are starting points. Production systems should add error handling, metrics, circuit breakers, and dead letter queues based on your reliability requirements.
