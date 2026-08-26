# HIP-4 Outcome Markets

HIP-4 outcome markets trade on coins named `#N` (for example `#146870`), where
`N = 10 * outcome_id + side`. Outcomes are grouped into **venues** run by
deployers, and venues churn continuously as outcomes settle and new ones
activate — a static coin list goes stale within minutes. Three streaming
features make these markets practical to consume:

## 1. Venue and deployer filtering

`venue`, `venues`, `deployer`, and `deployers` are reserved filter keys on
`StreamSubscribe.filters`. They are resolved **server-side** against the
node's live outcome registry (refreshed every 60 seconds) and expanded to the
venue's current coin set, so a subscription follows the venue through outcome
churn without resubscribing:

```json
{ "venue": { "values": ["txyz"] } }
```

- `venue`/`venues` take venue names; `deployer`/`deployers` take deployer
  addresses (HIP-3 dex names/deployers are also accepted by the same keys).
- Unknown venues match nothing (the stream stays open and silently delivers
  no data — check your venue name against `outcomeMeta` if a stream is
  unexpectedly quiet).
- Combining `venue` with an explicit `coin` filter intersects the two.

Discover active venues from the info endpoint:

```bash
curl -s -X POST "https://your-endpoint.hype-testnet.quiknode.pro/YOUR_TOKEN/info" \
  -H "Content-Type: application/json" -d '{"type":"outcomeMeta"}'
```

`outcomes[].venue` maps outcomes to venues; `deployers[]` lists each
deployer's venue. Coins for an outcome are `#(10*outcome + 0)` and
`#(10*outcome + 1)`.

## 2. Subscription identification

`StreamSubscribe.subscription_id` is a client-chosen tag echoed back on every
update for that stream type, alongside the populated
`StreamResponse.stream_type`. With multiple subscriptions multiplexed on one
`StreamData` connection, every update says exactly which subscription it
belongs to. One tag per (connection, stream type); the latest subscribe wins.

## 3. Signer enrichment

`StreamSubscribe.enrichment.include_signer = true` (TRADES/ORDERS only) adds a
`"signer"` field to each event: the wallet that actually **submitted** the
order — the master account itself, or the approved API/agent wallet that
signed on its behalf — recovered from the action's signature. Events with no
originating signed transaction (trigger fires, liquidations, TWAP children)
carry `"signer": null`.

For historical lookups of the same attribution, the JSON-RPC method
`hl_getSigner` accepts an order/event hash and returns the signer.

## Configuration

```bash
export GRPC_ENDPOINT="your-endpoint.hype-testnet.quiknode.pro:10000"
export AUTH_TOKEN="YOUR_QUICKNODE_TOKEN"
```

HIP-4 launches on testnet first; the examples default to a testnet endpoint
placeholder. At mainnet launch the identical subscription works against your
`hype-mainnet` endpoint. Credentials are read only from constants you edit or
the environment — do not commit endpoint tokens.

## Runnable examples

Each example subscribes to ORDERS for one venue with a subscription tag and
signer enrichment, then prints `streamType`, `subscriptionId`, and each
order's `coin`/`user`/`signer`. Edit the endpoint, token, and venue constants
at the top of each file first.

```bash
# JavaScript
node javascript/hip4VenueExample/hip4_venue_example.js

# Python
python3 python/hip4VenueExample/hip4_venue_example.py

# Go
cd golang && go run ./hip4VenueExample

# Rust
cd rust && cargo run --bin hip4_venue_example
```

Outcome-market order flow on testnet is bursty; a correctly-filtered stream
can be quiet for minutes at a time. The subscription acknowledges nothing on
subscribe — data arriving tagged with your `subscription_id` is the
confirmation.
