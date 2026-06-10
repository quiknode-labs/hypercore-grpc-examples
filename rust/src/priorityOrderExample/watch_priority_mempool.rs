use clap::Parser;
use tokio::sync::mpsc;
use tokio_stream::wrappers::ReceiverStream;
use tonic::transport::{Channel, ClientTlsConfig};
use tonic::{metadata::MetadataValue, Request};

pub mod hyperliquid {
    tonic::include_proto!("hyperliquid");
}

use hyperliquid::{
    streaming_client::StreamingClient, Ping, StreamSubscribe, StreamType,
    SubscribeRequest,
};

const DEFAULT_GRPC_ENDPOINT: &str = "https://your-endpoint.hype-testnet.quiknode.pro:10000";
const DEFAULT_AUTH_TOKEN: &str = "YOUR_QUICKNODE_TOKEN";
const ZSTD_MAGIC: [u8; 4] = [0x28, 0xB5, 0x2F, 0xFD];

fn grpc_endpoint() -> String {
    std::env::var("GRPC_ENDPOINT").unwrap_or_else(|_| DEFAULT_GRPC_ENDPOINT.to_string())
}

fn auth_token() -> String {
    std::env::var("AUTH_TOKEN")
        .or_else(|_| std::env::var("QN_AUTH_TOKEN"))
        .unwrap_or_else(|_| DEFAULT_AUTH_TOKEN.to_string())
}

async fn create_channel() -> Result<Channel, Box<dyn std::error::Error>> {
    let channel = Channel::from_shared(grpc_endpoint())?
        .tls_config(ClientTlsConfig::new())?
        .connect()
        .await?;
    Ok(channel)
}

fn decompress(data: &[u8]) -> Result<String, Box<dyn std::error::Error>> {
    if data.len() >= 4 && data[0..4] == ZSTD_MAGIC {
        let decompressed = zstd::decode_all(data)?;
        return Ok(String::from_utf8(decompressed)?);
    }

    Ok(String::from_utf8_lossy(data).to_string())
}

fn priority_fees(value: &serde_json::Value) -> Vec<String> {
    let mut fees = Vec::new();
    match value {
        serde_json::Value::Object(map) => {
            if let Some(grouping) = map.get("grouping") {
                if let Some(p) = grouping.get("p") {
                    fees.push(p.to_string());
                }
            }
            for item in map.values() {
                fees.extend(priority_fees(item));
            }
        }
        serde_json::Value::Array(items) => {
            for item in items {
                fees.extend(priority_fees(item));
            }
        }
        _ => {}
    }
    fees
}

fn matches_text_filters(text: &str, contains: &[String]) -> bool {
    contains.is_empty() || contains.iter().any(|needle| text.contains(needle))
}

#[derive(Parser)]
#[command(name = "priority-order-example")]
#[command(about = "Watch testnet priority MEMPOOL_TXS from a QuickNode Hyperliquid gRPC endpoint")]
struct Args {
    #[arg(long, default_value_t = 0)]
    start_block: u64,

    #[arg(long)]
    contains: Vec<String>,

    #[arg(long)]
    all_mempool: bool,

    #[arg(long)]
    max_messages: Option<usize>,

    #[arg(long)]
    compact: bool,
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let args = Args::parse();

    if grpc_endpoint() == DEFAULT_GRPC_ENDPOINT {
        eprintln!("Set GRPC_ENDPOINT to your QuickNode Hyperliquid testnet gRPC endpoint");
        std::process::exit(2);
    }
    if auth_token() == DEFAULT_AUTH_TOKEN {
        eprintln!("Set AUTH_TOKEN to your QuickNode token");
        std::process::exit(2);
    }

    let channel = create_channel().await?;
    let mut client = StreamingClient::new(channel);
    let (tx, rx) = mpsc::channel(32);
    let stream = ReceiverStream::new(rx);

    tx.send(SubscribeRequest {
        request: Some(hyperliquid::subscribe_request::Request::Subscribe(
            StreamSubscribe {
                stream_type: StreamType::MempoolTxs as i32,
                start_block: args.start_block,
                filters: std::collections::HashMap::new(),
                filter_name: String::new(),
            },
        )),
    })
    .await?;

    let tx_ping = tx.clone();
    tokio::spawn(async move {
        let mut interval = tokio::time::interval(std::time::Duration::from_secs(30));
        loop {
            interval.tick().await;
            let _ = tx_ping
                .send(SubscribeRequest {
                    request: Some(hyperliquid::subscribe_request::Request::Ping(Ping {
                        timestamp: chrono::Utc::now().timestamp_millis(),
                    })),
                })
                .await;
        }
    });

    let mut request = Request::new(stream);
    request
        .metadata_mut()
        .insert("x-token", auth_token().parse::<MetadataValue<_>>()?);

    println!("Watching testnet MEMPOOL_TXS");
    println!("Endpoint: {}", grpc_endpoint());
    if !args.all_mempool {
        println!("Filter: priority grouping only");
    }
    if !args.contains.is_empty() {
        println!("Text filters: {:?}", args.contains);
    }

    let mut response_stream = client.stream_data(request).await?.into_inner();
    let mut printed = 0usize;

    while let Some(response) = response_stream.message().await? {
        let Some(update) = response.update else {
            continue;
        };
        let hyperliquid::subscribe_update::Update::Data(data) = update else {
            continue;
        };

        let text = decompress(data.data.as_bytes())?;
        if !matches_text_filters(&text, &args.contains) {
            continue;
        }

        let parsed = serde_json::from_str::<serde_json::Value>(&text).ok();
        let fees = parsed.as_ref().map(priority_fees).unwrap_or_default();
        if !args.all_mempool && fees.is_empty() {
            continue;
        }

        printed += 1;
        println!("\nBlock {} | Timestamp {}", data.block_number, data.timestamp);
        if !fees.is_empty() {
            println!("Priority fee grouping p: {}", fees.join(", "));
        }
        if args.compact {
            println!("{}", text.chars().take(1000).collect::<String>());
        } else if let Some(parsed) = parsed {
            println!("{}", serde_json::to_string_pretty(&parsed)?);
        } else {
            println!("{}", text);
        }

        if let Some(max) = args.max_messages {
            if printed >= max {
                return Ok(());
            }
        }
    }

    Ok(())
}
