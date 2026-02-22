# Protocol Buffer Definitions

This directory contains the gRPC service definitions for Hyperliquid streaming APIs.

## Available Proto Files

### hyperliquid.proto

Main streaming service for blockchain data:
- **Service**: `Streaming`
- **Methods**:
  - `StreamData` - Bi-directional streaming for trades, orders, book updates, etc.
  - `Ping` - Health check
- **Stream Types**: TRADES, ORDERS, BOOK_UPDATES, TWAP, EVENTS, BLOCKS, WRITER_ACTIONS

### orderbook.proto

Dedicated orderbook streaming service:
- **Service**: `OrderBookStreaming`
- **Methods**:
  - `StreamL2Book` - Stream aggregated price levels (L2 orderbook)
  - `StreamL4Book` - Stream individual orders (L4 orderbook)

#### L2 Book Features
- Aggregated price levels with total size and order count
- Configurable number of levels (up to 100)
- Optional price bucketing with significance figures and mantissa
- Full snapshot sent after each block

#### L4 Book Features
- Individual order details with full order information
- Initial snapshot on connection
- Incremental diffs per block (as JSON)
- Includes order IDs, prices, sizes, users, triggers, etc.

## Generating Code

### Python
```bash
python -m grpc_tools.protoc -I. --python_out=. --grpc_python_out=. hyperliquid.proto
python -m grpc_tools.protoc -I. --python_out=. --grpc_python_out=. orderbook.proto
```

### JavaScript
```bash
# Using @grpc/proto-loader (runtime loading, no generation needed)
const protoLoader = require('@grpc/proto-loader');
const packageDefinition = protoLoader.loadSync('orderbook.proto', {});
```

### Go
```bash
protoc --go_out=. --go-grpc_out=. hyperliquid.proto
protoc --go_out=. --go-grpc_out=. orderbook.proto
```

### Rust
```bash
# Add to build.rs:
tonic_build::compile_protos("orderbook.proto")?;
```

## Usage Examples

See the language-specific example directories:
- `python/orderbookStreamExample/`
- `javascript/orderbookStreamExample/`
- `golang/orderbookStreamExample/`
- `rust/src/orderbookStreamExample/`
