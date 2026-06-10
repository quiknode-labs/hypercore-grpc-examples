# Orderbook Stream Example (JavaScript)

Stream real-time Hyperliquid orderbook data from a QuickNode gRPC endpoint.

## Setup

```bash
npm install
```

Set your hosted QuickNode endpoint and token:

```bash
export GRPC_ENDPOINT="your-endpoint.hype-mainnet.quiknode.pro:10000"
export AUTH_TOKEN="YOUR_QUICKNODE_TOKEN"
```

## Usage

### Stream L2 Orderbook (Aggregated Levels)

```bash
# Default: BTC with 20 levels
node orderbook_stream_example.js --mode=l2 --coin=BTC

# ETH with 50 levels
node orderbook_stream_example.js --mode=l2 --coin=ETH --levels=50

# With price bucketing (merges nearby price levels to reduce data)
node orderbook_stream_example.js --mode=l2 --coin=BTC --sig-figs=5 --mantissa=1

# Or use npm scripts
npm run l2
```

### Stream L4 Orderbook (Individual Orders)

```bash
# Stream L4 for BTC
node orderbook_stream_example.js --mode=l4 --coin=BTC

# Or use npm scripts
npm run l4
```

### Stream BBO (Best Bid/Offer)

```bash
# BTC best bid/offer
node orderbook_stream_example.js --mode=bbo --coin=BTC

# Multiple coins
node orderbook_stream_example.js --mode=bbo --coin=BTC,ETH,SOL

# All coins
node orderbook_stream_example.js --mode=bbo --all

# Or use npm scripts
npm run bbo
```

### Stream L2 Book Diffs

```bash
# BTC L2 price-level changes with the default 20 levels
node orderbook_stream_example.js --mode=l2-diff --coin=BTC

# Multiple coins
node orderbook_stream_example.js --mode=l2-diff --coin=BTC,ETH

# All coins
node orderbook_stream_example.js --mode=l2-diff --all

# Skip initial snapshot and only receive future changes
node orderbook_stream_example.js --mode=l2-diff --coin=BTC --skip-initial-snapshot

# Or use npm scripts
npm run l2-diff
```

### Stream Typed L4 Updates

```bash
# BTC typed order-level changes
node orderbook_stream_example.js --mode=l4-updates --coin=BTC

# All coins
node orderbook_stream_example.js --mode=l4-updates --all

# Or use npm scripts
npm run l4-updates
```

### Stream TP/SL Updates

```bash
# BTC trigger-order lifecycle updates
node orderbook_stream_example.js --mode=tpsl --coin=BTC

# All perp coins
node orderbook_stream_example.js --mode=tpsl --all

# Or use npm scripts
npm run tpsl
```

## Options

- `--mode=<l2|l4|bbo|l2-diff|l4-updates|tpsl>`: Streaming mode
- `--coin=<COIN[,COIN...]>`: Coin symbol or comma-separated symbols to stream
- `--all`: Subscribe to all eligible coins on multi-coin streams
- `--levels=<N>`: Number of price levels for L2 (default: 20)
- `--sig-figs=<N>`: Significant figures for L2 price bucketing (2-5)
- `--mantissa=<N>`: Mantissa for L2 price bucketing (1, 2, or 5)
- `--skip-initial-snapshot`: For `l2-diff`, skip the initial snapshot
- `--max-messages=<N>`: Stop after receiving N messages

## Auto-Reconnect

The example includes automatic reconnection with exponential backoff when the server returns `DATA_LOSS`. For `l4-updates` and `tpsl`, clients should rebuild local state from the next `snapshot=true` message after reconnecting.
