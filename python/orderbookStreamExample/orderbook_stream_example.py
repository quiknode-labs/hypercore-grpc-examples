#!/usr/bin/env python3
"""
Orderbook Stream Example - Stream L2 and L4 orderbook data via gRPC

This example demonstrates how to use the OrderBookStreaming service to:
1. StreamL2Book - Get aggregated price levels (L2 orderbook)
2. StreamL4Book - Get individual orders (L4 orderbook)

Setup:
    pip install grpcio grpcio-tools protobuf zstandard
    python -m grpc_tools.protoc -I../../proto --python_out=. --grpc_python_out=. ../../proto/orderbook.proto

Usage:
    # Stream L2 orderbook for BTC
    python orderbook_stream_example.py --mode l2 --coin BTC

    # Stream L4 orderbook for ETH
    python orderbook_stream_example.py --mode l4 --coin ETH

    # Stream both L2 and L4 for multiple coins
    python orderbook_stream_example.py --mode both --coin BTC,ETH
"""

import grpc
import json
import sys
import time
import argparse
import threading
from typing import Optional

try:
    import orderbook_pb2 as pb
    import orderbook_pb2_grpc as pb_grpc
except ImportError:
    print("Error: Proto files not generated. Run:")
    print("  python -m grpc_tools.protoc -I../../proto --python_out=. --grpc_python_out=. ../../proto/orderbook.proto")
    sys.exit(1)

# Configuration
GRPC_ENDPOINT = "your-endpoint.hype-mainnet.quiknode.pro:10000"
AUTH_TOKEN = "your-auth-token"


def stream_l2_orderbook(coin: str, n_levels: int = 20, n_sig_figs: Optional[int] = None, mantissa: Optional[int] = None, auto_reconnect: bool = True):
    """
    Stream L2 (aggregated) orderbook updates for a coin.

    Args:
        coin: Symbol to stream (e.g., "BTC", "ETH")
        n_levels: Number of price levels to display (default 20, max 100)
        n_sig_figs: Significance figures for price bucketing (2-5)
        mantissa: Mantissa for bucketing (1, 2, or 5)
        auto_reconnect: Auto-reconnect on DATA_LOSS errors (default True)
    """
    print(f"\n{'='*60}")
    print(f"Streaming L2 Orderbook for {coin}")
    print(f"Levels: {n_levels}")
    print(f"Auto-reconnect: {auto_reconnect}")
    print(f"{'='*60}\n")

    retry_count = 0
    max_retries = 10
    base_delay = 2

    while retry_count < max_retries:
        channel = grpc.secure_channel(
            GRPC_ENDPOINT,
            grpc.ssl_channel_credentials(),
            options=[
                ('grpc.max_receive_message_length', 100 * 1024 * 1024),
                ('grpc.keepalive_time_ms', 30000),
            ]
        )
        stub = pb_grpc.OrderBookStreamingStub(channel)

        # Build request
        request = pb.L2BookRequest(
            coin=coin,
            n_levels=n_levels
        )
        if n_sig_figs is not None:
            request.n_sig_figs = n_sig_figs
        if mantissa is not None:
            request.mantissa = mantissa

        msg_count = 0

        try:
            if retry_count > 0:
                print(f"\n🔄 Reconnecting (attempt {retry_count + 1}/{max_retries})...")
            else:
                print(f"Connecting to {GRPC_ENDPOINT}...")

            for update in stub.StreamL2Book(request, metadata=[('x-token', AUTH_TOKEN)]):
                msg_count += 1

                if msg_count == 1:
                    print(f"✓ First L2 update received!\n")
                    retry_count = 0  # Reset retry count on successful connection

                # Display the L2 orderbook
                print(f"\n{'─'*60}")
                print(f"Block: {update.block_number} | Time: {update.time} | Coin: {update.coin}")
                print(f"{'─'*60}")

                # Show asks (sorted highest to lowest for display)
                if update.asks:
                    print("\n  ASKS:")
                    for level in reversed(list(update.asks[:10])):  # Top 10 asks
                        print(f"    {level.px:>12} | {level.sz:>12} | ({level.n} orders)")

                # Show spread
                if update.bids and update.asks:
                    best_bid = float(update.bids[0].px) if update.bids else 0
                    best_ask = float(update.asks[0].px) if update.asks else 0
                    if best_bid and best_ask:
                        spread = best_ask - best_bid
                        spread_bps = (spread / best_bid) * 10000 if best_bid > 0 else 0
                        print(f"\n  {'─'*44}")
                        print(f"  SPREAD: {spread:.2f} ({spread_bps:.2f} bps)")
                        print(f"  {'─'*44}")

                # Show bids
                if update.bids:
                    print("\n  BIDS:")
                    for level in update.bids[:10]:  # Top 10 bids
                        print(f"    {level.px:>12} | {level.sz:>12} | ({level.n} orders)")

                print(f"\n  Messages received: {msg_count}")

        except grpc.RpcError as e:
            if e.code() == grpc.StatusCode.DATA_LOSS and auto_reconnect:
                print(f"\n⚠️  Server reinitialized: {e.details()}")
                retry_count += 1
                if retry_count < max_retries:
                    delay = base_delay * (2 ** (retry_count - 1))  # Exponential backoff
                    print(f"⏳ Waiting {delay}s before reconnecting...")
                    time.sleep(delay)
                    channel.close()
                    continue
                else:
                    print(f"\n❌ Max retries ({max_retries}) reached. Giving up.")
                    break
            else:
                print(f"\ngRPC error: {e.code()} - {e.details()}")
                break
        except KeyboardInterrupt:
            print("\nStopping L2 stream...")
            break
        finally:
            channel.close()

        # If we get here without error, break the retry loop
        break


def stream_l4_orderbook(coin: str, max_messages: Optional[int] = None, auto_reconnect: bool = True):
    """
    Stream L4 (individual orders) orderbook updates for a coin.

    Args:
        coin: Symbol to stream (e.g., "BTC", "ETH")
        max_messages: Maximum number of messages to receive (None for unlimited)
        auto_reconnect: Auto-reconnect on DATA_LOSS errors (default True)
    """
    print(f"\n{'='*60}")
    print(f"Streaming L4 Orderbook for {coin}")
    print(f"Auto-reconnect: {auto_reconnect}")
    print(f"{'='*60}\n")

    retry_count = 0
    max_retries = 10
    base_delay = 2
    total_msg_count = 0

    while retry_count < max_retries:
        channel = grpc.secure_channel(
            GRPC_ENDPOINT,
            grpc.ssl_channel_credentials(),
            options=[
                ('grpc.max_receive_message_length', 100 * 1024 * 1024),
                ('grpc.keepalive_time_ms', 30000),
            ]
        )
        stub = pb_grpc.OrderBookStreamingStub(channel)

        request = pb.L4BookRequest(coin=coin)

        msg_count = 0
        snapshot_received = False

        try:
            if retry_count > 0:
                print(f"\n🔄 Reconnecting (attempt {retry_count + 1}/{max_retries})...")
            else:
                print(f"Connecting to {GRPC_ENDPOINT}...")

            for update in stub.StreamL4Book(request, metadata=[('x-token', AUTH_TOKEN)]):
                msg_count += 1
                total_msg_count += 1

                if update.HasField('snapshot'):
                    snapshot = update.snapshot
                    snapshot_received = True
                    retry_count = 0  # Reset retry count on successful connection

                    print(f"\n✓ L4 Snapshot Received!")
                    print(f"{'─'*60}")
                    print(f"Coin: {snapshot.coin}")
                    print(f"Height: {snapshot.height}")
                    print(f"Time: {snapshot.time}")
                    print(f"Bids: {len(snapshot.bids)} orders")
                    print(f"Asks: {len(snapshot.asks)} orders")
                    print(f"{'─'*60}")

                    # Show sample of orders
                    if snapshot.bids:
                        print(f"\nSample Bids (first 5):")
                        for order in snapshot.bids[:5]:
                            print(f"  OID: {order.oid} | Price: {order.limit_px} | Size: {order.sz} | User: {order.user[:10]}...")

                    if snapshot.asks:
                        print(f"\nSample Asks (first 5):")
                        for order in snapshot.asks[:5]:
                            print(f"  OID: {order.oid} | Price: {order.limit_px} | Size: {order.sz} | User: {order.user[:10]}...")

                elif update.HasField('diff'):
                    diff = update.diff

                    if not snapshot_received:
                        print(f"\n⚠ Received diff before snapshot")

                    # Parse the JSON diff data
                    try:
                        diff_data = json.loads(diff.data)
                        order_statuses = diff_data.get('order_statuses', [])
                        book_diffs = diff_data.get('book_diffs', [])

                        print(f"\n[Block {diff.height}] L4 Diff:")
                        print(f"  Time: {diff.time}")
                        print(f"  Order Statuses: {len(order_statuses)}")
                        print(f"  Book Diffs: {len(book_diffs)}")

                        # Show sample diffs
                        if book_diffs and len(book_diffs) <= 5:
                            print(f"  Diffs: {json.dumps(book_diffs, indent=4)}")

                    except json.JSONDecodeError as e:
                        print(f"  Error parsing diff data: {e}")
                        print(f"  Raw data: {diff.data[:200]}...")

                if max_messages and total_msg_count >= max_messages:
                    print(f"\nReached max messages ({max_messages}), stopping...")
                    channel.close()
                    return

        except grpc.RpcError as e:
            if e.code() == grpc.StatusCode.DATA_LOSS and auto_reconnect:
                print(f"\n⚠️  Server reinitialized: {e.details()}")
                retry_count += 1
                if retry_count < max_retries:
                    delay = base_delay * (2 ** (retry_count - 1))  # Exponential backoff
                    print(f"⏳ Waiting {delay}s before reconnecting...")
                    time.sleep(delay)
                    channel.close()
                    continue
                else:
                    print(f"\n❌ Max retries ({max_retries}) reached. Giving up.")
                    break
            else:
                print(f"\ngRPC error: {e.code()} - {e.details()}")
                break
        except KeyboardInterrupt:
            print("\nStopping L4 stream...")
            break
        finally:
            channel.close()

        # If we get here without error, break the retry loop
        break


def stream_both(coins: list):
    """
    Stream both L2 and L4 orderbooks for multiple coins simultaneously.

    Args:
        coins: List of coin symbols to stream
    """
    print(f"\n{'='*60}")
    print(f"Streaming L2 and L4 for: {', '.join(coins)}")
    print(f"{'='*60}\n")

    threads = []

    # Start L2 streams
    for coin in coins:
        t = threading.Thread(target=stream_l2_orderbook, args=(coin, 10), daemon=True)
        t.start()
        threads.append(t)
        time.sleep(1)  # Stagger starts

    # Start L4 streams
    for coin in coins:
        t = threading.Thread(target=stream_l4_orderbook, args=(coin, 100), daemon=True)
        t.start()
        threads.append(t)
        time.sleep(1)

    print("\nAll streams started. Press Ctrl+C to stop.\n")

    try:
        for t in threads:
            t.join()
    except KeyboardInterrupt:
        print("\nStopping all streams...")


def main():
    parser = argparse.ArgumentParser(description='Stream Hyperliquid orderbook data via gRPC')
    parser.add_argument('--mode', choices=['l2', 'l4', 'both'], default='l2',
                        help='Streaming mode: l2 (aggregated), l4 (individual orders), or both')
    parser.add_argument('--coin', default='BTC',
                        help='Coin symbol(s) to stream (comma-separated for multiple)')
    parser.add_argument('--levels', type=int, default=20,
                        help='Number of price levels for L2 (default: 20, max: 100)')
    parser.add_argument('--max-messages', type=int, default=None,
                        help='Maximum number of messages to receive (L4 only)')

    args = parser.parse_args()

    coins = [c.strip() for c in args.coin.split(',')]

    print(f"\n{'='*60}")
    print("Hyperliquid Orderbook Stream Example")
    print(f"Endpoint: {GRPC_ENDPOINT}")
    print(f"{'='*60}")

    try:
        if args.mode == 'l2':
            if len(coins) == 1:
                stream_l2_orderbook(coins[0], n_levels=args.levels)
            else:
                print("Error: L2 mode only supports single coin. Use --mode both for multiple coins.")
                sys.exit(1)
        elif args.mode == 'l4':
            if len(coins) == 1:
                stream_l4_orderbook(coins[0], max_messages=args.max_messages)
            else:
                print("Error: L4 mode only supports single coin. Use --mode both for multiple coins.")
                sys.exit(1)
        elif args.mode == 'both':
            stream_both(coins)
    except Exception as e:
        print(f"\nError: {e}")
        import traceback
        traceback.print_exc()
        sys.exit(1)


if __name__ == "__main__":
    main()
