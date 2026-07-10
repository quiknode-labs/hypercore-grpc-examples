# Priority Order Stream Example (Rust)

Watch live mainnet or testnet priority-order events through your QuickNode Hyperliquid gRPC endpoint.

This example subscribes to `hyperliquid.Streaming/StreamData` with stream type `ORDER_PRIORITY` and default filter `source=mempool_txs`. These are pre-consensus mempool events, not finalized orders. Use `--include-confirmed` to include confirmed events with `user`, `block_number`, and `outcome` when available.

## Setup

From the `rust` directory:

```bash
export GRPC_ENDPOINT="https://your-endpoint.hype-mainnet.quiknode.pro:10000"
export AUTH_TOKEN="YOUR_QUICKNODE_TOKEN"
```

## Watch Priority Transactions

```bash
cargo run --bin priorityOrderExample -- --max-messages=5
```

Print compact payloads:

```bash
cargo run --bin priorityOrderExample -- --compact --max-messages=5
```

Filter by text in the normalized order-priority event:

```bash
cargo run --bin priorityOrderExample -- --contains BTC --max-messages=5
```

Also include confirmed order-priority events from `source=replica_cmds`:

```bash
cargo run --bin priorityOrderExample -- --include-confirmed --max-messages=5
```

Inspect raw `MEMPOOL_TXS` payloads and detect `grouping.p` locally:

```bash
cargo run --bin priorityOrderExample -- --raw-mempool --max-messages=5
```

## Notes

- Default output is `source=mempool_txs`, which is mempool data and is not finalized.
- `source=replica_cmds` means the order-priority event came from confirmed block data.
- Normalized order fields include `cloid`, `tx_hash`, `vault`, `coin`, and the raw priority value `p` when available.
- No private key or mnemonic is required to watch the stream.
- The example connects to your QuickNode gRPC endpoint with TLS and `x-token` auth.
- `--raw-mempool` subscribes to `MEMPOOL_TXS`; otherwise the example subscribes to `ORDER_PRIORITY`.
