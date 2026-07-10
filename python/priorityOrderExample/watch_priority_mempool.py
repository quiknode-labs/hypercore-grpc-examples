#!/usr/bin/env python3
"""
Watch live priority-fee activity from a QuickNode Hyperliquid gRPC endpoint.

By default this example subscribes to the derived ORDER_PRIORITY stream and filters
for source=mempool_txs. These are pre-consensus mempool signals, not finalized
orders. Use them to see priority-fee order flow before it lands on-chain.
"""

import argparse
import json
import os
import sys
import time

import grpc
import zstandard as zstd

try:
    import hyperliquid_pb2 as pb
    import hyperliquid_pb2_grpc as pb_grpc
except ImportError:
    print("Error: Proto files not generated. Run:")
    print("  python -m grpc_tools.protoc -I../../proto --python_out=. --grpc_python_out=. ../../proto/hyperliquid.proto")
    sys.exit(1)


DEFAULT_GRPC_ENDPOINT = "your-endpoint.hype-mainnet.quiknode.pro:10000"
DEFAULT_AUTH_TOKEN = "YOUR_QUICKNODE_TOKEN"


def env_or_default(name: str, default: str) -> str:
    value = os.environ.get(name)
    return value if value else default


GRPC_ENDPOINT = env_or_default("GRPC_ENDPOINT", DEFAULT_GRPC_ENDPOINT)
AUTH_TOKEN = env_or_default("AUTH_TOKEN", env_or_default("QN_AUTH_TOKEN", DEFAULT_AUTH_TOKEN))
decompressor = zstd.ZstdDecompressor()
ZSTD_MAGIC = b"\x28\xB5\x2F\xFD"


def string_has_zstd_magic(data: str) -> bool:
    if len(data) < 4:
        return False
    try:
        return bytes(ord(char) for char in data[:4]) == ZSTD_MAGIC
    except ValueError:
        return False


def decompress(data) -> str:
    if isinstance(data, str):
        if string_has_zstd_magic(data):
            return decompressor.decompress(data.encode("latin-1")).decode("utf-8")
        return data
    if not data or len(data) < 4:
        return data.decode("utf-8") if isinstance(data, bytes) else str(data)
    if data[0:4] == ZSTD_MAGIC:
        return decompressor.decompress(data).decode("utf-8")
    return data.decode("utf-8")


def create_channel():
    return grpc.secure_channel(
        GRPC_ENDPOINT,
        grpc.ssl_channel_credentials(),
        options=[
            ("grpc.max_receive_message_length", 100 * 1024 * 1024),
            ("grpc.keepalive_time_ms", 30000),
            ("grpc.keepalive_timeout_ms", 10000),
        ],
    )


def request_generator(start_block: int, raw_mempool: bool, include_confirmed: bool):
    stream_type = "MEMPOOL_TXS" if raw_mempool else "ORDER_PRIORITY"
    filters = {}
    if not raw_mempool and not include_confirmed:
        filters["source"] = pb.FilterValues(values=["mempool_txs"])

    yield pb.SubscribeRequest(
        subscribe=pb.StreamSubscribe(
            stream_type=pb.StreamType.Value(stream_type),
            start_block=start_block,
            filters=filters,
        )
    )

    while True:
        time.sleep(30)
        yield pb.SubscribeRequest(ping=pb.Ping(timestamp=int(time.time() * 1000)))


def text_matches_filters(text: str, needles: list[str]) -> bool:
    return not needles or any(needle in text for needle in needles)


def priority_fees(value) -> list[str]:
    fees = []
    if isinstance(value, dict):
        if value.get("source") and value.get("type") == "order" and "p" in value:
            fees.append(str(value["p"]))
        grouping = value.get("grouping")
        if isinstance(grouping, dict) and "p" in grouping:
            fees.append(str(grouping["p"]))
        for item in value.values():
            fees.extend(priority_fees(item))
    elif isinstance(value, list):
        for item in value:
            fees.extend(priority_fees(item))
    return fees


def parse_json(text: str):
    try:
        return json.loads(text)
    except json.JSONDecodeError:
        return None


def print_payload(block_number: int, timestamp: int, text: str, compact: bool, fees: list[str]):
    print(f"\nBlock {block_number} | Timestamp {timestamp}")
    if fees:
        print(f"Priority p: {', '.join(fees)}")
    if compact:
        print(text[:1000])
        return

    parsed = parse_json(text)
    if parsed is not None:
        print(json.dumps(parsed, indent=2))
    else:
        print(text)


def main() -> int:
    parser = argparse.ArgumentParser(description="Watch pre-consensus Hyperliquid priority-fee mempool events from a QuickNode gRPC endpoint.")
    parser.add_argument("--start-block", type=int, default=0, help="Start block for the stream. Default 0.")
    parser.add_argument("--contains", action="append", default=[], help="Only print messages containing this text. Can be repeated.")
    parser.add_argument("--include-confirmed", action="store_true", help="Also include confirmed ORDER_PRIORITY events from replica_cmds.")
    parser.add_argument("--raw-mempool", action="store_true", help="Subscribe to raw MEMPOOL_TXS and detect grouping.p locally.")
    parser.add_argument("--all-mempool", action="store_true", help="With --raw-mempool, print all MEMPOOL_TXS messages, not only priority transactions.")
    parser.add_argument("--max-messages", type=int, help="Stop after printing this many matching messages.")
    parser.add_argument("--compact", action="store_true", help="Print compact payload text instead of pretty JSON.")
    args = parser.parse_args()
    raw_mempool = args.raw_mempool or args.all_mempool

    if GRPC_ENDPOINT == DEFAULT_GRPC_ENDPOINT:
        print("Set GRPC_ENDPOINT to your QuickNode Hyperliquid mainnet or testnet gRPC endpoint.", file=sys.stderr)
        return 2
    if AUTH_TOKEN == DEFAULT_AUTH_TOKEN:
        print("Set AUTH_TOKEN to your QuickNode token.", file=sys.stderr)
        return 2

    if raw_mempool:
        print("Watching raw MEMPOOL_TXS")
    elif args.include_confirmed:
        print("Watching ORDER_PRIORITY events from mempool_txs and replica_cmds")
    else:
        print("Watching pre-consensus ORDER_PRIORITY mempool events")
    print(f"Endpoint: {GRPC_ENDPOINT}")
    if not raw_mempool and not args.include_confirmed:
        print("Server filter: source=mempool_txs (not finalized)")
    elif raw_mempool and not args.all_mempool:
        print("Local filter: priority grouping only")
    if args.contains:
        print(f"Text filters: {args.contains}")

    channel = create_channel()
    stub = pb_grpc.StreamingStub(channel)
    metadata = [("x-token", AUTH_TOKEN)]
    printed = 0

    try:
        for response in stub.StreamData(request_generator(args.start_block, raw_mempool, args.include_confirmed), metadata=metadata):
            if response.HasField("pong"):
                continue
            if not response.HasField("data"):
                continue

            data = response.data
            text = decompress(data.data)
            if not text_matches_filters(text, args.contains):
                continue

            parsed = parse_json(text)
            fees = priority_fees(parsed) if parsed is not None else []
            if raw_mempool and not args.all_mempool and not fees:
                continue

            printed += 1
            print_payload(data.block_number, data.timestamp, text, args.compact, fees)
            if args.max_messages and printed >= args.max_messages:
                return 0
    except grpc.RpcError as exc:
        print(f"gRPC error: {exc.code()} - {exc.details()}", file=sys.stderr)
        return 1
    except KeyboardInterrupt:
        print("\nStopping stream...")
        return 0
    finally:
        channel.close()


if __name__ == "__main__":
    raise SystemExit(main())
