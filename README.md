# Hyperliquid gRPC Examples

gRPC streaming examples for Hyperliquid with zstd compression support.

## Languages

- **JavaScript** - Node.js with `@grpc/grpc-js`
- **Python** - `grpcio` with `zstandard`
- **Go** - `google.golang.org/grpc` with `klauspost/compress`
- **Rust** - `tonic` with `zstd`

## Proto File

The proto definition is in `proto/hyperliquid.proto`.

## Stream Types

| Type | Description |
|------|-------------|
| `TRADES` | Trade executions |
| `ORDERS` | Order updates |
| `EVENTS` | General events |
| `BOOK_UPDATES` | Order book changes |
| `TWAP` | TWAP orders |
| `BLOCKS` | Raw blocks |
| `WRITER_ACTIONS` | Writer actions |

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

1. **GRPC_ENDPOINT** - Your QuickNode endpoint (e.g., `your-endpoint.hype-mainnet.quiknode.pro:10000`)
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
