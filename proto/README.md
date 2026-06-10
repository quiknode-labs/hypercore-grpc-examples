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
  - `StreamBboBook` - Stream best bid and offer changes
  - `StreamL2BookDiff` - Stream changed aggregated L2 price levels
  - `StreamL4BookUpdates` - Stream typed L4 order-level changes
  - `StreamTpslUpdates` - Stream typed TP/SL trigger-order changes

#### L2 Book Features
- Aggregated price levels with total size and order count
- Configurable number of levels (default 20, up to 100)
- Optional price bucketing with significance figures and mantissa
- Full snapshot sent after each block

#### BBO Features
- Best bid and best ask for one or more coins
- Emits only when the top bid or ask changes
- Empty `coins` means all coins
- Uses the same `L2Level` shape as L2 book levels

#### L2 Book Diff Features
- Incremental price-level changes for one or more coins
- Empty `coins` means all coins
- Uses the same `n_levels`, `n_sig_figs`, and `mantissa` parameters as `StreamL2Book`
- `snapshot=true` means reset local state before applying levels
- A changed level with `sz: "0"` and `n: 0` means remove that price level

#### L4 Book Features
- Individual order details with full order information
- Initial snapshot on connection
- Incremental diffs per block (as JSON)
- Includes order IDs, prices, sizes, users, triggers, etc.

#### L4 Book Updates Features
- Typed order-level changes for one or more coins
- Empty `coins` means all coins
- Diff types are `NEW`, `UPDATE`, and `REMOVE`
- `snapshot=true` means rebuild local L4 state from the included diffs
- `DATA_LOSS` means reconnect and rebuild from the next snapshot

#### TP/SL Updates Features
- Trigger-order lifecycle updates for one or more perp coins
- Empty `coins` means all perp coins
- Diff types are `ADD` and `REMOVE`
- `snapshot=true` means rebuild local TP/SL state from the included adds
- Position TP/SL orders can have `sz: "0.0"` because that is what the node emits
- `DATA_LOSS` means reconnect and rebuild from the next snapshot

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

For runnable examples using a QuickNode endpoint, see `docs/orderbook-streaming.md`.
