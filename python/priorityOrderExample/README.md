# Priority Order Stream Example

Watch live mainnet or testnet priority-order events through your QuickNode Hyperliquid gRPC endpoint.

By default this example subscribes to `ORDER_PRIORITY` with `source=mempool_txs`. These are pre-consensus mempool events, not finalized orders. Use `--include-confirmed` to include confirmed events with `user`, `block_number`, and `outcome` when available.

This example uses:

- gRPC service: `hyperliquid.Streaming/StreamData`
- stream type: `ORDER_PRIORITY`
- default filter: `source=mempool_txs`
- network: mainnet or testnet

Priority orders include a priority value:

```json
{"p": 10000, "source": "mempool_txs"}
```

## Setup

```bash
cd python/priorityOrderExample
pip install -r requirements.txt
python -m grpc_tools.protoc -I../../proto --python_out=. --grpc_python_out=. ../../proto/hyperliquid.proto

export GRPC_ENDPOINT="your-endpoint.hype-mainnet.quiknode.pro:10000"
export AUTH_TOKEN="YOUR_QUICKNODE_TOKEN"
```

## Watch Priority Transactions

```bash
python watch_priority_mempool.py --max-messages 5
```

Print compact payloads:

```bash
python watch_priority_mempool.py --compact --max-messages 5
```

Filter by text that appears in the normalized priority event:

```bash
python watch_priority_mempool.py --contains BTC --max-messages 5
```

Also include confirmed priority events from `source=replica_cmds`:

```bash
python watch_priority_mempool.py --include-confirmed --max-messages 5
```

Inspect raw `MEMPOOL_TXS` payloads and detect `grouping.p` locally:

```bash
python watch_priority_mempool.py --raw-mempool --max-messages 5
```

## Notes

- Default output is `source=mempool_txs`, which is mempool data and is not finalized.
- `source=replica_cmds` means the priority event came from confirmed block data.
- Normalized order fields include `cloid`, `tx_hash`, `vault`, `coin`, and the raw priority value `p` when available.
- No private key or mnemonic is required to watch the stream.
- The example connects to your QuickNode gRPC endpoint with TLS and `x-token` auth.
- `--raw-mempool` subscribes to `MEMPOOL_TXS`; otherwise the example subscribes to `ORDER_PRIORITY`.
