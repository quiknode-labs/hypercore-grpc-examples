# Testnet Priority Mempool Example (Go)

Watch live testnet priority transactions through your QuickNode Hyperliquid gRPC endpoint.

This example subscribes to `hyperliquid.Streaming/StreamData` with stream type `MEMPOOL_TXS`. By default it prints only mempool messages that contain priority grouping, `grouping.p`.

## Setup

From the `golang` directory:

```bash
export GRPC_ENDPOINT="your-endpoint.hype-testnet.quiknode.pro:10000"
export AUTH_TOKEN="YOUR_QUICKNODE_TOKEN"
```

## Watch Priority Transactions

```bash
go run priorityOrderExample/watch_priority_mempool.go -max-messages=5
```

Print compact payloads:

```bash
go run priorityOrderExample/watch_priority_mempool.go -compact -max-messages=5
```

Filter by text:

```bash
go run priorityOrderExample/watch_priority_mempool.go -contains=BTC -max-messages=5
```

Print all testnet mempool messages instead of priority-only messages:

```bash
go run priorityOrderExample/watch_priority_mempool.go -all-mempool -max-messages=5
```

## Notes

- `MEMPOOL_TXS` is testnet-only.
- No private key or mnemonic is required to watch the stream.
- The example connects to your QuickNode gRPC endpoint with TLS and `x-token` auth.
- Priority transactions are detected by recursively finding `grouping.p` in each mempool payload.
