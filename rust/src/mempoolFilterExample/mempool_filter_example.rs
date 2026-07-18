use clap::Parser;
use serde_json::Value;
use std::collections::{HashMap, HashSet};
use std::time::Duration;
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

const DEFAULT_GRPC_ENDPOINT: &str = "your-endpoint.hype-mainnet.quiknode.pro:10000";
const DEFAULT_AUTH_TOKEN: &str = "YOUR_QUICKNODE_TOKEN";
const ZSTD_MAGIC: [u8; 4] = [0x28, 0xb5, 0x2f, 0xfd];
const HEARTBEAT_SECONDS: u64 = 30;

#[derive(Debug, PartialEq, Eq)]
struct TouchingAction {
    action_type: String,
    asset_ids: Vec<String>,
}

fn signed_actions(value: &Value) -> &[Value] {
    let tx = value
        .as_array()
        .and_then(|items| items.get(1))
        .unwrap_or(value);
    tx.get("signed_actions")
        .and_then(Value::as_array)
        .map(Vec::as_slice)
        .unwrap_or(&[])
}

fn asset_id(value: Option<&Value>) -> Option<String> {
    match value? {
        Value::Number(number) => number.as_u64().map(|id| id.to_string()),
        Value::String(raw) if !raw.is_empty() && raw.bytes().all(|byte| byte.is_ascii_digit()) => {
            Some(raw.clone())
        }
        _ => None,
    }
}

fn direct_assets(value: Option<&Value>) -> Vec<String> {
    let Some(value) = value else {
        return Vec::new();
    };
    ["a", "asset"]
        .iter()
        .filter_map(|field| asset_id(value.get(field)))
        .collect()
}

fn append_unique(target: &mut Vec<String>, values: impl IntoIterator<Item = String>) {
    for value in values {
        if !target.contains(&value) {
            target.push(value);
        }
    }
}

fn order_touching_actions(value: &Value) -> Vec<TouchingAction> {
    let mut matches = Vec::new();
    for signed in signed_actions(value) {
        let Some(action) = signed.get("action") else {
            continue;
        };
        let Some(action_type) = action.get("type").and_then(Value::as_str) else {
            continue;
        };
        let mut assets = Vec::new();
        match action_type {
            "order" => {
                for item in action
                    .get("orders")
                    .and_then(Value::as_array)
                    .into_iter()
                    .flatten()
                {
                    append_unique(&mut assets, direct_assets(Some(item)));
                }
            }
            "cancel" | "cancelByCloid" => {
                for item in action
                    .get("cancels")
                    .and_then(Value::as_array)
                    .into_iter()
                    .flatten()
                {
                    append_unique(&mut assets, direct_assets(Some(item)));
                }
            }
            "batchModify" => {
                for item in action
                    .get("modifies")
                    .and_then(Value::as_array)
                    .into_iter()
                    .flatten()
                {
                    append_unique(&mut assets, direct_assets(item.get("order")));
                    append_unique(&mut assets, direct_assets(Some(item)));
                }
            }
            "modify" => {
                append_unique(&mut assets, direct_assets(action.get("order")));
                append_unique(&mut assets, direct_assets(Some(action)));
            }
            "twapOrder" => append_unique(&mut assets, direct_assets(action.get("twap"))),
            "twapCancel" => append_unique(&mut assets, direct_assets(Some(action))),
            _ => {}
        }
        if !assets.is_empty() {
            matches.push(TouchingAction {
                action_type: action_type.to_string(),
                asset_ids: assets,
            });
        }
    }
    matches
}

fn order_touching_asset_ids(value: &Value) -> Vec<String> {
    order_touching_actions(value)
        .into_iter()
        .flat_map(|action| action.asset_ids)
        .collect()
}

fn endpoint() -> String {
    std::env::var("GRPC_ENDPOINT").unwrap_or_else(|_| DEFAULT_GRPC_ENDPOINT.to_string())
}

fn endpoint_uri() -> String {
    let endpoint = endpoint();
    if endpoint.starts_with("http://") || endpoint.starts_with("https://") {
        endpoint
    } else {
        format!("https://{endpoint}")
    }
}

fn auth_token() -> String {
    std::env::var("AUTH_TOKEN")
        .or_else(|_| std::env::var("QN_AUTH_TOKEN"))
        .unwrap_or_else(|_| DEFAULT_AUTH_TOKEN.to_string())
}

async fn create_channel() -> Result<Channel, Box<dyn std::error::Error>> {
    Ok(Channel::from_shared(endpoint_uri())?
        .tls_config(ClientTlsConfig::new())?
        .http2_keep_alive_interval(Duration::from_secs(HEARTBEAT_SECONDS))
        .keep_alive_timeout(Duration::from_secs(10))
        .keep_alive_while_idle(true)
        .connect()
        .await?)
}

fn ping_request(timestamp: i64) -> SubscribeRequest {
    SubscribeRequest {
        request: Some(hyperliquid::subscribe_request::Request::Ping(Ping {
            timestamp,
        })),
    }
}

fn decode_data(data: &[u8]) -> Result<String, Box<dyn std::error::Error>> {
    if data.len() >= 4 && data[0..4] == ZSTD_MAGIC {
        return Ok(String::from_utf8(zstd::decode_all(data)?)?);
    }
    Ok(String::from_utf8_lossy(data).to_string())
}

#[derive(Parser)]
#[command(name = "mempool-filter-example")]
#[command(about = "Validate server-side coin filtering on raw Hyperliquid MEMPOOL_TXS")]
struct Args {
    #[arg(long, value_delimiter = ',', default_value = "BTC")]
    coin: Vec<String>,

    #[arg(long, default_value = "coin", value_parser = ["coin", "coins"])]
    filter_field: String,

    #[arg(long, value_delimiter = ',', default_value = "0")]
    expected_asset_ids: Vec<String>,

    #[arg(long, default_value_t = 5)]
    max_messages: usize,

    #[arg(long, default_value_t = 60)]
    timeout_seconds: u64,

    #[arg(long)]
    unfiltered: bool,

    #[arg(long)]
    expect_no_match: bool,

    #[arg(long)]
    print_raw: bool,
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let args = Args::parse();
    if endpoint() == DEFAULT_GRPC_ENDPOINT || auth_token() == DEFAULT_AUTH_TOKEN {
        return Err("set GRPC_ENDPOINT and AUTH_TOKEN (or QN_AUTH_TOKEN)".into());
    }
    if args.max_messages == 0 || args.timeout_seconds == 0 {
        return Err("max-messages and timeout-seconds must be positive".into());
    }
    if args.unfiltered && args.expect_no_match {
        return Err("unfiltered and expect-no-match cannot be combined".into());
    }

    let channel = create_channel().await?;
    let mut client = StreamingClient::new(channel).max_decoding_message_size(100 * 1024 * 1024);
    let (tx, rx) = mpsc::channel(32);
    let mut filters = HashMap::new();
    if !args.unfiltered {
        filters.insert(
            args.filter_field.clone(),
            FilterValues {
                values: args.coin.clone(),
            },
        );
    }
    tx.send(SubscribeRequest {
        request: Some(hyperliquid::subscribe_request::Request::Subscribe(
            StreamSubscribe {
                stream_type: StreamType::MempoolTxs as i32,
                start_block: 0,
                filters,
                filter_name: "mempool-coin-filter".to_string(),
            },
        )),
    })
    .await?;

    let ping_tx = tx.clone();
    tokio::spawn(async move {
        let period = Duration::from_secs(HEARTBEAT_SECONDS);
        let mut interval = tokio::time::interval_at(tokio::time::Instant::now() + period, period);
        loop {
            interval.tick().await;
            let timestamp = chrono::Utc::now().timestamp_millis();
            println!("PING timestamp={timestamp}");
            if ping_tx.send(ping_request(timestamp)).await.is_err() {
                return;
            }
        }
    });

    let mut request = Request::new(ReceiverStream::new(rx));
    request
        .metadata_mut()
        .insert("x-token", auth_token().parse::<MetadataValue<_>>()?);
    let mut responses = client.stream_data(request).await?.into_inner();
    let expected: HashSet<String> = args.expected_asset_ids.iter().cloned().collect();
    let deadline = tokio::time::Instant::now() + Duration::from_secs(args.timeout_seconds);
    let mut received = 0usize;

    loop {
        let remaining = deadline.saturating_duration_since(tokio::time::Instant::now());
        if remaining.is_zero() {
            if args.expect_no_match {
                println!(
                    "PASS: no MEMPOOL_TXS messages matched within {}s",
                    args.timeout_seconds
                );
                return Ok(());
            }
            return Err(format!("timed out after {received} message(s)").into());
        }
        let response = match tokio::time::timeout(remaining, responses.message()).await {
            Err(_) if args.expect_no_match => {
                println!(
                    "PASS: no MEMPOOL_TXS messages matched within {}s",
                    args.timeout_seconds
                );
                return Ok(());
            }
            Err(_) => return Err(format!("timed out after {received} message(s)").into()),
            Ok(result) => result?,
        };
        let Some(response) = response else {
            return Err(format!("stream ended after {received} message(s)").into());
        };
        let data = match response.update {
            Some(hyperliquid::subscribe_update::Update::Data(data)) => data,
            Some(hyperliquid::subscribe_update::Update::Pong(pong)) => {
                println!("PONG timestamp={}", pong.timestamp);
                continue;
            }
            None => continue,
        };
        if args.expect_no_match {
            return Err("deliberately non-matching coin returned a transaction".into());
        }

        let raw = decode_data(data.data.as_bytes())?;
        let value: Value = serde_json::from_str(&raw)?;
        let observed = order_touching_asset_ids(&value);
        let mut matches: Vec<String> = observed
            .iter()
            .filter(|id| expected.contains(*id))
            .cloned()
            .collect();
        matches.sort();
        matches.dedup();
        if !args.unfiltered && matches.is_empty() {
            return Err(
                format!("raw transaction lacks expected asset; observed={observed:?}").into(),
            );
        }
        received += 1;
        let action_summary = order_touching_actions(&value)
            .iter()
            .map(|action| format!("{}:{}", action.action_type, action.asset_ids.join("|")))
            .collect::<Vec<_>>()
            .join(", ");
        println!(
            "message {}/{}: expected_asset_matches={:?} order_touching=[{}] bytes={}",
            received,
            args.max_messages,
            matches,
            action_summary,
            raw.len()
        );
        if args.print_raw {
            println!("{raw}");
        }
        if received >= args.max_messages {
            println!("PASS: received {received} raw mempool message(s)");
            return Ok(());
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    fn fixture(object_root: bool) -> Value {
        let tx = json!({
            "tx_hash": "0xraw",
            "signed_actions": [
                {"action": {"type": "order", "orders": [{"a": 0}]}},
                {"action": {"type": "cancel", "cancels": [{"a": "5"}]}},
                {"action": {"type": "cancelByCloid", "cancels": [{"asset": 0}]}},
                {"action": {"type": "batchModify", "modifies": [{"order": {"a": "0"}}]}},
                {"action": {"type": "modify", "order": {"asset": 0}}},
                {"action": {"type": "twapOrder", "twap": {"a": 0}}},
                {"action": {"type": "twapCancel", "asset": 0}},
                {"action": {"type": "noop"}}
            ]
        });
        if object_root {
            tx
        } else {
            json!(["2026-07-17T00:00:00Z", tx])
        }
    }

    #[test]
    fn extracts_every_order_touching_action_without_mutating_raw_tuple() {
        let raw = fixture(false);
        let before = raw.clone();
        let actions = order_touching_actions(&raw);
        assert_eq!(
            actions
                .iter()
                .map(|action| action.action_type.as_str())
                .collect::<Vec<_>>(),
            vec![
                "order",
                "cancel",
                "cancelByCloid",
                "batchModify",
                "modify",
                "twapOrder",
                "twapCancel"
            ]
        );
        assert_eq!(
            order_touching_asset_ids(&raw)
                .into_iter()
                .collect::<HashSet<_>>(),
            HashSet::from(["0".to_string(), "5".to_string()])
        );
        assert_eq!(raw, before);
    }

    #[test]
    fn supports_object_root_payloads() {
        assert!(order_touching_asset_ids(&fixture(true)).contains(&"0".to_string()));
    }

    #[test]
    fn ignores_non_order_actions_and_invalid_assets() {
        let raw = json!({"signed_actions": [
            {"action": {"type": "order", "orders": [{"a": -1}, {"a": "BTC"}]}},
            {"action": {"type": "noop", "a": 0}}
        ]});
        assert!(order_touching_actions(&raw).is_empty());
    }

    #[test]
    fn builds_ping_request() {
        let request = ping_request(123_456_789);
        let Some(hyperliquid::subscribe_request::Request::Ping(ping)) = request.request else {
            panic!("expected ping request");
        };
        assert_eq!(ping.timestamp, 123_456_789);
    }
}
