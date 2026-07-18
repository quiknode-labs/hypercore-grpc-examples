#!/usr/bin/env python3
"""Validate server-side coin filtering on raw Hyperliquid MEMPOOL_TXS.

The payload does not contain a top-level ``coin`` field. The gRPC service resolves coin
names dynamically and matches numeric asset IDs across every order-touching
action, while returning the original raw tuple/object unchanged.
"""

from __future__ import annotations

import argparse
import json
import os
from pathlib import Path
import sys
import time
from typing import Any


DEFAULT_ENDPOINT = "your-endpoint.hype-mainnet.quiknode.pro:10000"
DEFAULT_TOKEN = "YOUR_QUICKNODE_TOKEN"
ORDER_TOUCHING_TYPES = {
    "order",
    "cancel",
    "cancelByCloid",
    "batchModify",
    "modify",
    "twapOrder",
    "twapCancel",
}
HEARTBEAT_SECONDS = 30


def signed_actions(value: Any) -> list[dict[str, Any]]:
    tx = value[1] if isinstance(value, list) and len(value) > 1 else value
    if not isinstance(tx, dict) or not isinstance(tx.get("signed_actions"), list):
        return []
    return [item for item in tx["signed_actions"] if isinstance(item, dict)]


def asset_id(value: Any) -> str | None:
    if isinstance(value, bool):
        return None
    if isinstance(value, int) and value >= 0:
        return str(value)
    if isinstance(value, str) and value.isascii() and value.isdigit():
        return value
    return None


def direct_assets(value: Any) -> list[str]:
    if not isinstance(value, dict):
        return []
    return [asset for field in ("a", "asset") if (asset := asset_id(value.get(field))) is not None]


def order_touching_actions(value: Any) -> list[dict[str, Any]]:
    matches: list[dict[str, Any]] = []
    for signed_action in signed_actions(value):
        action = signed_action.get("action")
        if not isinstance(action, dict):
            continue
        action_type = action.get("type")
        if action_type not in ORDER_TOUCHING_TYPES:
            continue

        assets: list[str] = []
        if action_type == "order":
            assets.extend(
                asset
                for item in action.get("orders", [])
                for asset in direct_assets(item)
            )
        elif action_type in {"cancel", "cancelByCloid"}:
            assets.extend(
                asset
                for item in action.get("cancels", [])
                for asset in direct_assets(item)
            )
        elif action_type == "batchModify":
            for item in action.get("modifies", []):
                assets.extend(direct_assets(item.get("order")) if isinstance(item, dict) else [])
                assets.extend(direct_assets(item))
        elif action_type == "modify":
            assets.extend(direct_assets(action.get("order")))
            assets.extend(direct_assets(action))
        elif action_type == "twapOrder":
            assets.extend(direct_assets(action.get("twap")))
        elif action_type == "twapCancel":
            assets.extend(direct_assets(action))

        if assets:
            matches.append({"type": action_type, "asset_ids": list(dict.fromkeys(assets))})
    return matches


def order_touching_asset_ids(value: Any) -> list[str]:
    return [asset for action in order_touching_actions(value) for asset in action["asset_ids"]]


def decode_data(data: Any) -> str:
    if isinstance(data, str):
        try:
            raw = data.encode("latin-1")
        except UnicodeEncodeError:
            return data
        if raw.startswith(bytes((0x28, 0xB5, 0x2F, 0xFD))):
            import zstandard  # Imported lazily so extractor unit tests need no native dependency.

            return zstandard.ZstdDecompressor().decompress(raw).decode()
        return data
    raw = bytes(data)
    if raw.startswith(bytes((0x28, 0xB5, 0x2F, 0xFD))):
        import zstandard  # Imported lazily so extractor unit tests need no native dependency.

        return zstandard.ZstdDecompressor().decompress(raw).decode()
    return raw.decode()


def protobuf_modules():
    generated = Path(__file__).resolve().parents[1] / "grpcRawDataExample"
    sys.path.insert(0, str(generated))
    import hyperliquid_pb2 as pb
    import hyperliquid_pb2_grpc as pb_grpc

    return pb, pb_grpc


def ping_request(pb: Any, timestamp: int):
    return pb.SubscribeRequest(ping=pb.Ping(timestamp=timestamp))


def request_stream(pb: Any, filters: dict[str, list[str]]):
    subscribe = pb.StreamSubscribe(
        stream_type=pb.MEMPOOL_TXS,
        start_block=0,
        filter_name="mempool-unfiltered-sample" if not filters else "mempool-coin-filter",
    )
    for field, values in filters.items():
        subscribe.filters[field].values.extend(values)
    yield pb.SubscribeRequest(subscribe=subscribe)
    while True:
        time.sleep(HEARTBEAT_SECONDS)
        timestamp = int(time.time() * 1000)
        print(f"PING timestamp={timestamp}")
        yield ping_request(pb, timestamp)


def run(args: argparse.Namespace) -> int:
    import grpc

    pb, pb_grpc = protobuf_modules()
    endpoint = os.getenv("GRPC_ENDPOINT", DEFAULT_ENDPOINT)
    token = os.getenv("AUTH_TOKEN", os.getenv("QN_AUTH_TOKEN", DEFAULT_TOKEN))
    if endpoint == DEFAULT_ENDPOINT or token == DEFAULT_TOKEN:
        raise RuntimeError("Set GRPC_ENDPOINT and AUTH_TOKEN (or QN_AUTH_TOKEN) before running")

    filters = {} if args.unfiltered else {args.filter_field: args.coin}
    expected = set(args.expected_asset_ids)
    options = (
        ("grpc.max_receive_message_length", 100 * 1024 * 1024),
        ("grpc.keepalive_time_ms", 30_000),
        ("grpc.keepalive_timeout_ms", 10_000),
    )
    channel = grpc.secure_channel(endpoint, grpc.ssl_channel_credentials(), options)
    stub = pb_grpc.StreamingStub(channel)
    received = 0
    try:
        responses = stub.StreamData(
            request_stream(pb, filters),
            metadata=(("x-token", token),),
            timeout=args.timeout_seconds,
        )
        for response in responses:
            if response.HasField("pong"):
                print(f"PONG timestamp={response.pong.timestamp}")
                continue
            if not response.HasField("data"):
                continue
            if args.expect_no_match:
                raise RuntimeError("server returned a transaction for the deliberately non-matching coin")

            text = decode_data(response.data.data)
            value = json.loads(text)
            actions = order_touching_actions(value)
            observed = order_touching_asset_ids(value)
            matches = sorted(expected.intersection(observed))
            if not args.unfiltered and not matches:
                raise RuntimeError(f"server returned a raw transaction without an expected asset: {observed}")

            received += 1
            summary = ", ".join(
                f"{action['type']}:{'|'.join(action['asset_ids'])}" for action in actions
            )
            print(
                f"message {received}/{args.max_messages}: "
                f"expected_asset_matches={matches} order_touching=[{summary}] bytes={len(text.encode())}"
            )
            if args.print_raw:
                print(text)
            if received >= args.max_messages:
                print(f"PASS: received {received} raw mempool message(s)")
                return 0
    except grpc.RpcError as error:
        if args.expect_no_match and error.code() == grpc.StatusCode.DEADLINE_EXCEEDED:
            print(f"PASS: no MEMPOOL_TXS messages matched within {args.timeout_seconds}s")
            return 0
        raise
    finally:
        channel.close()

    raise RuntimeError(f"stream ended after {received} message(s)")


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--coin", action="append", default=None, help="Coin name; repeat for OR semantics (default: BTC)")
    parser.add_argument("--filter-field", default="coin", choices=("coin", "coins"))
    parser.add_argument("--expected-asset-ids", default="0", help="Numeric IDs used only to validate returned raw data")
    parser.add_argument("--max-messages", type=int, default=5)
    parser.add_argument("--timeout-seconds", type=int, default=60)
    parser.add_argument("--unfiltered", action="store_true")
    parser.add_argument("--expect-no-match", action="store_true")
    parser.add_argument("--print-raw", action="store_true")
    args = parser.parse_args()
    args.coin = args.coin or ["BTC"]
    args.expected_asset_ids = [part.strip() for part in args.expected_asset_ids.split(",") if part.strip()]
    if args.max_messages <= 0 or args.timeout_seconds <= 0:
        parser.error("--max-messages and --timeout-seconds must be positive")
    if not args.expected_asset_ids or any(not part.isdigit() for part in args.expected_asset_ids):
        parser.error("--expected-asset-ids must contain non-negative integers")
    if args.unfiltered and args.expect_no_match:
        parser.error("--unfiltered and --expect-no-match cannot be combined")
    return args


if __name__ == "__main__":
    try:
        raise SystemExit(run(parse_args()))
    except Exception as error:  # The example should fail with a concise bounded diagnostic.
        print(f"FAILED: {error}", file=sys.stderr)
        raise SystemExit(1)
