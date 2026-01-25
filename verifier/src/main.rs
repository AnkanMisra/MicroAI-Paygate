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
    let (_correlation_id, res_headers) = correlation_id_headers(&headers);

    (
        res_headers,
        Json(HealthResponse {
            status: "healthy",
            service: "verifier",
            version: env!("CARGO_PKG_VERSION"),
        }),
    )
}

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

fn correlation_id_headers(headers: &HeaderMap) -> (String, HeaderMap) {
    // Extract correlation ID
    let correlation_id = headers
        .get("X-Correlation-ID")
        .and_then(|v| v.to_str().ok())
        .unwrap_or("unknown");

    let mut res_headers = HeaderMap::new();
    if let Ok(header_value) = correlation_id.parse() {
        res_headers.insert("X-Correlation-ID", header_value);
    }

    (correlation_id.to_string(), res_headers)
}

#[derive(Debug)]
enum VerifyError {
    SignatureExpired { age_seconds: u64, max_seconds: u64 },
    FutureTimestamp { timestamp: u64, now: u64 },
    MissingTimestamp,
}

fn get_env_u64(key: &str, default: u64) -> u64 {
    env::var(key)
        .ok()
        .and_then(|v| v.parse::<u64>().ok())
        .unwrap_or(default)
}

fn validate_timestamp_internal(
    timestamp: Option<u64>,
    window_seconds: u64,
    clock_skew_seconds: u64,
    now: u64,
) -> Result<(), VerifyError> {
    let ts = match timestamp {
        Some(t) => t,
        None => return Err(VerifyError::MissingTimestamp),
    };

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
    let window_seconds = get_env_u64("SIGNATURE_EXPIRY_SECONDS", 300);
    let clock_skew_seconds = get_env_u64("SIGNATURE_CLOCK_SKEW_SECONDS", 60);

    let now = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .expect("Time went backwards")
        .as_secs();

    validate_timestamp_internal(timestamp, window_seconds, clock_skew_seconds, now)
}

async fn verify_signature(
    headers: HeaderMap,
    Json(payload): Json<VerifyRequest>,
) -> (StatusCode, HeaderMap, Json<VerifyResponse>) {
    // Correlation ID propagation
    let (correlation_id, res_headers) = correlation_id_headers(&headers);

    println!(
        "[CorrelationID: {}] Received verification request for nonce: {}",
        correlation_id, payload.context.nonce
    );

    // Timestamp validation (HARD security gate)
    if let Err(err) = validate_timestamp(payload.context.timestamp) {
        let error_message = match err {
            VerifyError::SignatureExpired {
                age_seconds,
                max_seconds,
            } => format!(
                "E007: Signature expired (age={}s, max={}s)",
                age_seconds, max_seconds
            ),
            VerifyError::FutureTimestamp { timestamp, now } => format!(
                "E008: Future timestamp (timestamp={}, now={})",
                timestamp, now
            ),
            VerifyError::MissingTimestamp => "E009: Missing timestamp field".to_string(),
        };

        return (
            StatusCode::OK,
            res_headers,
            Json(VerifyResponse {
                is_valid: false,
                recovered_address: None,
                error: Some(error_message),
            }),
        );
    }

    // -------------------------------
    // Reconstruct EIP-712 Typed Data
    // -------------------------------
    let domain = serde_json::json!({
        "name": "MicroAI Paygate",
        "version": "1",
        "chainId": payload.context.chain_id,
        "verifyingContract": "0x0000000000000000000000000000000000000000"
    });

    let types = serde_json::json!({
        "Payment": [
            { "name": "recipient", "type": "address" },
            { "name": "token", "type": "string" },
            { "name": "amount", "type": "string" },
            { "name": "nonce", "type": "string" },
            { "name": "timestamp", "type": "uint256" }
        ]
    });

    let value = serde_json::json!({
        "recipient": payload.context.recipient,
        "token": payload.context.token,
        "amount": payload.context.amount,
        "nonce": payload.context.nonce,
        "timestamp": payload.context.timestamp
    });

    let typed_data_json = serde_json::json!({
        "domain": domain,
        "types": types,
        "primaryType": "Payment",
        "message": value
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
                    error: Some(format!("Failed to build typed data: {}", e)),
                }),
            );
        }
    };

    let signature = match Signature::from_str(&payload.signature) {
        Ok(s) => s,
        Err(e) => {
            return (
                StatusCode::BAD_REQUEST,
                res_headers,
                Json(VerifyResponse {
                    is_valid: false,
                    recovered_address: None,
                    error: Some(format!("Invalid signature format: {}", e)),
                }),
            );
        }
    };

    // -------------------------------
    // Final verification
    // -------------------------------
    match signature.recover_typed_data(&typed_data) {
        Ok(address) => {
            println!(
                "[CorrelationID: {}] Signature valid! Recovered: {:?}",
                correlation_id, address
            );
            (
                StatusCode::OK,
                res_headers,
                Json(VerifyResponse {
                    is_valid: true,
                    recovered_address: Some(format!("{:?}", address)),
                    error: None,
                }),
            )
        }
        Err(e) => {
            println!(
                "[CorrelationID: {}] Verification failed: {}",
                correlation_id, e
            );
            (
                StatusCode::OK,
                res_headers,
                Json(VerifyResponse {
                    is_valid: false,
                    recovered_address: None,
                    error: Some(format!("Verification failed: {}", e)),
                }),
            )
        }
    }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use ethers::signers::{LocalWallet, Signer};

    use std::time::{SystemTime, UNIX_EPOCH};

    fn current_timestamp() -> u64 {
        SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_secs()
    }

    #[tokio::test]
    async fn test_verify_signature_valid() {
        let wallet: LocalWallet =
            "380eb0f3d505f087e438eca80bc4df9a7faa24f868e69fc0440261a0fc0567dc"
                .parse()
                .unwrap();
        let wallet = wallet.with_chain_id(1u64);

        let json_typed_data = serde_json::json!({
            "domain": {
                "name": "MicroAI Paygate",
                "version": "1",
                "chainId": 1,
                "verifyingContract": "0x0000000000000000000000000000000000000000"
            },
            "types": {
                "EIP712Domain": [
                    { "name": "name", "type": "string" },
                    { "name": "version", "type": "string" },
                    { "name": "chainId", "type": "uint256" },
                    { "name": "verifyingContract", "type": "address" }
                ],
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
                "nonce": "unique-nonce-123",
                "timestamp": current_timestamp()
            }
        });

        let typed_data: TypedData = serde_json::from_value(json_typed_data).unwrap();
        let signature = wallet.sign_typed_data(&typed_data).await.unwrap();
        let signature_str = format!("0x{}", hex::encode(signature.to_vec()));

        let req = VerifyRequest {
            context: PaymentContext {
                recipient: "0x1234567890123456789012345678901234567890".to_string(),
                token: "USDC".to_string(),
                amount: "100".to_string(),
                nonce: "unique-nonce-123".to_string(),
                chain_id: 1,
                timestamp: Some(current_timestamp()),
            },
            signature: signature_str,
        };

        let (status, Json(response)) = verify_signature(Json(req)).await;
        assert_eq!(status, StatusCode::OK);
        assert!(response.is_valid);
<<<<<<< HEAD
=======
        assert_eq!(response.error, None);
    }

    #[tokio::test]
    async fn test_verify_signature_invalid() {
        let req = VerifyRequest {
            context: PaymentContext {
                recipient: "0x1234...".to_string(),
                token: "USDC".to_string(),
                amount: "100".to_string(),
                nonce: "nonce".to_string(),
                chain_id: 1,
                timestamp: Some(1_700_000_000u64),
            },
            signature: "0x1234567890".to_string(),
        };

        let (status, _) = verify_signature(Json(req)).await;
        assert_eq!(status, StatusCode::BAD_REQUEST);
    }

    #[tokio::test]
    async fn test_validate_timestamp_within_window() {
        let req = VerifyRequest {
            context: PaymentContext {
                recipient: "0x1234...".to_string(),
                token: "USDC".to_string(),
                amount: "100".to_string(),
                nonce: "valid-nonce".to_string(),
                chain_id: 1,
                timestamp: Some(current_timestamp()),
            },
            signature: "0x1234567890".to_string(),
        };

        let (status, _, Json(response)) =
            verify_signature(HeaderMap::new(), Json(req)).await;

        assert_eq!(status, StatusCode::OK);
        assert!(response.is_valid);
    }

    #[tokio::test]
    async fn test_validate_timestamp_expired() {
        let expired_ts = current_timestamp() - 1000;

        let req = VerifyRequest {
            context: PaymentContext {
                recipient: "0x1234...".to_string(),
                token: "USDC".to_string(),
                amount: "100".to_string(),
                nonce: "expired-nonce".to_string(),
                chain_id: 1,
                timestamp: Some(expired_ts),
            },
            signature: "0x1234567890".to_string(),
        };

        let (status, _, Json(response)) =
            verify_signature(HeaderMap::new(), Json(req)).await;

        assert_eq!(status, StatusCode::BAD_REQUEST);
        assert!(!response.is_valid);
    }

    #[tokio::test]
    async fn test_validate_timestamp_future() {
        let future_ts = current_timestamp() + 1000;

        let req = VerifyRequest {
            context: PaymentContext {
                recipient: "0x1234...".to_string(),
                token: "USDC".to_string(),
                amount: "100".to_string(),
                nonce: "future-nonce".to_string(),
                chain_id: 1,
                timestamp: Some(future_ts),
            },
            signature: "0x1234567890".to_string(),
        };

        let (status, _, Json(response)) =
            verify_signature(HeaderMap::new(), Json(req)).await;

        assert_eq!(status, StatusCode::BAD_REQUEST);
        assert!(!response.is_valid);
    }

    #[tokio::test]
    async fn test_validate_timestamp_missing() {
        let req = VerifyRequest {
            context: PaymentContext {
                recipient: "0x1234...".to_string(),
                token: "USDC".to_string(),
                amount: "100".to_string(),
                nonce: "missing-ts".to_string(),
                chain_id: 1,
                timestamp: None,
            },
            signature: "0x1234567890".to_string(),
        };

        let (status, _, Json(response)) =
            verify_signature(HeaderMap::new(), Json(req)).await;

        assert_eq!(status, StatusCode::BAD_REQUEST);
        assert!(!response.is_valid);
>>>>>>> 836a2de (Implement Greptile suggestions: dynamic timestamp in tests, add timestamp validation tests, and fix health endpoint duplication)
    }
}
