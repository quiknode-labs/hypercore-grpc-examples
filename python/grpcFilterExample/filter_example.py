# Filtering Example - Stream only trades for specific coins
import grpc
import json
import time
import zstandard as zstd

import hyperliquid_pb2 as pb
import hyperliquid_pb2_grpc as pb_grpc

GRPC_ENDPOINT = "your-endpoint.hype-mainnet.quiknode.pro:10000"
AUTH_TOKEN = "your-auth-token"

decompressor = zstd.ZstdDecompressor()


def decompress(data: bytes) -> str:
    if data and len(data) >= 4 and data[0:4] == b'\x28\xB5\x2F\xFD':
        return decompressor.decompress(data).decode('utf-8')
    return data.decode('utf-8') if isinstance(data, bytes) else str(data)


def stream_with_filter():
    credentials = grpc.ssl_channel_credentials()
    options = [('grpc.max_receive_message_length', 100 * 1024 * 1024)]
    channel = grpc.secure_channel(GRPC_ENDPOINT, credentials, options)
    stub = pb_grpc.StreamingStub(channel)

    metadata = [('x-token', AUTH_TOKEN)]

    def request_generator():
        # Subscribe to TRADES with filters
        subscribe = pb.StreamSubscribe(
            stream_type=pb.StreamType.TRADES,
            start_block=0,
            filter_name='eth-btc-trades'
        )
        # Filter for specific coins only
        subscribe.filters['coin'].values.extend(['ETH', 'BTC'])

        yield pb.SubscribeRequest(subscribe=subscribe)

        # Keep-alive pings
        while True:
            time.sleep(30)
            yield pb.SubscribeRequest(ping=pb.Ping(timestamp=int(time.time() * 1000)))

    print('Streaming TRADES filtered by coin: ETH, BTC\n')

    try:
        for response in stub.StreamData(request_generator(), metadata=metadata):
            if response.HasField('data'):
                decompressed = decompress(response.data.data)
                try:
                    parsed = json.loads(decompressed)
                    print(f"Block {response.data.block_number}:")
                    print(json.dumps(parsed, indent=2))
                except json.JSONDecodeError:
                    print(decompressed)
    except KeyboardInterrupt:
        print("\nStopping...")
    finally:
        channel.close()


if __name__ == "__main__":
    stream_with_filter()
