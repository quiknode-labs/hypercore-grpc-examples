# Raw Mempool Filtering

`MEMPOOL_TXS` is a pre-consensus stream. A transaction may fail, expire, be
replaced, or never be included in a block.

## Coin semantics

Raw mempool messages are either a tuple such as `[first_seen_time, tx]` or a
single transaction object. In both shapes, `signed_actions[].action` contains
numeric asset IDs rather than a customer-facing top-level `coin` field.

The virtual `coin` and `coins` filters resolve names through the server's
current asset metadata. Customers therefore subscribe with names such as
`BTC`, while newly listed markets do not require a client release or a static
asset table in these examples.

A transaction matches when any of these order-touching actions references a
requested asset:

| Action | Asset locations |
|---|---|
| `order` | `orders[].a` or `orders[].asset` |
| `cancel` | `cancels[].a` or `cancels[].asset` |
| `cancelByCloid` | `cancels[].asset` or `cancels[].a` |
| `batchModify` | `modifies[].order.a`, `modifies[].order.asset`, or a direct asset field |
| `modify` | `order.a`, `order.asset`, or a direct asset field |
| `twapOrder` | `twap.a` or `twap.asset` |
| `twapCancel` | `a` or `asset` |

Matching is at the transaction boundary. If one signed action matches, the
server returns the complete original raw transaction, including unrelated
signed actions in the same tuple. The examples parse a copy for validation but
do not transform the gRPC `data` string; use `--print-raw` (or `-print-raw` in
Go) to print that original string.

`--expected-asset-ids=0` is a client-side assertion used by the BTC example.
It is not sent as part of the coin filter and is not the source of the server's
mapping.

## Configuration

```bash
export GRPC_ENDPOINT="your-endpoint.hype-mainnet.quiknode.pro:10000"
export AUTH_TOKEN="YOUR_QUICKNODE_TOKEN"
```

Credentials are read only from the environment. Do not commit endpoint tokens.

## Runnable examples

All four examples default to `coin=BTC`, five matching messages, and a bounded
60-second timeout.

```bash
# JavaScript
node javascript/mempoolFilterExample/mempool_filter_example.js \
  --filter-field=coin --filter-values=BTC --expected-asset-ids=0

# Python
python3 python/mempoolFilterExample/mempool_filter_example.py \
  --filter-field coin --coin BTC --expected-asset-ids 0

# Go
(cd golang && go run ./mempoolFilterExample \
  -filter-field coin -coin BTC -expected-asset-ids 0)

# Rust
(cd rust && cargo run --bin mempool_filter_example -- \
  --filter-field coin --coin BTC --expected-asset-ids 0)
```

Use a deliberately unknown coin to verify the negative control:

```bash
node javascript/mempoolFilterExample/mempool_filter_example.js \
  --filter-field=coin --filter-values=__NO_SUCH_COIN__ \
  --expect-no-match --timeout-seconds=20 --max-messages=1
```

Use `--unfiltered` to confirm raw stream availability before testing a filter.

## Unit tests

The tests cover tuple and object roots, all supported order-touching action
shapes, invalid assets, and proof that client-side extraction does not mutate
the parsed raw value.

```bash
node --test javascript/mempoolFilterExample/mempool_filter_example.test.js
python3 -m unittest python/mempoolFilterExample/test_mempool_filter_example.py
(cd golang && go test ./mempoolFilterExample)
(cd rust && cargo test --bin mempool_filter_example)
```
