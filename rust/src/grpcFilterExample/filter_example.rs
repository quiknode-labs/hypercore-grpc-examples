// Filtering Example - Stream only trades for specific coins
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

const GRPC_ENDPOINT: &str = "https://your-endpoint.hype-mainnet.quiknode.pro:10000";
const AUTH_TOKEN: &str = "your-auth-token";

fn decompress(data: &[u8]) -> Result<String, Box<dyn std::error::Error>> {
    if data.len() >= 4 && data[0..4] == [0x28, 0xB5, 0x2F, 0xFD] {
        let decompressed = zstd::decode_all(data)?;
        return Ok(String::from_utf8(decompressed)?);
    }
    Ok(String::from_utf8_lossy(data).to_string())
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let channel = Channel::from_static(GRPC_ENDPOINT)
        .tls_config(ClientTlsConfig::new())?
        .connect()
        .await?;

    let mut client = StreamingClient::new(channel);
    let (tx, rx) = mpsc::channel(32);

    // Subscribe to TRADES with filters
    let mut filters = HashMap::new();
    // Filter for specific coins only
    filters.insert(
        "coin".to_string(),
        FilterValues {
            values: vec!["ETH".to_string(), "BTC".to_string()],
        },
    );

    let subscribe = StreamSubscribe {
        stream_type: StreamType::Trades as i32,
        start_block: 0,
        filters,
        filter_name: "eth-btc-trades".to_string(),
    };

    tx.send(SubscribeRequest {
        request: Some(hyperliquid::subscribe_request::Request::Subscribe(subscribe)),
    })
    .await?;

    println!("Streaming TRADES filtered by coin: ETH, BTC\n");

    // Keep-alive pings
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

    let mut request = Request::new(ReceiverStream::new(rx));
    request
        .metadata_mut()
        .insert("x-token", AUTH_TOKEN.parse::<MetadataValue<_>>()?);

    let mut stream = client.stream_data(request).await?.into_inner();

    while let Some(response) = stream.message().await? {
        if let Some(hyperliquid::subscribe_update::Update::Data(data)) = response.update {
            let decompressed = decompress(&data.data)?;
            if let Ok(parsed) = serde_json::from_str::<serde_json::Value>(&decompressed) {
                println!("Block {}:", data.block_number);
                println!("{}", serde_json::to_string_pretty(&parsed)?);
            }
        }
    }

    Ok(())
}
