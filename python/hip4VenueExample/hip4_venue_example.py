# HIP-4 Outcome Markets Example - Stream one venue's orders with
# subscription tagging and signer enrichment.
#
# Demonstrates the three HIP-4-era streaming features:
#  1. "venue" filter key: server-side expansion of an outcome venue's name
#     to its full coin set (coins look like "#146870" and churn as outcomes
#     settle - the server tracks that for you).
#  2. subscription_id: a client-chosen tag echoed on every update, plus the
#     stream_type field, so multiplexed subscriptions are distinguishable.
#  3. enrichment.include_signer: each order carries "signer" - the wallet
#     that actually SUBMITTED it (master or approved API wallet), recovered
#     from the action's signature. Unsigned engine events (trigger fires,
#     liquidations, TWAP children) carry "signer": null.
#
# Find active venue names via the info endpoint: {"type":"outcomeMeta"}
# (fields: outcomes[].venue, deployers[].venue).
import grpc
import json
import sys
import time
from pathlib import Path

import zstandard as zstd

# Stubs are generated into python/ by `cd python && ./generate_proto.sh`;
# put that directory on sys.path so this runs from any working directory.
GENERATED_PROTO_DIR = str(Path(__file__).resolve().parents[1])
if GENERATED_PROTO_DIR not in sys.path:
    sys.path.insert(0, GENERATED_PROTO_DIR)

try:
    import hyperliquid_pb2 as pb
    import hyperliquid_pb2_grpc as pb_grpc
except ModuleNotFoundError as error:
    raise SystemExit(
        "Python protobuf stubs are missing; run `cd python && ./generate_proto.sh`"
    ) from error

# HIP-4 launches on testnet first; use your testnet endpoint until mainnet
# venues go live.
# Mainnet: "your-endpoint.hype-mainnet.quiknode.pro:10000"
# Testnet: "your-endpoint.hype-testnet.quiknode.pro:10000"
GRPC_ENDPOINT = "your-endpoint.hype-testnet.quiknode.pro:10000"
AUTH_TOKEN = "your-auth-token"
VENUE_NAME = "txyz"  # an active venue from {"type":"outcomeMeta"}

decompressor = zstd.ZstdDecompressor()


def decompress(data: bytes) -> str:
    if data and len(data) >= 4 and data[0:4] == b'\x28\xB5\x2F\xFD':
        return decompressor.decompress(data).decode('utf-8')
    return data.decode('utf-8') if isinstance(data, bytes) else str(data)


def stream_venue_orders():
    credentials = grpc.ssl_channel_credentials()
    options = [('grpc.max_receive_message_length', 100 * 1024 * 1024)]
    channel = grpc.secure_channel(GRPC_ENDPOINT, credentials, options)
    stub = pb_grpc.StreamingStub(channel)

    metadata = [('x-token', AUTH_TOKEN)]

    def request_generator():
        # Subscribe to ORDERS for one outcome venue, tagged and
        # signer-enriched.
        subscribe = pb.StreamSubscribe(
            stream_type=pb.StreamType.ORDERS,
            start_block=0,
            filter_name=f'hip4-{VENUE_NAME}',
            # Echoed on every update for this stream type.
            subscription_id='hip4-orders-demo',
            # Adds "signer" to each order (requires a server with signer
            # enrichment enabled; testnet has it on).
            enrichment=pb.EnrichmentOptions(include_signer=True),
        )
        # Reserved key: expanded server-side to the venue's coin set.
        # Also accepted: "venues", "deployer", "deployers" (address).
        subscribe.filters['venue'].values.extend([VENUE_NAME])

        yield pb.SubscribeRequest(subscribe=subscribe)

        # Keep-alive pings
        while True:
            time.sleep(30)
            yield pb.SubscribeRequest(ping=pb.Ping(timestamp=int(time.time() * 1000)))

    print(f'Streaming ORDERS for venue "{VENUE_NAME}" with signer enrichment\n')

    try:
        for response in stub.StreamData(request_generator(), metadata=metadata):
            if not response.HasField('data'):
                continue  # pong
            decompressed = decompress(response.data.data)

            # Every update says which subscription it belongs to.
            print(
                f"[block {response.data.block_number}] "
                f"streamType={pb.StreamType.Name(response.data.stream_type)} "
                f"subscriptionId=\"{response.data.subscription_id}\""
            )

            try:
                entries = json.loads(decompressed)
            except json.JSONDecodeError:
                print(decompressed)
                continue
            for entry in entries:
                order = entry.get('order') or {}
                coin = (order.get('order') or {}).get('coin')
                user = order.get('user')
                # "signer" is present because of enrichment above.
                print(
                    f"  coin={coin} user={user} "
                    f"signer={entry.get('signer')} status={entry.get('status')}"
                )
    except grpc.RpcError as e:
        print(f"Stream error: {e.code()}: {e.details()}")


if __name__ == '__main__':
    stream_venue_orders()
