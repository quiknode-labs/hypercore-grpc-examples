import grpc
import json
import sys
import time
import zstandard as zstd

# Generate proto files first: python -m grpc_tools.protoc -I../proto --python_out=. --grpc_python_out=. ../proto/hyperliquid.proto
import hyperliquid_pb2 as pb
import hyperliquid_pb2_grpc as pb_grpc

# Configuration
GRPC_ENDPOINT = "your-endpoint.hype-mainnet.quiknode.pro:10000"
AUTH_TOKEN = "your-auth-token"

# Zstd decompressor
decompressor = zstd.ZstdDecompressor()

def decompress(data: bytes) -> str:
    """Decompress zstd data if compressed, otherwise decode as utf-8."""
    if not data or len(data) < 4:
        return data.decode('utf-8') if isinstance(data, bytes) else str(data)

    # Check zstd magic number: 0x28 0xB5 0x2F 0xFD
    if data[0:4] == b'\x28\xB5\x2F\xFD':
        return decompressor.decompress(data).decode('utf-8')
    return data.decode('utf-8')


def create_channel():
    """Create a secure gRPC channel with authentication."""
    credentials = grpc.ssl_channel_credentials()
    options = [
        ('grpc.max_receive_message_length', 100 * 1024 * 1024),
        ('grpc.keepalive_time_ms', 30000),
        ('grpc.keepalive_timeout_ms', 10000),
    ]
    return grpc.secure_channel(GRPC_ENDPOINT, credentials, options)


def stream_data(stream_type: str, filters: dict = None):
    """Stream data from Hyperliquid with optional filters."""
    channel = create_channel()
    stub = pb_grpc.StreamingStub(channel)

    # Auth metadata
    metadata = [('x-token', AUTH_TOKEN)]

    def request_generator():
        # Send subscription request
        subscribe = pb.StreamSubscribe(
            stream_type=pb.StreamType.Value(stream_type),
            start_block=0
        )

        # Add filters if provided
        if filters:
            for field, values in filters.items():
                subscribe.filters[field].values.extend(values if isinstance(values, list) else [values])
            print(f"Filters applied: {filters}")

        yield pb.SubscribeRequest(subscribe=subscribe)

        # Keep-alive pings every 30s
        while True:
            time.sleep(30)
            yield pb.SubscribeRequest(ping=pb.Ping(timestamp=int(time.time() * 1000)))

    print(f"Streaming {stream_type}...")

    try:
        for response in stub.StreamData(request_generator(), metadata=metadata):
            if response.HasField('data'):
                data = response.data
                decompressed = decompress(data.data)

                try:
                    parsed = json.loads(decompressed)
                    print(f"\nBlock {data.block_number} | Timestamp {data.timestamp}")
                    print(json.dumps(parsed, indent=2))
                except json.JSONDecodeError:
                    print(f"Block {data.block_number}: {decompressed}")

            elif response.HasField('pong'):
                print(f"Pong: {response.pong.timestamp}")

    except grpc.RpcError as e:
        print(f"Error: {e.code()} - {e.details()}")
    except KeyboardInterrupt:
        print("\nStopping stream...")
    finally:
        channel.close()


def main():
    stream_type = sys.argv[1].upper() if len(sys.argv) > 1 else "TRADES"
    filters = {}

    # Parse --filter args: --filter coin=ETH,BTC
    args = sys.argv[2:]
    for i, arg in enumerate(args):
        if arg == '--filter' and i + 1 < len(args):
            field, values = args[i + 1].split('=')
            filters[field] = values.split(',')

    stream_data(stream_type, filters)


if __name__ == "__main__":
    main()
