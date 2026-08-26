// HIP-4 Outcome Markets Example - Stream one venue's orders with
// subscription tagging and signer enrichment.
//
// Demonstrates the three HIP-4-era streaming features:
//  1. "venue" filter key: server-side expansion of an outcome venue's name
//     to its full coin set (coins look like "#146870" and churn as outcomes
//     settle - the server tracks that for you).
//  2. subscription_id: a client-chosen tag echoed on every update, plus the
//     stream_type field, so multiplexed subscriptions are distinguishable.
//  3. EnrichmentOptions.include_signer: each order carries "signer" - the
//     wallet that actually SUBMITTED it (master or approved API wallet),
//     recovered from the action's signature. Unsigned engine events
//     (trigger fires, liquidations, TWAP children) carry "signer": null.
//
// Find active venue names via the info endpoint: {"type":"outcomeMeta"}
// (fields: outcomes[].venue, deployers[].venue).
use std::collections::HashMap;
use tokio::sync::mpsc;
use tokio_stream::wrappers::ReceiverStream;
use tonic::transport::{Channel, ClientTlsConfig};
use tonic::{metadata::MetadataValue, Request};

pub mod hyperliquid {
    tonic::include_proto!("hyperliquid");
}

use hyperliquid::{
    streaming_client::StreamingClient, EnrichmentOptions, FilterValues, Ping, StreamSubscribe,
    StreamType, SubscribeRequest,
};

// HIP-4 launches on testnet first; use your testnet endpoint until mainnet
// venues go live.
// Mainnet: "https://your-endpoint.hype-mainnet.quiknode.pro:10000"
// Testnet: "https://your-endpoint.hype-testnet.quiknode.pro:10000"
const GRPC_ENDPOINT: &str = "https://your-endpoint.hype-testnet.quiknode.pro:10000";
const AUTH_TOKEN: &str = "your-auth-token";
const VENUE_NAME: &str = "txyz"; // an active venue from {"type":"outcomeMeta"}

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

    // Subscribe to ORDERS for one outcome venue, tagged and signer-enriched.
    let mut filters = HashMap::new();
    // Reserved key: expanded server-side to the venue's coin set.
    // Also accepted: "venues", "deployer", "deployers" (address).
    filters.insert(
        "venue".to_string(),
        FilterValues {
            values: vec![VENUE_NAME.to_string()],
        },
    );

    let subscribe = StreamSubscribe {
        stream_type: StreamType::Orders as i32,
        start_block: 0,
        filters,
        filter_name: format!("hip4-{VENUE_NAME}"),
        // Adds "signer" to each order (requires a server with signer
        // enrichment enabled; testnet has it on).
        enrichment: Some(EnrichmentOptions {
            include_signer: true,
        }),
        // Echoed on every update for this stream type.
        subscription_id: "hip4-orders-demo".to_string(),
    };

    tx.send(SubscribeRequest {
        request: Some(hyperliquid::subscribe_request::Request::Subscribe(
            subscribe,
        )),
    })
    .await?;

    // Keep-alive pings
    let ping_tx = tx.clone();
    tokio::spawn(async move {
        loop {
            tokio::time::sleep(std::time::Duration::from_secs(30)).await;
            let ping = SubscribeRequest {
                request: Some(hyperliquid::subscribe_request::Request::Ping(Ping {
                    timestamp: chrono_now_millis(),
                })),
            };
            if ping_tx.send(ping).await.is_err() {
                break;
            }
        }
    });

    let mut request = Request::new(ReceiverStream::new(rx));
    request
        .metadata_mut()
        .insert("x-token", MetadataValue::try_from(AUTH_TOKEN)?);

    println!("Streaming ORDERS for venue \"{VENUE_NAME}\" with signer enrichment\n");

    let mut stream = client.stream_data(request).await?.into_inner();

    while let Some(update) = stream.message().await? {
        let Some(hyperliquid::subscribe_update::Update::Data(data)) = update.update else {
            continue; // pong
        };
        let payload = decompress(data.data.as_bytes())?;

        // Every update says which subscription it belongs to.
        println!(
            "[block {}] streamType={:?} subscriptionId={:?}",
            data.block_number,
            StreamType::try_from(data.stream_type).unwrap_or(StreamType::Unknown),
            data.subscription_id
        );

        let Ok(entries) = serde_json::from_str::<Vec<serde_json::Value>>(&payload) else {
            println!("{payload}");
            continue;
        };
        for entry in entries {
            let coin = entry
                .pointer("/order/order/coin")
                .and_then(|v| v.as_str())
                .unwrap_or("");
            let user = entry
                .pointer("/order/user")
                .and_then(|v| v.as_str())
                .unwrap_or("");
            // "signer" is present because of EnrichmentOptions above.
            println!(
                "  coin={coin} user={user} signer={} status={}",
                entry.get("signer").unwrap_or(&serde_json::Value::Null),
                entry.get("status").unwrap_or(&serde_json::Value::Null)
            );
        }
    }

    Ok(())
}

fn chrono_now_millis() -> i64 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map(|d| d.as_millis() as i64)
        .unwrap_or(0)
}
