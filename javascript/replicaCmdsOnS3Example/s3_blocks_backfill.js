/**
 * S3 Blocks Backfill Example
 * ==========================
 *
 * WHY THIS EXISTS:
 * ----------------
 * The `blocks` stream contains raw HyperLiquid replica commands - the fundamental
 * building blocks of the chain. Unlike other streams (trades, orders, book_updates),
 * blocks are ONLY available via gRPC streaming.
 *
 * The problem: gRPC connections can disconnect. When that happens, you miss blocks.
 * There's no HTTP API to query historical blocks.
 *
 * The solution: The Hyperliquid Foundation maintains a public S3 bucket with
 * historical data. You can backfill missed blocks directly from there.
 *
 *
 * S3 BUCKET STRUCTURE:
 * --------------------
 * Bucket: s3://hl-mainnet-node-data/
 * Access: Requester pays (you pay for data transfer)
 *
 * Available prefixes:
 *   - replica_cmds/          <- blocks data
 *
 * replica_cmds/ directory structure:
 *   replica_cmds/{CHECKPOINT_TIMESTAMP}/{DATE}/{BLOCK_RANGE_FILE}
 *
 *   Example: replica_cmds/1704067200/20240101/830000000-830010000
 *
 *
 * FILE FORMAT:
 * ------------
 * - JSON Lines format (one JSON object per line)
 * - NO block_number field in the JSON (ordering is implicit by line position)
 * - Files are MASSIVE: 3-7 GB each (uncompressed)
 *
 *
 * USAGE:
 * ------
 * npm install @aws-sdk/client-s3
 * node s3_blocks_backfill.js
 *
 *
 * COST CONSIDERATIONS:
 * --------------------
 * - Requester pays bucket - you pay for data transfer
 * - Files are 3-7 GB each
 * - Stream instead of downloading entirely when possible
 */

const { S3Client, ListObjectsV2Command, GetObjectCommand } = require("@aws-sdk/client-s3");
const readline = require("readline");

const S3_BUCKET = "hl-mainnet-node-data";
const BLOCKS_PREFIX = "replica_cmds";

// Requester pays - you need valid AWS credentials
const s3 = new S3Client({ region: "us-east-1" });

/**
 * Parse S3 key into block range info
 * Example: replica_cmds/1704067200/20240101/830000000-830010000
 */
function parseBlockRange(key) {
  const parts = key.split("/");
  if (parts.length !== 4 || parts[0] !== BLOCKS_PREFIX) return null;

  const [start, end] = parts[3].split("-");
  if (!start || !end) return null;

  return {
    checkpoint: parts[1],
    date: parts[2],
    startBlock: parseInt(start, 10),
    endBlock: parseInt(end, 10),
    s3Key: key,
  };
}

/**
 * List S3 prefixes/objects
 */
async function listS3(prefix) {
  try {
    const response = await s3.send(
      new ListObjectsV2Command({
        Bucket: S3_BUCKET,
        Prefix: prefix,
        Delimiter: "/",
        RequestPayer: "requester",
      })
    );

    const items = [];

    // Directories
    for (const p of response.CommonPrefixes || []) {
      if (p.Prefix) {
        const name = p.Prefix.replace(prefix, "").replace("/", "");
        if (name) items.push(name);
      }
    }

    // Files
    for (const obj of response.Contents || []) {
      if (obj.Key) {
        const name = obj.Key.replace(prefix, "");
        if (name) items.push(name);
      }
    }

    return items.sort();
  } catch (err) {
    console.error("S3 error:", err.message);
    return [];
  }
}

/**
 * Find which S3 file contains a specific block number
 */
async function findBlockFile(targetBlock) {
  const checkpoints = await listS3(`${BLOCKS_PREFIX}/`);
  if (checkpoints.length === 0) return null;

  const checkpoint = checkpoints[checkpoints.length - 1];
  const dates = await listS3(`${BLOCKS_PREFIX}/${checkpoint}/`);

  for (const date of dates) {
    const files = await listS3(`${BLOCKS_PREFIX}/${checkpoint}/${date}/`);
    for (const file of files) {
      const br = parseBlockRange(`${BLOCKS_PREFIX}/${checkpoint}/${date}/${file}`);
      if (br && br.startBlock <= targetBlock && targetBlock <= br.endBlock) {
        return br;
      }
    }
  }

  return null;
}

/**
 * Stream blocks from S3
 *
 * Files are 3-7 GB - this streams line-by-line to avoid loading into memory.
 */
async function* streamBlocks(blockRange) {
  const response = await s3.send(
    new GetObjectCommand({
      Bucket: S3_BUCKET,
      Key: blockRange.s3Key,
      RequestPayer: "requester",
    })
  );

  const rl = readline.createInterface({
    input: response.Body,
    crlfDelay: Infinity,
  });

  let lineNumber = 0;
  for await (const line of rl) {
    if (line.trim()) {
      try {
        yield {
          blockNumber: blockRange.startBlock + lineNumber,
          data: JSON.parse(line),
        };
      } catch {
        // Skip malformed lines
      }
      lineNumber++;
    }
  }
}

// Main
async function main() {
  console.log("S3 Blocks Backfill Example\n");
  console.log("=".repeat(60));
  console.log("DISCOVERING S3 STRUCTURE");
  console.log("=".repeat(60) + "\n");

  const checkpoints = await listS3(`${BLOCKS_PREFIX}/`);
  console.log("Checkpoints:", checkpoints);

  if (checkpoints.length > 0) {
    const latest = checkpoints[checkpoints.length - 1];
    const dates = await listS3(`${BLOCKS_PREFIX}/${latest}/`);
    console.log(`Dates in checkpoint ${latest}:`, dates.slice(0, 5), "...");
  }

  // Example: find and stream a block (commented to avoid S3 charges)
  //
  // const br = await findBlockFile(830_000_000);
  // if (br) {
  //   console.log(`Found in ${br.s3Key}`);
  //   for await (const block of streamBlocks(br)) {
  //     if (block.blockNumber === 830_000_000) {
  //       console.log(block);
  //       break;
  //     }
  //   }
  // }
}

main().catch(console.error);
