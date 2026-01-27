use axum::{
    extract::Json,
    http::{HeaderMap, StatusCode},
    routing::{get, post},
    Router,
};
use ethers::types::transaction::eip712::TypedData;
use ethers::types::Signature;
use serde::{Deserialize, Serialize};
use std::env;
use std::net::SocketAddr;
use std::str::FromStr;
use std::time::{SystemTime, UNIX_EPOCH};

#[tokio::main]
async fn main() {
    let app = Router::new()
        .route("/health", get(health))
        .route("/verify", post(verify_signature));

    let addr = SocketAddr::from(([0, 0, 0, 0], 3002));
    println!("Rust Verifier listening on {}", addr);

    let listener = tokio::net::TcpListener::bind(addr).await.unwrap();
    axum::serve(listener, app).await.unwrap();
}

async fn health(headers: HeaderMap) -> (HeaderMap, Json<HealthResponse>) {
    let (_, res_headers) = correlation_id_headers(&headers);

    (
        res_headers,
        Json(HealthResponse {
            status: "healthy",
            service: "verifier",
            version: env!("CARGO_PKG_VERSION"),
        }),
    )
}

/* =======================
   Request / Response
======================= */

#[derive(Deserialize, Debug)]
struct VerifyRequest {
    context: PaymentContext,
    signature: String,
}

#[derive(Deserialize, Debug)]
struct PaymentContext {
    recipient: String,
    token: String,
    amount: String,
    nonce: String,
    #[serde(rename = "chainId")]
    chain_id: u64,
    timestamp: Option<u64>,
}

#[derive(Serialize)]
struct VerifyResponse {
    is_valid: bool,
    recovered_address: Option<String>,
    error: Option<String>,
}

#[derive(Serialize)]
struct HealthResponse {
    status: &'static str,
    service: &'static str,
    version: &'static str,
}

/* =======================
   Correlation ID
======================= */

fn correlation_id_headers(headers: &HeaderMap) -> (String, HeaderMap) {
    let correlation_id = headers
        .get("X-Correlation-ID")
        .and_then(|v| v.to_str().ok())
        .unwrap_or("unknown");

    let mut res_headers = HeaderMap::new();
    if let Ok(val) = correlation_id.parse() {
        res_headers.insert("X-Correlation-ID", val);
    }

    (correlation_id.to_string(), res_headers)
}

/* =======================
   Timestamp Validation
======================= */

#[derive(Debug)]
enum VerifyError {
    SignatureExpired { age_seconds: u64, max_seconds: u64 },
    FutureTimestamp { timestamp: u64, now: u64 },
    MissingTimestamp,
}

fn get_env_u64(key: &str, default: u64) -> u64 {
    env::var(key)
        .ok()
        .and_then(|v| v.parse().ok())
        .unwrap_or(default)
}

fn validate_timestamp_internal(
    timestamp: Option<u64>,
    window_seconds: u64,
    clock_skew_seconds: u64,
    now: u64,
) -> Result<(), VerifyError> {
    let ts = timestamp.ok_or(VerifyError::MissingTimestamp)?;

    if ts > now.saturating_add(clock_skew_seconds) {
        return Err(VerifyError::FutureTimestamp { timestamp: ts, now });
    }

    let age = now.saturating_sub(ts);
    if age > window_seconds {
        return Err(VerifyError::SignatureExpired {
            age_seconds: age,
            max_seconds: window_seconds,
        });
    }

    Ok(())
}

fn validate_timestamp(timestamp: Option<u64>) -> Result<(), VerifyError> {
    let window = get_env_u64("SIGNATURE_EXPIRY_SECONDS", 300);
    let skew = get_env_u64("SIGNATURE_CLOCK_SKEW_SECONDS", 60);

    let now = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap()
        .as_secs();

    validate_timestamp_internal(timestamp, window, skew, now)
}

/* =======================
   Signature Verification
======================= */

async fn verify_signature(
    headers: HeaderMap,
    Json(payload): Json<VerifyRequest>,
) -> (StatusCode, HeaderMap, Json<VerifyResponse>) {
    let (cid, res_headers) = correlation_id_headers(&headers);

    println!("[CID: {}] Verify nonce={}", cid, payload.context.nonce);

    if let Err(err) = validate_timestamp(payload.context.timestamp) {
        let msg = match err {
            VerifyError::SignatureExpired {
                age_seconds,
                max_seconds,
            } => format!("E007: expired (age={} max={})", age_seconds, max_seconds),
            VerifyError::FutureTimestamp { timestamp, now } => {
                format!("E008: future ts={} now={}", timestamp, now)
            }
            VerifyError::MissingTimestamp => "E009: missing timestamp".to_string(),
        };

        return (
            StatusCode::OK,
            res_headers,
            Json(VerifyResponse {
                is_valid: false,
                recovered_address: None,
                error: Some(msg),
            }),
        );
    }

    let typed_data_json = serde_json::json!({
        "domain": {
            "name": "MicroAI Paygate",
            "version": "1",
            "chainId": payload.context.chain_id,
            "verifyingContract": "0x0000000000000000000000000000000000000000"
        },
        "types": {
            "Payment": [
                { "name": "recipient", "type": "address" },
                { "name": "token", "type": "string" },
                { "name": "amount", "type": "string" },
                { "name": "nonce", "type": "string" },
                { "name": "timestamp", "type": "uint256" }
            ]
        },
        "primaryType": "Payment",
        "message": {
            "recipient": payload.context.recipient,
            "token": payload.context.token,
            "amount": payload.context.amount,
            "nonce": payload.context.nonce,
            "timestamp": payload.context.timestamp
        }
    });

    let typed_data: TypedData = match serde_json::from_value(typed_data_json) {
        Ok(td) => td,
        Err(e) => {
            return (
                StatusCode::BAD_REQUEST,
                res_headers,
                Json(VerifyResponse {
                    is_valid: false,
                    recovered_address: None,
                    error: Some(format!("typed data error: {}", e)),
                }),
            );
        }
    };

    let sig = match Signature::from_str(&payload.signature) {
        Ok(s) => s,
        Err(e) => {
            return (
                StatusCode::BAD_REQUEST,
                res_headers,
                Json(VerifyResponse {
                    is_valid: false,
                    recovered_address: None,
                    error: Some(format!("bad signature: {}", e)),
                }),
            );
        }
    };

    match sig.recover_typed_data(&typed_data) {
        Ok(addr) => (
            StatusCode::OK,
            res_headers,
            Json(VerifyResponse {
                is_valid: true,
                recovered_address: Some(format!("{:?}", addr)),
                error: None,
            }),
        ),
        Err(e) => (
            StatusCode::OK,
            res_headers,
            Json(VerifyResponse {
                is_valid: false,
                recovered_address: None,
                error: Some(e.to_string()),
            }),
        ),
    }
}

/* =======================
   Tests
======================= */

#[cfg(test)]
mod tests {
    use super::*;
    use ethers::signers::{LocalWallet, Signer};

    fn now() -> u64 {
        SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_secs()
    }

    #[test]
    fn test_timestamp_valid() {
        let n = now();
        assert!(validate_timestamp_internal(Some(n), 300, 60, n).is_ok());
    }

    #[test]
    fn test_timestamp_expired() {
        let n = now();
        let res = validate_timestamp_internal(Some(n - 1000), 300, 60, n);
        assert!(matches!(res, Err(VerifyError::SignatureExpired { .. })));
    }

    #[tokio::test]
    async fn test_verify_signature_valid() {
        let wallet: LocalWallet =
            "380eb0f3d505f087e438eca80bc4df9a7faa24f868e69fc0440261a0fc0567dc"
                .parse()
                .unwrap();

        let wallet = wallet.with_chain_id(1u64);

        let ts = now();
        let typed = serde_json::json!({
            "domain": {
                "name": "MicroAI Paygate",
                "version": "1",
                "chainId": 1,
                "verifyingContract": "0x0000000000000000000000000000000000000000"
            },
            "types": {
                "Payment": [
                    { "name": "recipient", "type": "address" },
                    { "name": "token", "type": "string" },
                    { "name": "amount", "type": "string" },
                    { "name": "nonce", "type": "string" },
                    { "name": "timestamp", "type": "uint256" }
                ]
            },
            "primaryType": "Payment",
            "message": {
                "recipient": "0x1234567890123456789012345678901234567890",
                "token": "USDC",
                "amount": "100",
                "nonce": "nonce-1",
                "timestamp": ts
            }
        });

        let typed: TypedData = serde_json::from_value(typed).unwrap();
        let sig = wallet.sign_typed_data(&typed).await.unwrap();

        let req = VerifyRequest {
            context: PaymentContext {
                recipient: "0x1234567890123456789012345678901234567890".into(),
                token: "USDC".into(),
                amount: "100".into(),
                nonce: "nonce-1".into(),
                chain_id: 1,
                timestamp: Some(ts),
            },
            signature: format!("0x{}", hex::encode(sig.to_vec())),
        };

        let (status, _, Json(resp)) = verify_signature(HeaderMap::new(), Json(req)).await;

        assert_eq!(status, StatusCode::OK);
        assert!(resp.is_valid);
    }
}
