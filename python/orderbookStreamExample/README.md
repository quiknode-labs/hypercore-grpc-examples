# Orderbook Stream Example (Python)

Stream Hyperliquid orderbook data from a hosted QuickNode gRPC endpoint.

## Setup

```bash
pip install -r requirements.txt
python -m grpc_tools.protoc -I../../proto --python_out=. --grpc_python_out=. ../../proto/orderbook.proto
```

Set your endpoint and auth token:

```bash
export GRPC_ENDPOINT="your-endpoint.hype-mainnet.quiknode.pro:10000"
export AUTH_TOKEN="YOUR_QUICKNODE_TOKEN"
```

## Usage

### BBO

```bash
python orderbook_stream_example.py --mode bbo --coin BTC
python orderbook_stream_example.py --mode bbo --coin BTC,ETH,SOL
python orderbook_stream_example.py --mode bbo --all
```

### L2 Full Snapshots

```bash
python orderbook_stream_example.py --mode l2 --coin BTC
python orderbook_stream_example.py --mode l2 --coin ETH --levels 50
python orderbook_stream_example.py --mode l2 --coin BTC --sig-figs 5 --mantissa 1
```

### L2 Diffs

```bash
python orderbook_stream_example.py --mode l2-diff --coin BTC
python orderbook_stream_example.py --mode l2-diff --coin BTC,ETH
python orderbook_stream_example.py --mode l2-diff --all
python orderbook_stream_example.py --mode l2-diff --coin BTC --skip-initial-snapshot
```

### L4 Full Book

```bash
python orderbook_stream_example.py --mode l4 --coin BTC
python orderbook_stream_example.py --mode l4 --coin ETH --max-messages 100
```

Version `1.0.70` preserves ALO priority-fee queue ordering without changing the
public response shape. The stream can send a full snapshot again after normal
diffs. The example labels it `reset=replacement`; discard the entire local L4
book and rebuild `bids` and `asks` in the emitted order.

### Typed L4 Updates

```bash
python orderbook_stream_example.py --mode l4-updates --coin BTC
python orderbook_stream_example.py --mode l4-updates --all
```

### TP/SL Updates

```bash
python orderbook_stream_example.py --mode tpsl --coin BTC
python orderbook_stream_example.py --mode tpsl --all
```

## Options

- `--mode`: `l2`, `l4`, `bbo`, `l2-diff`, `l4-updates`, or `tpsl`
- `--coin`: Coin symbol or comma-separated symbols
- `--all`: Subscribe to all eligible coins on multi-coin streams
- `--levels`: Number of L2 levels, default `20`, max `100`
- `--sig-figs`: L2 bucketing significant figures, `2` to `5`
- `--mantissa`: L2 bucketing mantissa, `1`, `2`, or `5`
- `--skip-initial-snapshot`: For `l2-diff`, skip the initial snapshot
- `--max-messages`: Stop after receiving N messages

## Client Notes

- `StreamL2Book` and `StreamL2BookDiff` share the same depth and bucketing defaults.
- In `StreamL2BookDiff`, `sz: "0"` and `n: 0` means remove that price level.
- In `StreamL2BookDiff`, `snapshot=true` means reset local state before applying the included levels.
- In `StreamL4BookUpdates` and `StreamTpslUpdates`, `DATA_LOSS` means reconnect and rebuild from the next `snapshot=true` message.
- On `StreamL4Book`, treat every snapshot as authoritative and replace the entire local book.
- On `StreamL4BookUpdates`, clear local order state whenever `snapshot=true` before applying the update.
- Position TP/SL rows can have `sz: "0.0"` because that is what the node emits.
