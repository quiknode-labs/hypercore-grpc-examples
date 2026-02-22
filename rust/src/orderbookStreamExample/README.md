# Orderbook Stream Example (Rust)

Stream real-time L2 and L4 orderbook data from Hyperliquid via gRPC.

## Setup

From the `rust` directory, build the project:

```bash
cargo build --release
```

## Usage

### Stream L2 Orderbook (Aggregated Levels)

```bash
# Default: BTC with 20 levels
cargo run --bin orderbookStreamExample -- --mode=l2 --coin=BTC

# ETH with 50 levels
cargo run --bin orderbookStreamExample -- --mode=l2 --coin=ETH --levels=50
```

### Stream L4 Orderbook (Individual Orders)

```bash
# Stream L4 for BTC
cargo run --bin orderbookStreamExample -- --mode=l4 --coin=BTC

# Limit to 100 messages
cargo run --bin orderbookStreamExample -- --mode=l4 --coin=ETH --max-messages=100
```

## Options

- `--mode=<l2|l4>`: Streaming mode
- `--coin=<COIN>`: Coin symbol to stream
- `--levels=<N>`: Number of price levels for L2 (default: 20)
- `--max-messages=<N>`: Maximum messages for L4

## Auto-Reconnect

The example includes automatic reconnection with exponential backoff when the server reinitializes (`DATA_LOSS` error). It will retry up to 10 times with delays of 2s, 4s, 8s, 16s, etc.
