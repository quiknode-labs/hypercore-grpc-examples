// Orderbook Stream Example - Stream L2 and L4 orderbook data via gRPC
use std::time::Duration;
use tonic::transport::{Channel, ClientTlsConfig};
use tonic::{metadata::MetadataValue, Request, Status};

pub mod hyperliquid {
    tonic::include_proto!("hyperliquid");
}

use hyperliquid::order_book_streaming_client::OrderBookStreamingClient;
use hyperliquid::{L2BookRequest, L4BookRequest};

const GRPC_ENDPOINT: &str = "https://your-endpoint.hype-mainnet.quiknode.pro:10000";
const AUTH_TOKEN: &str = "your-auth-token";
const MAX_RETRIES: usize = 10;
const BASE_DELAY_SECS: u64 = 2;

async fn stream_l2_orderbook(coin: &str, n_levels: u32) -> Result<(), Box<dyn std::error::Error>> {
    println!("{}", "=".repeat(60));
    println!("Streaming L2 Orderbook for {}", coin);
    println!("Levels: {}", n_levels);
    println!("Auto-reconnect: true");
    println!("{}\n", "=".repeat(60));

    let mut retry_count = 0;

    while retry_count < MAX_RETRIES {
        let channel = Channel::from_static(GRPC_ENDPOINT)
            .tls_config(ClientTlsConfig::new())?
            .connect()
            .await?;

        let mut client = OrderBookStreamingClient::new(channel);

        let request = L2BookRequest {
            coin: coin.to_string(),
            n_levels,
            n_sig_figs: None,
            mantissa: None,
        };

        if retry_count > 0 {
            println!("\n🔄 Reconnecting (attempt {}/{})...", retry_count + 1, MAX_RETRIES);
        } else {
            println!("Connecting to {}...", GRPC_ENDPOINT);
        }

        let mut request_with_metadata = Request::new(request);
        request_with_metadata
            .metadata_mut()
            .insert("x-token", AUTH_TOKEN.parse::<MetadataValue<_>>()?);

        let mut stream = match client.stream_l2_book(request_with_metadata).await {
            Ok(response) => response.into_inner(),
            Err(e) => {
                eprintln!("Failed to start stream: {:?}", e);
                return Err(Box::new(e));
            }
        };

        let mut msg_count = 0;
        let mut should_retry = false;

        loop {
            match stream.message().await {
                Ok(Some(update)) => {
                    msg_count += 1;

                    if msg_count == 1 {
                        println!("✓ First L2 update received!\n");
                        retry_count = 0; // Reset on success
                    }

                    // Display orderbook
                    println!("\n{}", "─".repeat(60));
                    println!("Block: {} | Time: {} | Coin: {}", update.block_number, update.time, update.coin);
                    println!("{}", "─".repeat(60));

                    // Display asks (reversed)
                    if !update.asks.is_empty() {
                        println!("\n  ASKS:");
                        let ask_count = update.asks.len().min(10);
                        for level in update.asks.iter().take(ask_count).rev() {
                            println!("    {:>12} | {:>12} | ({} orders)", level.px, level.sz, level.n);
                        }
                    }

                    // Display spread
                    if !update.bids.is_empty() && !update.asks.is_empty() {
                        println!("\n  {}", "─".repeat(44));
                        println!("  SPREAD: (best bid: {}, best ask: {})", update.bids[0].px, update.asks[0].px);
                        println!("  {}", "─".repeat(44));
                    }

                    // Display bids
                    if !update.bids.is_empty() {
                        println!("\n  BIDS:");
                        let bid_count = update.bids.len().min(10);
                        for level in update.bids.iter().take(bid_count) {
                            println!("    {:>12} | {:>12} | ({} orders)", level.px, level.sz, level.n);
                        }
                    }

                    println!("\n  Messages received: {}", msg_count);
                }
                Ok(None) => {
                    println!("\nStream ended");
                    break;
                }
                Err(status) => {
                    if status.code() == tonic::Code::DataLoss {
                        println!("\n⚠️  Server reinitialized: {}", status.message());
                        retry_count += 1;
                        if retry_count < MAX_RETRIES {
                            let delay = BASE_DELAY_SECS * 2_u64.pow((retry_count - 1) as u32);
                            println!("⏳ Waiting {}s before reconnecting...", delay);
                            tokio::time::sleep(Duration::from_secs(delay)).await;
                            should_retry = true;
                            break;
                        } else {
                            println!("\n❌ Max retries ({}) reached. Giving up.", MAX_RETRIES);
                            return Ok(());
                        }
                    } else {
                        eprintln!("\ngRPC error: {:?}", status);
                        return Err(Box::new(status));
                    }
                }
            }
        }

        if !should_retry {
            break;
        }
    }

    Ok(())
}

async fn stream_l4_orderbook(coin: &str, max_messages: Option<usize>) -> Result<(), Box<dyn std::error::Error>> {
    println!("{}", "=".repeat(60));
    println!("Streaming L4 Orderbook for {}", coin);
    println!("Auto-reconnect: true");
    println!("{}\n", "=".repeat(60));

    let mut retry_count = 0;
    let mut total_msg_count = 0;

    while retry_count < MAX_RETRIES {
        let channel = Channel::from_static(GRPC_ENDPOINT)
            .tls_config(ClientTlsConfig::new())?
            .connect()
            .await?;

        let mut client = OrderBookStreamingClient::new(channel);

        let request = L4BookRequest {
            coin: coin.to_string(),
        };

        if retry_count > 0 {
            println!("\n🔄 Reconnecting (attempt {}/{})...", retry_count + 1, MAX_RETRIES);
        } else {
            println!("Connecting to {}...", GRPC_ENDPOINT);
        }

        let mut request_with_metadata = Request::new(request);
        request_with_metadata
            .metadata_mut()
            .insert("x-token", AUTH_TOKEN.parse::<MetadataValue<_>>()?);

        let mut stream = match client.stream_l4_book(request_with_metadata).await {
            Ok(response) => response.into_inner(),
            Err(e) => {
                eprintln!("Failed to start stream: {:?}", e);
                return Err(Box::new(e));
            }
        };

        let mut snapshot_received = false;
        let mut should_retry = false;

        loop {
            match stream.message().await {
                Ok(Some(update)) => {
                    total_msg_count += 1;

                    if let Some(snapshot) = update.snapshot {
                        snapshot_received = true;
                        retry_count = 0; // Reset on success

                        println!("\n✓ L4 Snapshot Received!");
                        println!("{}", "─".repeat(60));
                        println!("Coin: {}", snapshot.coin);
                        println!("Height: {}", snapshot.height);
                        println!("Time: {}", snapshot.time);
                        println!("Bids: {} orders", snapshot.bids.len());
                        println!("Asks: {} orders", snapshot.asks.len());
                        println!("{}", "─".repeat(60));

                        // Sample bids
                        if !snapshot.bids.is_empty() {
                            println!("\nSample Bids (first 5):");
                            for order in snapshot.bids.iter().take(5) {
                                let user_short = if order.user.len() > 10 {
                                    format!("{}...", &order.user[..10])
                                } else {
                                    order.user.clone()
                                };
                                println!("  OID: {} | Price: {} | Size: {} | User: {}",
                                    order.oid, order.limit_px, order.sz, user_short);
                            }
                        }

                        // Sample asks
                        if !snapshot.asks.is_empty() {
                            println!("\nSample Asks (first 5):");
                            for order in snapshot.asks.iter().take(5) {
                                let user_short = if order.user.len() > 10 {
                                    format!("{}...", &order.user[..10])
                                } else {
                                    order.user.clone()
                                };
                                println!("  OID: {} | Price: {} | Size: {} | User: {}",
                                    order.oid, order.limit_px, order.sz, user_short);
                            }
                        }

                    } else if let Some(diff) = update.diff {
                        if !snapshot_received {
                            println!("\n⚠ Received diff before snapshot");
                        }

                        match serde_json::from_str::<serde_json::Value>(&diff.data) {
                            Ok(diff_data) => {
                                let order_statuses = diff_data["order_statuses"].as_array()
                                    .map(|v| v.len()).unwrap_or(0);
                                let book_diffs = diff_data["book_diffs"].as_array()
                                    .map(|v| v.len()).unwrap_or(0);

                                println!("\n[Block {}] L4 Diff:", diff.height);
                                println!("  Time: {}", diff.time);
                                println!("  Order Statuses: {}", order_statuses);
                                println!("  Book Diffs: {}", book_diffs);

                                if book_diffs > 0 && book_diffs <= 5 {
                                    if let Some(diffs_array) = diff_data["book_diffs"].as_array() {
                                        println!("  Diffs: {}", serde_json::to_string_pretty(diffs_array)?);
                                    }
                                }
                            }
                            Err(e) => {
                                println!("  Error parsing diff: {}", e);
                            }
                        }
                    }

                    if let Some(max) = max_messages {
                        if total_msg_count >= max {
                            println!("\nReached max messages ({}), stopping...", max);
                            return Ok(());
                        }
                    }
                }
                Ok(None) => {
                    println!("\nStream ended");
                    break;
                }
                Err(status) => {
                    if status.code() == tonic::Code::DataLoss {
                        println!("\n⚠️  Server reinitialized: {}", status.message());
                        retry_count += 1;
                        if retry_count < MAX_RETRIES {
                            let delay = BASE_DELAY_SECS * 2_u64.pow((retry_count - 1) as u32);
                            println!("⏳ Waiting {}s before reconnecting...", delay);
                            tokio::time::sleep(Duration::from_secs(delay)).await;
                            should_retry = true;
                            break;
                        } else {
                            println!("\n❌ Max retries ({}) reached. Giving up.", MAX_RETRIES);
                            return Ok(());
                        }
                    } else {
                        eprintln!("\ngRPC error: {:?}", status);
                        return Err(Box::new(status));
                    }
                }
            }
        }

        if !should_retry {
            break;
        }
    }

    Ok(())
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let args: Vec<String> = std::env::args().collect();

    let mut mode = "l2";
    let mut coin = "BTC";
    let mut levels = 20u32;
    let mut max_messages: Option<usize> = None;

    // Parse args
    for arg in args.iter().skip(1) {
        if let Some(value) = arg.strip_prefix("--mode=") {
            mode = value;
        } else if let Some(value) = arg.strip_prefix("--coin=") {
            coin = value;
        } else if let Some(value) = arg.strip_prefix("--levels=") {
            levels = value.parse().unwrap_or(20);
        } else if let Some(value) = arg.strip_prefix("--max-messages=") {
            max_messages = Some(value.parse().unwrap_or(0));
        }
    }

    println!("\n{}", "=".repeat(60));
    println!("Hyperliquid Orderbook Stream Example");
    println!("Endpoint: {}", GRPC_ENDPOINT);
    println!("{}", "=".repeat(60));

    match mode {
        "l2" => stream_l2_orderbook(coin, levels).await,
        "l4" => stream_l4_orderbook(coin, max_messages).await,
        _ => {
            eprintln!("Invalid mode. Use --mode=l2 or --mode=l4");
            std::process::exit(1);
        }
    }
}
