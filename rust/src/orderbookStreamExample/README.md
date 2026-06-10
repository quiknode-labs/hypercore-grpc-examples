# Orderbook Stream Example (Rust)

Stream Hyperliquid orderbook data from a hosted QuickNode gRPC endpoint.

## Setup

From the `rust` directory:

```bash
export GRPC_ENDPOINT="https://your-endpoint.hype-mainnet.quiknode.pro:10000"
export AUTH_TOKEN="YOUR_QUICKNODE_TOKEN"
```

## Run Streams

```bash
# New: best bid/offer
cargo run --bin orderbookStreamExample -- --mode=bbo --coin=BTC --max-messages=5

# New: L2 price-level diffs
cargo run --bin orderbookStreamExample -- --mode=l2-diff --coin=BTC --max-messages=5

# New: typed L4 order updates
cargo run --bin orderbookStreamExample -- --mode=l4-updates --coin=BTC --max-messages=5

# New: TP/SL trigger-order updates
cargo run --bin orderbookStreamExample -- --mode=tpsl --coin=BTC --max-messages=5

# Existing: full L2 snapshots
cargo run --bin orderbookStreamExample -- --mode=l2 --coin=BTC --max-messages=5

# Existing: full L4 snapshot plus raw diffs
cargo run --bin orderbookStreamExample -- --mode=l4 --coin=BTC --max-messages=5
```

## Multi-Coin Streams

```bash
cargo run --bin orderbookStreamExample -- --mode=bbo --coin=BTC,ETH,SOL --max-messages=5
cargo run --bin orderbookStreamExample -- --mode=bbo --all --max-messages=5
```

`--all` works for `bbo`, `l2-diff`, `l4-updates`, and `tpsl`. For `tpsl`, `--all` means all perp coins.

## Options

- `--mode=<l2|l4|bbo|l2-diff|l4-updates|tpsl>`
- `--coin=<COIN[,COIN...]>`
- `--all`
- `--levels=<N>` for L2 and L2 diff, default `20`, max `100`
- `--sig-figs=<N>` for L2 bucketing, `2` to `5`
- `--mantissa=<N>` for L2 bucketing, `1`, `2`, or `5`
- `--skip-initial-snapshot` for `l2-diff`
- `--max-messages=<N>`

## Client Notes

- `StreamL2Book` and `StreamL2BookDiff` share the same `n_levels`, `n_sig_figs`, and `mantissa` behavior.
- In `StreamL2BookDiff`, `sz: "0"` and `n: 0` means remove that price level.
- In `StreamL4BookUpdates` and `StreamTpslUpdates`, `DATA_LOSS` means reconnect and rebuild from the next `snapshot=true` message.
- Position TP/SL rows can have `sz: "0.0"` because that is what the node emits.
