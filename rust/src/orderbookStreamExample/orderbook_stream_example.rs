// Orderbook Stream Example - Stream orderbook data via QuickNode gRPC
use std::time::Duration;
use tonic::transport::{Channel, ClientTlsConfig};
use tonic::{metadata::MetadataValue, Request, Status};

pub mod hyperliquid {
    tonic::include_proto!("hyperliquid");
}

use hyperliquid::order_book_streaming_client::OrderBookStreamingClient;
use hyperliquid::{BboBookRequest, L2BookDiffRequest, L2BookRequest, L4BookRequest, L4BookUpdatesRequest, TpslUpdatesRequest};

// Mainnet: "https://your-endpoint.hype-mainnet.quiknode.pro:10000"
// Testnet: "https://your-endpoint.hype-testnet.quiknode.pro:10000"
const DEFAULT_GRPC_ENDPOINT: &str = "https://your-endpoint.hype-mainnet.quiknode.pro:10000";
const DEFAULT_AUTH_TOKEN: &str = "your-quicknode-token";
const MAX_RETRIES: usize = 10;
const BASE_DELAY_SECS: u64 = 2;

fn grpc_endpoint() -> String {
    std::env::var("GRPC_ENDPOINT").unwrap_or_else(|_| DEFAULT_GRPC_ENDPOINT.to_string())
}

fn auth_token() -> String {
    std::env::var("AUTH_TOKEN")
        .or_else(|_| std::env::var("QN_AUTH_TOKEN"))
        .unwrap_or_else(|_| DEFAULT_AUTH_TOKEN.to_string())
}

async fn orderbook_client() -> Result<OrderBookStreamingClient<Channel>, Box<dyn std::error::Error>> {
    let channel = Channel::from_shared(grpc_endpoint())?
        .tls_config(ClientTlsConfig::new())?
        .connect()
        .await?;
    Ok(OrderBookStreamingClient::new(channel))
}

fn with_auth<T>(message: T) -> Result<Request<T>, Box<dyn std::error::Error>> {
    let mut request = Request::new(message);
    request
        .metadata_mut()
        .insert("x-token", auth_token().parse::<MetadataValue<_>>()?);
    Ok(request)
}

fn split_coins(coin_arg: &str, all: bool) -> Vec<String> {
    if all {
        return Vec::new();
    }
    coin_arg
        .split(',')
        .map(|coin| coin.trim())
        .filter(|coin| !coin.is_empty())
        .map(|coin| coin.to_string())
        .collect()
}

fn level_text(level: Option<&hyperliquid::L2Level>) -> String {
    match level {
        Some(level) if !level.px.is_empty() => format!("{} / {} ({})", level.px, level.sz, level.n),
        _ => "n/a".to_string(),
    }
}

async fn stream_l2_orderbook(coin: &str, n_levels: u32, n_sig_figs: Option<u32>, mantissa: Option<u64>, max_messages: Option<usize>) -> Result<(), Box<dyn std::error::Error>> {
    println!("{}", "=".repeat(60));
    println!("Streaming L2 Orderbook for {}", coin);
    println!("Levels: {}", n_levels);
    if let Some(nsf) = n_sig_figs {
        println!("Sig Figs: {}", nsf);
    }
    if let Some(m) = mantissa {
        println!("Mantissa: {}", m);
    }
    println!("Auto-reconnect: true");
    println!("{}\n", "=".repeat(60));

    let mut retry_count = 0;
    let mut total_msg_count = 0usize;

    while retry_count < MAX_RETRIES {
        let endpoint = grpc_endpoint();
        let channel = Channel::from_shared(endpoint.clone())?
            .tls_config(ClientTlsConfig::new())?
            .connect()
            .await?;

        let mut client = OrderBookStreamingClient::new(channel);

        let request = L2BookRequest {
            coin: coin.to_string(),
            n_levels,
            n_sig_figs,
            mantissa,
        };

        if retry_count > 0 {
            println!("\n🔄 Reconnecting (attempt {}/{})...", retry_count + 1, MAX_RETRIES);
        } else {
            println!("Connecting to {}...", endpoint);
        }

        let mut request_with_metadata = Request::new(request);
        request_with_metadata
            .metadata_mut()
            .insert("x-token", auth_token().parse::<MetadataValue<_>>()?);

        let mut stream = match client.stream_l2_book(request_with_metadata).await {
            Ok(response) => response.into_inner(),
            Err(e) => {
                eprintln!("Failed to start stream: {:?}", e);
                return Err(Box::new(e));
            }
        };

        let mut should_retry = false;

        loop {
            match stream.message().await {
                Ok(Some(update)) => {
                    total_msg_count += 1;
                    if retry_count > 0 {
                        retry_count = 0;
                    }

                    if total_msg_count == 1 {
                        println!("✓ First L2 update received!\n");
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

                    println!("\n  Messages received: {}", total_msg_count);
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
                            return Err(Box::new(status));
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

async fn stream_bbo(coins: Vec<String>, max_messages: Option<usize>) -> Result<(), Box<dyn std::error::Error>> {
    println!("{}", "=".repeat(60));
    if coins.is_empty() {
        println!("Streaming BBO for all eligible coins");
    } else {
        println!("Streaming BBO for {}", coins.join(","));
    }
    println!("{}\n", "=".repeat(60));

    let mut retry_count = 0;
    let mut msg_count = 0usize;

    while retry_count < MAX_RETRIES {
        let mut client = orderbook_client().await?;
        let request = BboBookRequest { coins: coins.clone() };
        let mut stream = client.stream_bbo_book(with_auth(request)?).await?.into_inner();
        let mut should_retry = false;

        loop {
            match stream.message().await {
                Ok(Some(update)) => {
                    msg_count += 1;
                    if retry_count > 0 {
                        retry_count = 0;
                    }
                    println!("[{}] BBO {} block={} bid={} ask={}",
                        msg_count, update.coin, update.block_number, level_text(update.bid.as_ref()), level_text(update.ask.as_ref()));

                    if let Some(max) = max_messages {
                        if msg_count >= max {
                            return Ok(());
                        }
                    }
                }
                Ok(None) => break,
                Err(status) => {
                    if status.code() == tonic::Code::DataLoss {
                        retry_count += 1;
                        if retry_count < MAX_RETRIES {
                            let delay = BASE_DELAY_SECS * 2_u64.pow((retry_count - 1) as u32);
                            println!("DATA_LOSS from BBO stream; reconnecting in {}s", delay);
                            tokio::time::sleep(Duration::from_secs(delay)).await;
                            should_retry = true;
                            break;
                        }
                    }
                    return Err(Box::new(status));
                }
            }
        }

        if !should_retry {
            break;
        }
    }

    Ok(())
}

async fn stream_l2_book_diff(coins: Vec<String>, n_levels: u32, n_sig_figs: Option<u32>, mantissa: Option<u64>, skip_initial_snapshot: bool, max_messages: Option<usize>) -> Result<(), Box<dyn std::error::Error>> {
    println!("{}", "=".repeat(60));
    if coins.is_empty() {
        println!("Streaming L2 Book Diffs for all eligible coins");
    } else {
        println!("Streaming L2 Book Diffs for {}", coins.join(","));
    }
    println!("{}\n", "=".repeat(60));

    let mut retry_count = 0;
    let mut msg_count = 0usize;

    while retry_count < MAX_RETRIES {
        let mut client = orderbook_client().await?;
        let request = L2BookDiffRequest {
            coins: coins.clone(),
            n_levels,
            n_sig_figs,
            mantissa,
            skip_initial_snapshot,
        };
        let mut stream = client.stream_l2_book_diff(with_auth(request)?).await?.into_inner();
        let mut should_retry = false;

        loop {
            match stream.message().await {
                Ok(Some(update)) => {
                    msg_count += 1;
                    if retry_count > 0 {
                        retry_count = 0;
                    }
                    println!("[{}] L2 diff height={} snapshot={} coins={}", msg_count, update.height, update.snapshot, update.diffs.len());
                    for diff in update.diffs.iter().take(5) {
                        println!("  {} seq={} prev_seq={} snapshot={} bid_changes={} ask_changes={}",
                            diff.coin, diff.seq, diff.prev_seq, diff.snapshot, diff.bids.len(), diff.asks.len());
                    }

                    if let Some(max) = max_messages {
                        if msg_count >= max {
                            return Ok(());
                        }
                    }
                }
                Ok(None) => break,
                Err(status) => {
                    if status.code() == tonic::Code::DataLoss {
                        retry_count += 1;
                        if retry_count < MAX_RETRIES {
                            let delay = BASE_DELAY_SECS * 2_u64.pow((retry_count - 1) as u32);
                            println!("DATA_LOSS from L2 diff stream; reconnecting in {}s", delay);
                            tokio::time::sleep(Duration::from_secs(delay)).await;
                            should_retry = true;
                            break;
                        }
                    }
                    return Err(Box::new(status));
                }
            }
        }

        if !should_retry {
            break;
        }
    }

    Ok(())
}

async fn stream_l4_book_updates(coins: Vec<String>, max_messages: Option<usize>) -> Result<(), Box<dyn std::error::Error>> {
    println!("{}", "=".repeat(60));
    if coins.is_empty() {
        println!("Streaming L4 Book Updates for all eligible coins");
    } else {
        println!("Streaming L4 Book Updates for {}", coins.join(","));
    }
    println!("{}\n", "=".repeat(60));

    let mut retry_count = 0;
    let mut msg_count = 0usize;

    while retry_count < MAX_RETRIES {
        let mut client = orderbook_client().await?;
        let request = L4BookUpdatesRequest { coins: coins.clone() };
        let mut stream = client.stream_l4_book_updates(with_auth(request)?).await?.into_inner();
        let mut should_retry = false;

        loop {
            match stream.message().await {
                Ok(Some(update)) => {
                    msg_count += 1;
                    if retry_count > 0 {
                        retry_count = 0;
                    }
                    println!("[{}] L4 updates height={} snapshot={} diffs={}", msg_count, update.height, update.snapshot, update.diffs.len());
                    for diff in update.diffs.iter().take(5) {
                        println!("  type={} {} oid={} side={} px={} sz={}",
                            diff.diff_type, diff.coin, diff.oid, diff.side, diff.px, diff.sz);
                    }

                    if let Some(max) = max_messages {
                        if msg_count >= max {
                            return Ok(());
                        }
                    }
                }
                Ok(None) => break,
                Err(status) => {
                    if status.code() == tonic::Code::DataLoss {
                        retry_count += 1;
                        if retry_count < MAX_RETRIES {
                            let delay = BASE_DELAY_SECS * 2_u64.pow((retry_count - 1) as u32);
                            println!("DATA_LOSS from L4 updates stream; reconnecting in {}s", delay);
                            tokio::time::sleep(Duration::from_secs(delay)).await;
                            should_retry = true;
                            break;
                        }
                    }
                    return Err(Box::new(status));
                }
            }
        }

        if !should_retry {
            break;
        }
    }

    Ok(())
}

async fn stream_tpsl_updates(coins: Vec<String>, max_messages: Option<usize>) -> Result<(), Box<dyn std::error::Error>> {
    println!("{}", "=".repeat(60));
    if coins.is_empty() {
        println!("Streaming TP/SL Updates for all perp coins");
    } else {
        println!("Streaming TP/SL Updates for {}", coins.join(","));
    }
    println!("{}\n", "=".repeat(60));

    let mut retry_count = 0;
    let mut msg_count = 0usize;

    while retry_count < MAX_RETRIES {
        let mut client = orderbook_client().await?;
        let request = TpslUpdatesRequest { coins: coins.clone() };
        let mut stream = client.stream_tpsl_updates(with_auth(request)?).await?.into_inner();
        let mut should_retry = false;

        loop {
            match stream.message().await {
                Ok(Some(update)) => {
                    msg_count += 1;
                    if retry_count > 0 {
                        retry_count = 0;
                    }
                    println!("[{}] TP/SL height={} snapshot={} diffs={}", msg_count, update.height, update.snapshot, update.diffs.len());
                    for diff in update.diffs.iter().take(5) {
                        println!("  type={} {} oid={} trigger={} limit={} sz={} reason={}",
                            diff.diff_type, diff.coin, diff.oid, diff.trigger_px, diff.limit_px, diff.sz, diff.reason);
                    }

                    if let Some(max) = max_messages {
                        if msg_count >= max {
                            return Ok(());
                        }
                    }
                }
                Ok(None) => break,
                Err(status) => {
                    if status.code() == tonic::Code::DataLoss {
                        retry_count += 1;
                        if retry_count < MAX_RETRIES {
                            let delay = BASE_DELAY_SECS * 2_u64.pow((retry_count - 1) as u32);
                            println!("DATA_LOSS from TP/SL updates stream; reconnecting in {}s", delay);
                            tokio::time::sleep(Duration::from_secs(delay)).await;
                            should_retry = true;
                            break;
                        }
                    }
                    return Err(Box::new(status));
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
        let endpoint = grpc_endpoint();
        let channel = Channel::from_shared(endpoint.clone())?
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
            println!("Connecting to {}...", endpoint);
        }

        let mut request_with_metadata = Request::new(request);
        request_with_metadata
            .metadata_mut()
            .insert("x-token", auth_token().parse::<MetadataValue<_>>()?);

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
                    if retry_count > 0 {
                        retry_count = 0;
                    }

                    if let Some(snapshot) = update.snapshot {
                        snapshot_received = true;

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
                            return Err(Box::new(status));
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
    let mut all = false;
    let mut levels = 20u32;
    let mut n_sig_figs: Option<u32> = None;
    let mut mantissa: Option<u64> = None;
    let mut skip_initial_snapshot = false;
    let mut max_messages: Option<usize> = None;

    // Parse args
    for arg in args.iter().skip(1) {
        if let Some(value) = arg.strip_prefix("--mode=") {
            mode = value;
        } else if let Some(value) = arg.strip_prefix("--coin=") {
            coin = value;
        } else if arg == "--all" {
            all = true;
        } else if let Some(value) = arg.strip_prefix("--levels=") {
            levels = value.parse().unwrap_or(20);
        } else if let Some(value) = arg.strip_prefix("--sig-figs=") {
            n_sig_figs = value.parse().ok();
        } else if let Some(value) = arg.strip_prefix("--mantissa=") {
            mantissa = value.parse().ok();
        } else if arg == "--skip-initial-snapshot" {
            skip_initial_snapshot = true;
        } else if let Some(value) = arg.strip_prefix("--max-messages=") {
            max_messages = Some(value.parse().unwrap_or(0));
        }
    }

    if all && (mode == "l2" || mode == "l4") {
        eprintln!("--all is only supported for bbo, l2-diff, l4-updates, and tpsl. Use --coin for l2 or l4.");
        std::process::exit(2);
    }
    let coins = split_coins(coin, all);
    if !all && coins.is_empty() {
        eprintln!("--coin must include at least one symbol. Use --all to subscribe to every eligible coin on multi-coin streams.");
        std::process::exit(2);
    }

    println!("\n{}", "=".repeat(60));
    println!("Hyperliquid Orderbook Stream Example");
    println!("Endpoint: {}", grpc_endpoint());
    println!("{}", "=".repeat(60));

    if auth_token() == DEFAULT_AUTH_TOKEN {
        eprintln!("Set AUTH_TOKEN to your QuickNode token before running this example");
        std::process::exit(1);
    }

    let single_coin = coins.first().map(String::as_str).unwrap_or("");

    match mode {
        "l2" => stream_l2_orderbook(single_coin, levels, n_sig_figs, mantissa, max_messages).await,
        "l4" => stream_l4_orderbook(single_coin, max_messages).await,
        "bbo" => stream_bbo(coins, max_messages).await,
        "l2-diff" => stream_l2_book_diff(coins, levels, n_sig_figs, mantissa, skip_initial_snapshot, max_messages).await,
        "l4-updates" => stream_l4_book_updates(coins, max_messages).await,
        "tpsl" => stream_tpsl_updates(coins, max_messages).await,
        _ => {
            eprintln!("Invalid mode. Use --mode=l2, l4, bbo, l2-diff, l4-updates, or tpsl");
            std::process::exit(1);
        }
    }
}
