#!/usr/bin/env python3
"""
S3 Blocks Backfill Example
==========================

WHY THIS EXISTS:
----------------
The `blocks` stream contains raw HyperLiquid replica commands - the fundamental
building blocks of the chain. Unlike other streams (trades, orders, book_updates),
blocks are ONLY available via gRPC streaming.

The problem: gRPC connections can disconnect. When that happens, you miss blocks.
There's no HTTP API to query historical blocks.

The solution: The Hyperliquid Foundation maintains a public S3 bucket with
historical data. You can backfill missed blocks directly from there.

This script is NOT a production-ready backfill system. It's a documented example
showing how to access and parse the S3 data so you can build your own ETL.


S3 BUCKET STRUCTURE:
--------------------
Bucket: s3://hl-mainnet-node-data/
Access: Requester pays (you pay for data transfer)

Available prefixes:
  - replica_cmds/          <- blocks data
  
replica_cmds/ directory structure:
  replica_cmds/{CHECKPOINT_TIMESTAMP}/{DATE}/{BLOCK_RANGE_FILE}

  Example:
  replica_cmds/1704067200/20240101/830000000-830010000

  Where:
  - CHECKPOINT_TIMESTAMP: Unix timestamp directory
  - DATE: YYYYMMDD format
  - BLOCK_RANGE_FILE: Contains ~10,000 blocks per file


FILE FORMAT:
------------
- JSON Lines format (one JSON object per line)
- NO block_number field in the JSON (ordering is implicit by line position)
- Files are MASSIVE: 3-7 GB each (uncompressed)
- Each line is a complete replica command


USAGE:
------
pip install boto3
python s3_blocks_backfill.py


COST CONSIDERATIONS:
--------------------
- Requester pays bucket - you pay for data transfer
- Files are 3-7 GB each
- Consider S3 Select for partial reads
- Stream instead of downloading entirely when possible
"""

import json
from dataclasses import dataclass
from typing import Iterator, Optional

import boto3

S3_BUCKET = "hl-mainnet-node-data"
BLOCKS_PREFIX = "replica_cmds"

# Requester pays - you need valid AWS credentials
s3 = boto3.client("s3")


@dataclass
class BlockRange:
    """Represents a block range file in S3."""
    checkpoint: str
    date: str
    start_block: int
    end_block: int
    s3_key: str

    @classmethod
    def from_s3_key(cls, key: str) -> Optional["BlockRange"]:
        """Parse S3 key: replica_cmds/1704067200/20240101/830000000-830010000"""
        parts = key.split("/")
        if len(parts) != 4 or parts[0] != BLOCKS_PREFIX:
            return None
        try:
            start, end = parts[3].split("-")
            return cls(parts[1], parts[2], int(start), int(end), key)
        except (ValueError, AttributeError):
            return None


def list_s3(prefix: str) -> list[str]:
    """List S3 objects under a prefix."""
    try:
        response = s3.list_objects_v2(
            Bucket=S3_BUCKET,
            Prefix=prefix,
            Delimiter="/",
            RequestPayer="requester"
        )
        items = []

        # Directories
        for p in response.get("CommonPrefixes", []):
            name = p["Prefix"].replace(prefix, "").rstrip("/")
            if name:
                items.append(name)

        # Files
        for obj in response.get("Contents", []):
            name = obj["Key"].replace(prefix, "")
            if name:
                items.append(name)

        return sorted(items)
    except Exception as e:
        print(f"S3 error: {e}")
        return []


def find_block_file(target_block: int) -> Optional[BlockRange]:
    """Find which S3 file contains a specific block number."""
    checkpoints = list_s3(f"{BLOCKS_PREFIX}/")
    if not checkpoints:
        return None

    checkpoint = checkpoints[-1]
    dates = list_s3(f"{BLOCKS_PREFIX}/{checkpoint}/")

    for date in dates:
        files = list_s3(f"{BLOCKS_PREFIX}/{checkpoint}/{date}/")
        for f in files:
            br = BlockRange.from_s3_key(f"{BLOCKS_PREFIX}/{checkpoint}/{date}/{f}")
            if br and br.start_block <= target_block <= br.end_block:
                return br
    return None


def stream_blocks(block_range: BlockRange) -> Iterator[dict]:
    """
    Stream blocks from S3.

    Files are 3-7 GB - this streams line-by-line to avoid loading into memory.
    Yields dict with 'block_number' (computed) and 'data' (parsed JSON).
    """
    response = s3.get_object(
        Bucket=S3_BUCKET,
        Key=block_range.s3_key,
        RequestPayer="requester"
    )

    line_number = 0
    for line in response["Body"].iter_lines():
        if line.strip():
            try:
                yield {
                    "block_number": block_range.start_block + line_number,
                    "data": json.loads(line)
                }
            except json.JSONDecodeError:
                pass
            line_number += 1


if __name__ == "__main__":
    print(__doc__)
    print("=" * 60)
    print("DISCOVERING S3 STRUCTURE")
    print("=" * 60 + "\n")

    checkpoints = list_s3(f"{BLOCKS_PREFIX}/")
    print(f"Checkpoints: {checkpoints}")

    if checkpoints:
        dates = list_s3(f"{BLOCKS_PREFIX}/{checkpoints[-1]}/")
        print(f"Dates in latest checkpoint: {dates[:5]}...")

    # Example: find a block (commented to avoid S3 charges)
    #
    # br = find_block_file(830_000_000)
    # if br:
    #     print(f"Found in {br.s3_key}")
    #     for block in stream_blocks(br):
    #         if block["block_number"] == 830_000_000:
    #             print(block)
    #             break
