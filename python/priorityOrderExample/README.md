# Testnet Priority Mempool Example

Watch live testnet priority transactions through your QuickNode Hyperliquid gRPC endpoint.

This example uses:

- gRPC service: `hyperliquid.Streaming/StreamData`
- stream type: `MEMPOOL_TXS`
- network: testnet only

By default, the watcher prints only mempool messages that contain priority grouping:

```json
{"grouping": {"p": 13}}
```

## Setup

```bash
cd python/priorityOrderExample
pip install -r requirements.txt
python -m grpc_tools.protoc -I../../proto --python_out=. --grpc_python_out=. ../../proto/hyperliquid.proto

export GRPC_ENDPOINT="your-endpoint.hype-testnet.quiknode.pro:10000"
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

Filter by text that appears in the raw mempool payload:

```bash
python watch_priority_mempool.py --contains BTC --max-messages 5
```

Print all testnet mempool messages instead of priority-only messages:

```bash
python watch_priority_mempool.py --all-mempool --max-messages 5
```

## Notes

- `MEMPOOL_TXS` is testnet-only.
- No private key or mnemonic is required to watch the stream.
- The example connects to your QuickNode gRPC endpoint with TLS and `x-token` auth.
- Priority transactions are detected by recursively finding `grouping.p` in each mempool payload.
