# Orderbook Streaming

This repo includes runnable skeleton clients for the `hyperliquid.OrderBookStreaming` gRPC service. Set your QuickNode endpoint and token, then run the stream you want to test.

## Endpoint And Auth

Use your QuickNode Hyperliquid gRPC endpoint:

| Network | Endpoint format |
| --- | --- |
| Mainnet | `your-endpoint.hype-mainnet.quiknode.pro:10000` |
| Testnet | `your-endpoint.hype-testnet.quiknode.pro:10000` |

All customer connections use TLS and pass the token in the `x-token` metadata header. The examples do that for you after you set:

```bash
export GRPC_ENDPOINT="your-endpoint.hype-mainnet.quiknode.pro:10000"
export AUTH_TOKEN="YOUR_QUICKNODE_TOKEN"
```

For Rust only, include the URL scheme:

```bash
export GRPC_ENDPOINT="https://your-endpoint.hype-mainnet.quiknode.pro:10000"
```

## Run JavaScript

```bash
cd javascript/orderbookStreamExample
npm install

export GRPC_ENDPOINT="your-endpoint.hype-mainnet.quiknode.pro:10000"
export AUTH_TOKEN="YOUR_QUICKNODE_TOKEN"

node orderbook_stream_example.js --mode=bbo --coin=BTC --max-messages=5
```

## Run Python

```bash
cd python/orderbookStreamExample
pip install -r requirements.txt
python -m grpc_tools.protoc -I../../proto --python_out=. --grpc_python_out=. ../../proto/orderbook.proto

export GRPC_ENDPOINT="your-endpoint.hype-mainnet.quiknode.pro:10000"
export AUTH_TOKEN="YOUR_QUICKNODE_TOKEN"

python orderbook_stream_example.py --mode bbo --coin BTC --max-messages 5
```

## Stream Commands

These commands are the fastest way to confirm the new streams work against a customer endpoint.

| gRPC method | JavaScript command | Python command |
| --- | --- | --- |
| `StreamBboBook` | `node orderbook_stream_example.js --mode=bbo --coin=BTC --max-messages=5` | `python orderbook_stream_example.py --mode bbo --coin BTC --max-messages 5` |
| `StreamL2BookDiff` | `node orderbook_stream_example.js --mode=l2-diff --coin=BTC --max-messages=5` | `python orderbook_stream_example.py --mode l2-diff --coin BTC --max-messages 5` |
| `StreamL4BookUpdates` | `node orderbook_stream_example.js --mode=l4-updates --coin=BTC --max-messages=5` | `python orderbook_stream_example.py --mode l4-updates --coin BTC --max-messages 5` |
| `StreamTpslUpdates` | `node orderbook_stream_example.js --mode=tpsl --coin=BTC --max-messages=5` | `python orderbook_stream_example.py --mode tpsl --coin BTC --max-messages 5` |
| `StreamL2Book` | `node orderbook_stream_example.js --mode=l2 --coin=BTC --max-messages=5` | `python orderbook_stream_example.py --mode l2 --coin BTC --max-messages 5` |
| `StreamL4Book` | `node orderbook_stream_example.js --mode=l4 --coin=BTC --max-messages=5` | `python orderbook_stream_example.py --mode l4 --coin BTC --max-messages 5` |

## All-Coin Streams

The multi-coin streams accept comma-separated coins or all eligible coins.

```bash
# JavaScript
node orderbook_stream_example.js --mode=bbo --coin=BTC,ETH,SOL --max-messages=5
node orderbook_stream_example.js --mode=bbo --all --max-messages=5

# Python
python orderbook_stream_example.py --mode bbo --coin BTC,ETH,SOL --max-messages 5
python orderbook_stream_example.py --mode bbo --all --max-messages 5
```

This applies to:

- `StreamBboBook`
- `StreamL2BookDiff`
- `StreamL4BookUpdates`
- `StreamTpslUpdates`

For `StreamTpslUpdates`, `--all` means all perp coins.

## Method Summary

| Method | What it streams | When to use it |
| --- | --- | --- |
| `StreamBboBook` | Best bid and best ask. Only emits when top-of-book changes. | Live prices, spreads, ticker-style displays. |
| `StreamL2BookDiff` | Changed aggregated L2 price levels. | Maintain a local L2 book without receiving full snapshots every block. |
| `StreamL4BookUpdates` | Typed order-level changes: new, update, remove. | Maintain a local L4 book without parsing raw JSON diffs. |
| `StreamTpslUpdates` | TP/SL trigger-order adds and removes. | Trigger-order monitoring, heatmaps, alerts. |
| `StreamL2Book` | Full aggregated L2 snapshots for one coin. | Simple full-depth display or bootstrap. |
| `StreamL4Book` | Full L4 snapshot for one coin, then raw JSON diffs. | Raw node-compatible L4 book consumption. |

## Client Notes

- Prices and sizes are decimal strings.
- Timestamps are Unix milliseconds.
- `height` and `block_number` are the Hyperliquid data-layer block cursor emitted by the node.
- `StreamL2Book` and `StreamL2BookDiff` use the same L2 depth defaults: `n_levels` defaults to `20` and maxes at `100`.
- Optional L2 bucketing is consistent across `StreamL2Book` and `StreamL2BookDiff`: `n_sig_figs` is `2` to `5`, and `mantissa` is `1`, `2`, or `5`.
- In `StreamL2BookDiff`, `snapshot=true` means reset local state before applying the included levels.
- In `StreamL2BookDiff`, a level with `sz: "0"` and `n: 0` means remove that price level.
- In `StreamL4BookUpdates` and `StreamTpslUpdates`, `DATA_LOSS` means reconnect and rebuild from the next `snapshot=true` message.
- Position TP/SL orders can have `sz: "0.0"` because that is what the node emits.
