# Orderbook Stream Example (Go)

Stream real-time L2 and L4 orderbook data from Hyperliquid via gRPC.

## Setup

From the `golang` directory, generate proto files:

```bash
cd ..
./generate_proto.sh
```

## Usage

### Stream L2 Orderbook (Aggregated Levels)

```bash
# Default: BTC with 20 levels
go run orderbook_stream_example.go -mode=l2 -coin=BTC

# ETH with 50 levels
go run orderbook_stream_example.go -mode=l2 -coin=ETH -levels=50
```

### Stream L4 Orderbook (Individual Orders)

```bash
# Stream L4 for BTC
go run orderbook_stream_example.go -mode=l4 -coin=BTC

# Limit to 100 messages
go run orderbook_stream_example.go -mode=l4 -coin=ETH -max-messages=100
```

## Options

- `-mode=<l2|l4>`: Streaming mode
- `-coin=<COIN>`: Coin symbol to stream
- `-levels=<N>`: Number of price levels for L2 (default: 20)
- `-max-messages=<N>`: Maximum messages for L4 (0 = unlimited)

## Auto-Reconnect

The example includes automatic reconnection with exponential backoff when the server reinitializes (`DATA_LOSS` error). It will retry up to 10 times with delays of 2s, 4s, 8s, 16s, etc.
