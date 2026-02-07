//! S3 Blocks Backfill Example
//! ==========================
//!
//! WHY THIS EXISTS:
//! ----------------
//! The `blocks` stream contains raw HyperLiquid replica commands - the fundamental
//! building blocks of the chain. Unlike other streams (trades, orders, book_updates),
//! blocks are ONLY available via gRPC streaming.
//!
//! The problem: gRPC connections can disconnect. When that happens, you miss blocks.
//! There's no HTTP API to query historical blocks.
//!
//! The solution: The Hyperliquid Foundation maintains a public S3 bucket with
//! historical data. You can backfill missed blocks directly from there.
//!
//!
//! S3 BUCKET STRUCTURE:
//! --------------------
//! Bucket: s3://hl-mainnet-node-data/
//! Access: Requester pays (you pay for data transfer)
//!
//! Available prefixes:
//!   - replica_cmds/          <- blocks data
//!
//! replica_cmds/ directory structure:
//!   replica_cmds/{CHECKPOINT_TIMESTAMP}/{DATE}/{BLOCK_RANGE_FILE}
//!
//!   Example: replica_cmds/1704067200/20240101/830000000-830010000
//!
//!
//! FILE FORMAT:
//! ------------
//! - JSON Lines format (one JSON object per line)
//! - NO block_number field in the JSON (ordering is implicit by line position)
//! - Files are MASSIVE: 3-7 GB each (uncompressed)
//!
//!
//! USAGE:
//! ------
//! Add to Cargo.toml:
//!   aws-config = "1.0"
//!   aws-sdk-s3 = "1.0"
//!   tokio = { version = "1", features = ["full"] }
//!
//! cargo run --bin s3_blocks_backfill
//!
//!
//! COST CONSIDERATIONS:
//! --------------------
//! - Requester pays bucket - you pay for data transfer
//! - Files are 3-7 GB each
//! - Stream instead of downloading entirely when possible

use aws_sdk_s3::Client;
use std::io::{BufRead, BufReader, Cursor};

const S3_BUCKET: &str = "hl-mainnet-node-data";
const BLOCKS_PREFIX: &str = "replica_cmds";

/// Represents a block range file in S3
#[derive(Debug, Clone)]
pub struct BlockRange {
    pub checkpoint: String,
    pub date: String,
    pub start_block: u64,
    pub end_block: u64,
    pub s3_key: String,
}

impl BlockRange {
    /// Parse S3 key: replica_cmds/1704067200/20240101/830000000-830010000
    pub fn from_s3_key(key: &str) -> Option<Self> {
        let parts: Vec<&str> = key.split('/').collect();
        if parts.len() != 4 || parts[0] != BLOCKS_PREFIX {
            return None;
        }

        let range_parts: Vec<&str> = parts[3].split('-').collect();
        if range_parts.len() != 2 {
            return None;
        }

        let start_block = range_parts[0].parse().ok()?;
        let end_block = range_parts[1].parse().ok()?;

        Some(Self {
            checkpoint: parts[1].to_string(),
            date: parts[2].to_string(),
            start_block,
            end_block,
            s3_key: key.to_string(),
        })
    }
}

/// A parsed block from S3
#[derive(Debug)]
pub struct Block {
    pub block_number: u64,
    pub data: serde_json::Value,
}

/// List S3 objects under a prefix
pub async fn list_s3(client: &Client, prefix: &str) -> Result<Vec<String>, aws_sdk_s3::Error> {
    let result = client
        .list_objects_v2()
        .bucket(S3_BUCKET)
        .prefix(prefix)
        .delimiter("/")
        .request_payer(aws_sdk_s3::types::RequestPayer::Requester)
        .send()
        .await?;

    let mut items = Vec::new();

    // Directories
    for p in result.common_prefixes() {
        if let Some(prefix_str) = p.prefix() {
            let name = prefix_str.trim_start_matches(prefix).trim_end_matches('/');
            if !name.is_empty() {
                items.push(name.to_string());
            }
        }
    }

    // Files
    for obj in result.contents() {
        if let Some(key) = obj.key() {
            let name = key.trim_start_matches(prefix);
            if !name.is_empty() {
                items.push(name.to_string());
            }
        }
    }

    items.sort();
    Ok(items)
}

/// Find which S3 file contains a specific block number
pub async fn find_block_file(client: &Client, target_block: u64) -> Option<BlockRange> {
    let checkpoints = list_s3(client, &format!("{}/", BLOCKS_PREFIX)).await.ok()?;
    let checkpoint = checkpoints.last()?;

    let dates = list_s3(client, &format!("{}/{}/", BLOCKS_PREFIX, checkpoint))
        .await
        .ok()?;

    for date in dates {
        let files = list_s3(
            client,
            &format!("{}/{}/{}/", BLOCKS_PREFIX, checkpoint, date),
        )
        .await
        .ok()?;

        for file in files {
            let key = format!("{}/{}/{}/{}", BLOCKS_PREFIX, checkpoint, date, file);
            if let Some(br) = BlockRange::from_s3_key(&key) {
                if br.start_block <= target_block && target_block <= br.end_block {
                    return Some(br);
                }
            }
        }
    }

    None
}

/// Stream blocks from S3. Files are 3-7 GB - streams line-by-line.
pub async fn stream_blocks(
    client: &Client,
    block_range: &BlockRange,
) -> impl Iterator<Item = Block> {
    let result = client
        .get_object()
        .bucket(S3_BUCKET)
        .key(&block_range.s3_key)
        .request_payer(aws_sdk_s3::types::RequestPayer::Requester)
        .send()
        .await;

    let start_block = block_range.start_block;
    let mut blocks = Vec::new();

    if let Ok(output) = result {
        // Note: In production, use async streaming. This is simplified for example.
        let body = match output.body.collect().await {
            Ok(aggregated) => aggregated.into_bytes(),
            Err(err) => {
                eprintln!("Failed to read S3 body: {}", err);
                return blocks.into_iter();
            }
        };
        let reader = BufReader::new(Cursor::new(body));

        for (line_number, line) in reader.lines().enumerate() {
            if let Ok(line) = line {
                if line.trim().is_empty() {
                    continue;
                }
                if let Ok(data) = serde_json::from_str(&line) {
                    blocks.push(Block {
                        block_number: start_block + line_number as u64,
                        data,
                    });
                }
            }
        }
    }

    blocks.into_iter()
}

#[tokio::main]
async fn main() {
    println!("S3 Blocks Backfill Example");
    println!("{}", "=".repeat(60));
    println!("DISCOVERING S3 STRUCTURE");
    println!("{}\n", "=".repeat(60));

    // Load AWS config
    let config = aws_config::load_defaults(aws_config::BehaviorVersion::latest()).await;
    let client = Client::new(&config);

    // List checkpoints
    match list_s3(&client, &format!("{}/", BLOCKS_PREFIX)).await {
        Ok(checkpoints) => {
            println!("Checkpoints: {:?}", checkpoints);

            if let Some(latest) = checkpoints.last() {
                if let Ok(dates) = list_s3(&client, &format!("{}/{}/", BLOCKS_PREFIX, latest)).await
                {
                    let display: Vec<_> = dates.iter().take(5).collect();
                    println!("Dates in checkpoint {}: {:?} ...", latest, display);
                }
            }
        }
        Err(e) => println!("Error listing S3: {}", e),
    }

    // Example: find and stream a block (commented to avoid S3 charges)
    //
    // if let Some(br) = find_block_file(&client, 830_000_000).await {
    //     println!("Found in {}", br.s3_key);
    //     for block in stream_blocks(&client, &br).await {
    //         if block.block_number == 830_000_000 {
    //             println!("{:#?}", block);
    //             break;
    //         }
    //     }
    // }
}
