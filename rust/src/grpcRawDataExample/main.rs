use clap::Parser;
use std::collections::HashMap;
use tokio::sync::mpsc;
use tokio_stream::wrappers::ReceiverStream;
use tonic::transport::{Channel, ClientTlsConfig};
use tonic::{metadata::MetadataValue, Request};

pub mod hyperliquid {
    tonic::include_proto!("hyperliquid");
}

use hyperliquid::{
    streaming_client::StreamingClient, FilterValues, Ping, StreamSubscribe, StreamType,
    SubscribeRequest,
};

// Configuration
const GRPC_ENDPOINT: &str = "https://your-endpoint.hype-mainnet.quiknode.pro:10000";
const AUTH_TOKEN: &str = "your-auth-token";

// Zstd magic number
const ZSTD_MAGIC: [u8; 4] = [0x28, 0xB5, 0x2F, 0xFD];

fn decompress(data: &[u8]) -> Result<String, Box<dyn std::error::Error>> {
    if data.len() < 4 {
        return Ok(String::from_utf8_lossy(data).to_string());
    }

    // Check zstd magic number
    if data[0..4] == ZSTD_MAGIC {
        let decompressed = zstd::decode_all(data)?;
        return Ok(String::from_utf8(decompressed)?);
    }

    Ok(String::from_utf8_lossy(data).to_string())
}

async fn create_channel() -> Result<Channel, Box<dyn std::error::Error>> {
    let tls = ClientTlsConfig::new();

    let channel = Channel::from_static(GRPC_ENDPOINT)
        .tls_config(tls)?
        .connect()
        .await?;

    Ok(channel)
}

fn parse_stream_type(s: &str) -> StreamType {
    match s.to_uppercase().as_str() {
        "TRADES" => StreamType::Trades,
        "ORDERS" => StreamType::Orders,
        "EVENTS" => StreamType::Events,
        "BOOK_UPDATES" => StreamType::BookUpdates,
        "TWAP" => StreamType::Twap,
        "BLOCKS" => StreamType::Blocks,
        "WRITER_ACTIONS" => StreamType::WriterActions,
        _ => StreamType::Trades,
    }
}

async fn stream_data(
    stream_type: &str,
    filters: HashMap<String, Vec<String>>,
) -> Result<(), Box<dyn std::error::Error>> {
    let channel = create_channel().await?;
    let mut client = StreamingClient::new(channel);

    // Create request stream
    let (tx, rx) = mpsc::channel(32);
    let stream = ReceiverStream::new(rx);

    // Build subscription
    let mut subscribe = StreamSubscribe {
        stream_type: parse_stream_type(stream_type) as i32,
        start_block: 0,
        filters: HashMap::new(),
        filter_name: String::new(),
    };

    // Add filters
    if !filters.is_empty() {
        for (field, values) in &filters {
            subscribe.filters.insert(
                field.clone(),
                FilterValues {
                    values: values.clone(),
                },
            );
        }
        println!("Filters applied: {:?}", filters);
    }

    // Send subscription
    tx.send(SubscribeRequest {
        request: Some(hyperliquid::subscribe_request::Request::Subscribe(subscribe)),
    })
    .await?;

    println!("Streaming {}...", stream_type);

    // Keep-alive ping task
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

    // Create request with auth
    let mut request = Request::new(stream);
    let token: MetadataValue<_> = AUTH_TOKEN.parse()?;
    request.metadata_mut().insert("x-token", token);

    // Start streaming
    let mut response_stream = client.stream_data(request).await?.into_inner();

    while let Some(response) = response_stream.message().await? {
        if let Some(update) = response.update {
            match update {
                hyperliquid::subscribe_update::Update::Data(data) => {
                    let decompressed = decompress(&data.data)?;

                    match serde_json::from_str::<serde_json::Value>(&decompressed) {
                        Ok(parsed) => {
                            println!(
                                "\nBlock {} | Timestamp {}",
                                data.block_number, data.timestamp
                            );
                            println!("{}", serde_json::to_string_pretty(&parsed)?);
                        }
                        Err(_) => {
                            println!("Block {}: {}", data.block_number, decompressed);
                        }
                    }
                }
                hyperliquid::subscribe_update::Update::Pong(pong) => {
                    println!("Pong: {}", pong.timestamp);
                }
            }
        }
    }

    Ok(())
}

#[derive(Parser)]
#[command(name = "hyperliquid-grpc")]
#[command(about = "Hyperliquid gRPC streaming client")]
struct Args {
    /// Stream type: TRADES, ORDERS, EVENTS, etc.
    #[arg(short, long, default_value = "TRADES")]
    stream: String,

    /// Filters in format: field=val1,val2 (can be repeated)
    #[arg(short, long)]
    filter: Vec<String>,
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let args = Args::parse();

    // Parse filters
    let mut filters = HashMap::new();
    for f in &args.filter {
        if let Some((field, values)) = f.split_once('=') {
            filters.insert(
                field.to_string(),
                values.split(',').map(|s| s.to_string()).collect(),
            );
        }
    }

    stream_data(&args.stream, filters).await
}
