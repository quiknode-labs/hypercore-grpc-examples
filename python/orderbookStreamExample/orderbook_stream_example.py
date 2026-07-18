#!/usr/bin/env python3
"""
Orderbook Stream Example - Stream Hyperliquid orderbook data via QuickNode gRPC.

Setup:
    pip install -r requirements.txt
    python -m grpc_tools.protoc -I../../proto --python_out=. --grpc_python_out=. ../../proto/orderbook.proto

Required environment:
    export GRPC_ENDPOINT="your-endpoint.hype-mainnet.quiknode.pro:10000"
    export AUTH_TOKEN="YOUR_QUICKNODE_TOKEN"
"""

import argparse
import json
import os
import sys
import time
from typing import Callable, Iterable, Optional

import grpc

try:
    import orderbook_pb2 as pb
    import orderbook_pb2_grpc as pb_grpc
except ImportError:
    print("Error: Proto files not generated. Run:")
    print("  python -m grpc_tools.protoc -I../../proto --python_out=. --grpc_python_out=. ../../proto/orderbook.proto")
    sys.exit(1)

GRPC_ENDPOINT = os.environ.get("GRPC_ENDPOINT", "your-endpoint.hype-mainnet.quiknode.pro:10000")
AUTH_TOKEN = os.environ.get("AUTH_TOKEN") or os.environ.get("QN_AUTH_TOKEN") or "your-quicknode-token"
MAX_RETRIES = 10
BASE_DELAY_SECONDS = 2


def create_channel() -> grpc.Channel:
    return grpc.secure_channel(
        GRPC_ENDPOINT,
        grpc.ssl_channel_credentials(),
        options=[
            ("grpc.max_receive_message_length", 100 * 1024 * 1024),
            ("grpc.keepalive_time_ms", 30000),
        ],
    )


def metadata():
    return [("x-token", AUTH_TOKEN)]


def level_text(level) -> str:
    if not level or not level.px:
        return "n/a"
    return f"{level.px} / {level.sz} ({level.n})"


def enum_name(enum_type, value: int) -> str:
    try:
        return enum_type.Name(value)
    except ValueError:
        return str(value)


def l4_snapshot_reset_kind(snapshot_count: int) -> str:
    if isinstance(snapshot_count, bool) or not isinstance(snapshot_count, int) or snapshot_count < 1:
        raise ValueError("snapshot_count must be a positive integer")
    return "initial" if snapshot_count == 1 else "replacement"


def l2_request(args) -> pb.L2BookRequest:
    request = pb.L2BookRequest(coin=args.coin, n_levels=args.levels)
    if args.sig_figs is not None:
        request.n_sig_figs = args.sig_figs
    if args.mantissa is not None:
        request.mantissa = args.mantissa
    return request


def l2_diff_request(args) -> pb.L2BookDiffRequest:
    request = pb.L2BookDiffRequest(
        coins=args.coins,
        n_levels=args.levels,
        skip_initial_snapshot=args.skip_initial_snapshot,
    )
    if args.sig_figs is not None:
        request.n_sig_figs = args.sig_figs
    if args.mantissa is not None:
        request.mantissa = args.mantissa
    return request


def consume_with_reconnect(
    label: str,
    make_stream: Callable[[pb_grpc.OrderBookStreamingStub], Iterable],
    handle_update: Callable[[object, int], None],
    max_messages: Optional[int],
):
    msg_count = 0
    data_loss_count = 0

    while data_loss_count < MAX_RETRIES:
        if data_loss_count > 0:
            delay = BASE_DELAY_SECONDS * (2 ** (data_loss_count - 1))
            print(f"Reconnecting {label} after DATA_LOSS in {delay}s (attempt {data_loss_count + 1}/{MAX_RETRIES})")
            time.sleep(delay)

        channel = create_channel()
        stub = pb_grpc.OrderBookStreamingStub(channel)

        try:
            for update in make_stream(stub):
                data_loss_count = 0
                msg_count += 1
                handle_update(update, msg_count)

                if max_messages and msg_count >= max_messages:
                    return

            return
        except grpc.RpcError as exc:
            if exc.code() == grpc.StatusCode.DATA_LOSS:
                data_loss_count += 1
                channel.close()
                continue
            raise
        finally:
            channel.close()

    raise RuntimeError(f"{label} exceeded max reconnect attempts")


def stream_l2(args):
    consume_with_reconnect(
        "StreamL2Book",
        lambda stub: stub.StreamL2Book(l2_request(args), metadata=metadata()),
        lambda update, count: print(
            f"[{count}] L2 {update.coin} block={update.block_number} "
            f"bid={level_text(update.bids[0] if update.bids else None)} "
            f"ask={level_text(update.asks[0] if update.asks else None)} "
            f"bids={len(update.bids)} asks={len(update.asks)}"
        ),
        args.max_messages,
    )


def stream_l4(args):
    snapshot_count = 0

    def handle(update, count):
        nonlocal snapshot_count
        if update.HasField("snapshot"):
            snapshot_count += 1
            snapshot = update.snapshot
            reset = l4_snapshot_reset_kind(snapshot_count)
            print(
                f"[{count}] L4 snapshot {snapshot.coin} height={snapshot.height} "
                f"reset={reset} bids={len(snapshot.bids)} asks={len(snapshot.asks)}"
            )
            if reset == "replacement":
                print("  replace the entire local L4 book with this snapshot")
        elif update.HasField("diff"):
            try:
                data = json.loads(update.diff.data)
            except json.JSONDecodeError as exc:
                print(f"[{count}] L4 diff height={update.diff.height} invalid JSON: {exc}")
                return
            print(
                f"[{count}] L4 diff height={update.diff.height} "
                f"order_statuses={len(data.get('order_statuses', []))} "
                f"book_diffs={len(data.get('book_diffs', []))}"
            )

    consume_with_reconnect(
        "StreamL4Book",
        lambda stub: stub.StreamL4Book(pb.L4BookRequest(coin=args.coin), metadata=metadata()),
        handle,
        args.max_messages,
    )


def stream_bbo(args):
    consume_with_reconnect(
        "StreamBboBook",
        lambda stub: stub.StreamBboBook(pb.BboBookRequest(coins=args.coins), metadata=metadata()),
        lambda update, count: print(
            f"[{count}] BBO {update.coin} block={update.block_number} "
            f"bid={level_text(update.bid if update.HasField('bid') else None)} "
            f"ask={level_text(update.ask if update.HasField('ask') else None)}"
        ),
        args.max_messages,
    )


def stream_l2_diff(args):
    def handle(update, count):
        print(f"[{count}] L2 diff height={update.height} snapshot={update.snapshot} coins={len(update.diffs)}")
        for diff in list(update.diffs)[:5]:
            print(
                f"  {diff.coin} seq={diff.seq} prev_seq={diff.prev_seq} "
                f"snapshot={diff.snapshot} bid_changes={len(diff.bids)} ask_changes={len(diff.asks)}"
            )

    consume_with_reconnect(
        "StreamL2BookDiff",
        lambda stub: stub.StreamL2BookDiff(l2_diff_request(args), metadata=metadata()),
        handle,
        args.max_messages,
    )


def stream_l4_updates(args):
    def handle(update, count):
        print(f"[{count}] L4 updates height={update.height} snapshot={update.snapshot} diffs={len(update.diffs)}")
        if update.snapshot:
            print("  clear local L4 order state before applying this update")
        for diff in list(update.diffs)[:5]:
            print(
                f"  {enum_name(pb.L4OrderDiffType, diff.diff_type)} {diff.coin} "
                f"oid={diff.oid} side={diff.side or 'n/a'} px={diff.px or 'n/a'} sz={diff.sz or 'n/a'}"
            )

    consume_with_reconnect(
        "StreamL4BookUpdates",
        lambda stub: stub.StreamL4BookUpdates(pb.L4BookUpdatesRequest(coins=args.coins), metadata=metadata()),
        handle,
        args.max_messages,
    )


def stream_tpsl(args):
    def handle(update, count):
        print(f"[{count}] TP/SL height={update.height} snapshot={update.snapshot} diffs={len(update.diffs)}")
        for diff in list(update.diffs)[:5]:
            print(
                f"  {enum_name(pb.TpslDiffType, diff.diff_type)} {diff.coin} "
                f"oid={diff.oid} trigger={diff.trigger_px or 'n/a'} "
                f"limit={diff.limit_px or 'n/a'} sz={diff.sz or 'n/a'} reason={diff.reason or 'n/a'}"
            )

    consume_with_reconnect(
        "StreamTpslUpdates",
        lambda stub: stub.StreamTpslUpdates(pb.TpslUpdatesRequest(coins=args.coins), metadata=metadata()),
        handle,
        args.max_messages,
    )


def parse_args():
    parser = argparse.ArgumentParser(description="Stream Hyperliquid orderbook data via QuickNode gRPC")
    parser.add_argument("--mode", choices=["l2", "l4", "bbo", "l2-diff", "l4-updates", "tpsl"], default="bbo")
    parser.add_argument("--coin", default="BTC", help="Coin symbol or comma-separated symbols")
    parser.add_argument("--all", action="store_true", help="Subscribe to all eligible coins on multi-coin streams")
    parser.add_argument("--levels", type=int, default=20, help="Number of L2 levels (default 20, max 100)")
    parser.add_argument("--sig-figs", type=int, default=None, help="L2 bucketing significant figures (2-5)")
    parser.add_argument("--mantissa", type=int, default=None, help="L2 bucketing mantissa (1, 2, or 5)")
    parser.add_argument("--skip-initial-snapshot", action="store_true", help="For l2-diff, skip the initial snapshot")
    parser.add_argument("--max-messages", type=int, default=None, help="Stop after N messages")
    args = parser.parse_args()

    if args.all and args.mode in {"l2", "l4"}:
        parser.error("--all is only supported for bbo, l2-diff, l4-updates, and tpsl. Use --coin for l2 or l4.")

    args.coins = [] if args.all else [coin.strip() for coin in args.coin.split(",") if coin.strip()]
    if not args.all and not args.coins:
        parser.error("--coin must include at least one symbol. Use --all to subscribe to every eligible coin on multi-coin streams.")

    args.coin = args.coins[0] if args.coins else ""
    return args


def coin_display(args) -> tuple[str, str]:
    if args.mode in {"l2", "l4"}:
        return "Coin", args.coin
    return "Coins", ",".join(args.coins) if args.coins else "all eligible coins"


def main():
    args = parse_args()

    print("Hyperliquid Orderbook Stream Example")
    print(f"Endpoint: {GRPC_ENDPOINT}")
    print(f"Mode: {args.mode}")
    coin_label, coin_value = coin_display(args)
    print(f"{coin_label}: {coin_value}")

    if AUTH_TOKEN == "your-quicknode-token":
        print("Set AUTH_TOKEN to your QuickNode token before running this example.")
        sys.exit(1)

    if args.mode == "l2":
        stream_l2(args)
    elif args.mode == "l4":
        stream_l4(args)
    elif args.mode == "bbo":
        stream_bbo(args)
    elif args.mode == "l2-diff":
        stream_l2_diff(args)
    elif args.mode == "l4-updates":
        stream_l4_updates(args)
    elif args.mode == "tpsl":
        stream_tpsl(args)


if __name__ == "__main__":
    main()
