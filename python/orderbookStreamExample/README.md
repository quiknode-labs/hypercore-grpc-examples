# Orderbook Stream Example

Stream real-time L2 and L4 orderbook data from Hyperliquid via gRPC.

## Features

- **L2 Streaming**: Aggregated price levels with total size and order count
- **L4 Streaming**: Individual order details with full order book snapshots and incremental diffs
- **Multiple Coins**: Stream orderbooks for multiple coins simultaneously
- **Flexible Options**: Configure number of levels, price bucketing, and more

## Setup

1. Install dependencies:
```bash
pip install -r requirements.txt
```

2. Generate proto files:
```bash
python -m grpc_tools.protoc -I../../proto --python_out=. --grpc_python_out=. ../../proto/orderbook.proto
```

## Usage

### Stream L2 Orderbook (Aggregated Levels)

Stream aggregated price levels for a single coin:

```bash
# Default: BTC with 20 levels
python orderbook_stream_example.py --mode l2 --coin BTC

# ETH with 50 levels
python orderbook_stream_example.py --mode l2 --coin ETH --levels 50

# With price bucketing
python orderbook_stream_example.py --mode l2 --coin BTC --levels 20
```

### Stream L4 Orderbook (Individual Orders)

Stream individual orders with full snapshots and diffs:

```bash
# Stream L4 for BTC
python orderbook_stream_example.py --mode l4 --coin BTC

# Limit to 100 messages
python orderbook_stream_example.py --mode l4 --coin ETH --max-messages 100
```

### Stream Both L2 and L4

Stream both orderbook types for multiple coins:

```bash
# Stream both L2 and L4 for multiple coins
python orderbook_stream_example.py --mode both --coin BTC,ETH,SOL
```

## Options

- `--mode`: Streaming mode (`l2`, `l4`, or `both`)
- `--coin`: Coin symbol(s) to stream (comma-separated for multiple)
- `--levels`: Number of price levels for L2 (default: 20, max: 100)
- `--max-messages`: Maximum number of messages to receive (L4 only)

## Output

### L2 Output Example

```
============================================================
Block: 123456 | Time: 1234567890 | Coin: BTC
────────────────────────────────────────────────────────────

  ASKS:
        95500.50 |     1.234500 | (3 orders)
        95500.00 |     2.456000 | (5 orders)
        95499.50 |     0.789000 | (2 orders)

  ────────────────────────────────────────
  SPREAD: 1.00 (1.05 bps)
  ────────────────────────────────────────

  BIDS:
        95499.50 |     3.456000 | (7 orders)
        95499.00 |     1.234000 | (4 orders)
        95498.50 |     0.567000 | (1 orders)
```

### L4 Output Example

```
✓ L4 Snapshot Received!
────────────────────────────────────────────────────────────
Coin: BTC
Height: 123456
Time: 1234567890
Bids: 1234 orders
Asks: 5678 orders
────────────────────────────────────────────────────────────

Sample Bids (first 5):
  OID: 123456789 | Price: 95500.00 | Size: 1.234 | User: 0x1234567...
  OID: 987654321 | Price: 95499.50 | Size: 2.456 | User: 0xabcdef0...

[Block 123457] L4 Diff:
  Time: 1234567900
  Order Statuses: 5
  Book Diffs: 3
```

## Protocol Buffers

The example uses the `orderbook.proto` file which defines:

- `OrderBookStreaming` service with two RPCs:
  - `StreamL2Book`: Server-streaming RPC for aggregated orderbook
  - `StreamL4Book`: Server-streaming RPC for individual orders

- Message types:
  - `L2BookRequest`, `L2BookUpdate`, `L2Level`
  - `L4BookRequest`, `L4BookUpdate`, `L4BookSnapshot`, `L4BookDiff`, `L4Order`

## Notes

- The L2 stream sends full snapshots after each block
- The L4 stream sends an initial snapshot, then incremental diffs per block
- Use Ctrl+C to stop streaming
- The endpoint requires authentication via the `x-token` metadata header
