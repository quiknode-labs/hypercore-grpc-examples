# Orderbook Stream Example (JavaScript)

Stream real-time L2 and L4 orderbook data from Hyperliquid via gRPC.

## Setup

```bash
npm install
```

## Usage

### Stream L2 Orderbook (Aggregated Levels)

```bash
# Default: BTC with 20 levels
node orderbook_stream_example.js --mode=l2 --coin=BTC

# ETH with 50 levels
node orderbook_stream_example.js --mode=l2 --coin=ETH --levels=50

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

## Options

- `--mode=<l2|l4>`: Streaming mode
- `--coin=<COIN>`: Coin symbol to stream
- `--levels=<N>`: Number of price levels for L2 (default: 20)

## Auto-Reconnect

The example includes automatic reconnection with exponential backoff when the server reinitializes (`DATA_LOSS` error). It will retry up to 10 times with delays of 2s, 4s, 8s, 16s, etc.
