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

## L4 ALO Queue Priority

Version `1.0.70` accounts for Hyperliquid's ALO priority-fee queue insertions
without changing the public gRPC or JSON schemas. The upstream `insertBefore`
metadata is used to construct the canonical queue but is not added to customer
diff payloads.

Instead, the L4 streams use their existing snapshot mechanisms:

- `StreamL4Book` can send another `L4BookSnapshot` after the initial snapshot.
  Discard both sides of the local book and rebuild them from `bids` and `asks`
  in the order emitted. Continue applying later raw diffs from that snapshot's
  `height`.
- `StreamL4BookUpdates` can send an update with `snapshot=true`. Clear the
  keyed local order state before applying every order in that update.

Treat every snapshot as authoritative, including snapshots received after
normal incremental updates. A replacement snapshot may be sent for an ALO
queue insertion or when stream state is rebuilt. Clients that already reset on
every snapshot require no wire-format changes. Clients that assumed the first
snapshot was the only snapshot must update that behavior to preserve exact L4
queue order.

This is not a breaking response change: no protobuf field was added or removed,
and raw diff JSON retains its existing shape. For live book maintenance, the
replacement snapshot contains the complete canonical state at its height. It
replaces the priority-insertion diff rather than exposing that internal
mutation as a new public event shape.

Replacement snapshots contain the full L4 depth and can be much larger than an
incremental update, especially for BTC. Clients should allow at least 100 MB
for inbound gRPC messages, as the examples do, and apply each reset atomically
before processing later updates.

The JavaScript, Python, Go, and Rust `l4` examples label snapshots as
`reset=initial` or `reset=replacement` and explicitly call out when the entire
local L4 book must be replaced.
