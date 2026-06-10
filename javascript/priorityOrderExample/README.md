# Testnet Priority Mempool Example (JavaScript)

Watch live testnet priority transactions through your QuickNode Hyperliquid gRPC endpoint.

This example subscribes to `hyperliquid.Streaming/StreamData` with stream type `MEMPOOL_TXS`. By default it prints only mempool messages that contain priority grouping, `grouping.p`.

## Setup

```bash
cd javascript/priorityOrderExample
npm install

export GRPC_ENDPOINT="your-endpoint.hype-testnet.quiknode.pro:10000"
export AUTH_TOKEN="YOUR_QUICKNODE_TOKEN"
```

## Watch Priority Transactions

```bash
node watch_priority_mempool.js --max-messages=5
```

Print compact payloads:

```bash
node watch_priority_mempool.js --compact --max-messages=5
```

Filter by text:

```bash
node watch_priority_mempool.js --contains=BTC --max-messages=5
```

Print all testnet mempool messages instead of priority-only messages:

```bash
node watch_priority_mempool.js --all-mempool --max-messages=5
```

## Notes

- `MEMPOOL_TXS` is testnet-only.
- No private key or mnemonic is required to watch the stream.
- The example connects to your QuickNode gRPC endpoint with TLS and `x-token` auth.
- Priority transactions are detected by recursively finding `grouping.p` in each mempool payload.
