# Priority Streaming

QuickNode exposes Hyperliquid priority activity through the existing bidirectional `hyperliquid.Streaming/StreamData` RPC. The protobuf transport remains unchanged; priority is represented by two distinct `StreamType` values on both mainnet and testnet.

| Stream type | Customer use |
| --- | --- |
| `ORDER_PRIORITY` | Find and index order/write priority actions whose `grouping.p` is greater than zero |
| `GOSSIP_PRIORITY` | Find and index observed `gossipPriorityBid` actions for gossip/read priority auctions |

The split is semantic. Order priority changes write ordering, while gossip priority concerns read-side peer delivery. `GOSSIP_PRIORITY` exposes the auction bid action; it does not measure whether a customer connection received data earlier or faster.

## Network And Finality

`MEMPOOL_TXS`, `ORDER_PRIORITY`, and `GOSSIP_PRIORITY` are available on both mainnet and testnet. Connect to the endpoint for the network you need:

| Network | Endpoint format |
| --- | --- |
| Mainnet | `your-endpoint.hype-mainnet.quiknode.pro:10000` |
| Testnet | `your-endpoint.hype-testnet.quiknode.pro:10000` |

Every normalized priority event has a `source`:

| `source` | Meaning |
| --- | --- |
| `mempool_txs` | Pre-consensus observation. The action is not finalized and may fail, expire, be replaced, or never land. |
| `replica_cmds` | Confirmed block-derived observation with a block cursor, identity fields, and outcome when available. |

For a pre-consensus order subscription:

```json
{
  "subscribe": {
    "stream_type": "ORDER_PRIORITY",
    "start_block": 0,
    "filters": {
      "source": {"values": ["mempool_txs"]}
    }
  }
}
```

For IOC orders, this mempool source shows an order trying to move ahead of otherwise similar-time orders before consensus. It must not be described as finalized order flow.

## Order Events

Hyperliquid encodes the priority fee as `grouping: {"p": ...}`. `ORDER_PRIORITY` emits one normalized event per order in that action.

```json
{
  "type": "order",
  "source": "mempool_txs",
  "first_seen_time": "2026-07-09T18:00:00.000000000",
  "tx_hash": "0x3f700...",
  "signed_action_index": 0,
  "order_index": 0,
  "vault": "0x8d62...",
  "cloid": "0xclient-order-id",
  "asset_id": 0,
  "coin": "BTC",
  "market_type": "perp",
  "p": 80000,
  "side": "buy",
  "px": "105000.0",
  "sz": "0.001",
  "tif": "Ioc",
  "reduce_only": false,
  "nonce": 1780408128806,
  "expires_after": null
}
```

`coin` and `sz_decimals` are included when asset metadata is available. Confirmed `replica_cmds` events additionally include `user`, `broadcaster`, `block_number`, `block_time`, `bundle_index`, `outcome`, and `error` when available.

The raw `p` value converts as follows:

```text
fee rate = p / 100,000,000
fee in basis points = p / 10,000
```

For example, `p = 80000` represents 8 bps.

## Gossip Events

`GOSSIP_PRIORITY` emits one normalized event per observed `gossipPriorityBid` action.

```json
{
  "type": "gossip",
  "source": "mempool_txs",
  "first_seen_time": "2026-07-09T18:00:00.000000000",
  "tx_hash": "0xabc...",
  "signed_action_index": 0,
  "slot_id": 3,
  "ip": "192.0.2.10",
  "max_gas": 100000000,
  "nonce": 1780408128806,
  "expires_after": null
}
```

Confirmed `replica_cmds` events additionally include `user`, `broadcaster`, `block_number`, `block_time`, `bundle_index`, and `outcome` when available.

This event proves that the bid action was observed. It does not instrument peer delivery, compare arrival times, or prove that the bidder received gossip-prioritized data faster.

## Filtering And Indexing

Customers can apply server-side filters to normalized fields. Common patterns are:

| Goal | Stream | Suggested fields |
| --- | --- | --- |
| Watch an order before consensus | `ORDER_PRIORITY` | `source=mempool_txs`, plus `cloid`, `tx_hash`, `vault`, or `coin` |
| Find a confirmed user's order | `ORDER_PRIORITY` | `source=replica_cmds`, plus `user`, `cloid`, or `tx_hash` |
| Watch a gossip bid before consensus | `GOSSIP_PRIORITY` | `source=mempool_txs`, plus `ip`, `slot_id`, or `tx_hash` |
| Find a confirmed bidder's action | `GOSSIP_PRIORITY` | `source=replica_cmds`, plus `user`, `ip`, or `tx_hash` |

For middleware persistence, use these stable identities:

| Event | Unique source-event key | Cross-source correlation key |
| --- | --- | --- |
| Order | `(source, tx_hash, signed_action_index, order_index)` | `(tx_hash, signed_action_index, order_index)` |
| Gossip bid | `(source, tx_hash, signed_action_index)` | `(tx_hash, signed_action_index)` |

The cross-source key joins the pre-consensus mempool observation to its confirmed replica observation. `StreamData` is a live stream; middleware must persist confirmed events if customers need historical search.

## Hyperliquid Behavior

The stream API is available on both networks, while Hyperliquid's accepted actions and fee limits can differ by network:

| Network or order type | Current upstream behavior |
| --- | --- |
| Mainnet IOC | Order priority supports IOC orders on non-outcome assets, with a maximum of 8 bps. |
| Testnet IOC | The maximum is 100 bps. The 8-100 bps range has identical time preference and sorts otherwise similar-time orders within that range. |
| Testnet ALO | ALO priority affects queue position after L1 execution, not mempool prioritization. |

An observed priority action is not a guarantee of acceptance, finalization, matching, or execution. See the [Hyperliquid priority fees documentation](https://hyperliquid.gitbook.io/hyperliquid-docs/for-developers/api/priority-fees) for the current protocol rules.

## Run The Examples

Runnable `ORDER_PRIORITY` watchers are available in JavaScript, Python, Go, and Rust under each language's `priorityOrderExample` directory. They use `source=mempool_txs` by default and support `--include-confirmed` for `source=replica_cmds` events.

Use `--raw-mempool` to subscribe to `MEMPOOL_TXS` instead. The generic raw-stream examples also accept `GOSSIP_PRIORITY` as the stream type.
