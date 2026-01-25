use axum::{
    extract::Json,
    http::StatusCode,
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

async fn health() -> &'static str {
    "Rust Verifier OK"
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
    Json(payload): Json<VerifyRequest>,
) -> (StatusCode, Json<VerifyResponse>) {

    println!(
        "Received verification request for nonce: {}",
        payload.context.nonce
    );

    // ✅ Timestamp validation (resolved conflict)
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
            Json(VerifyResponse {
                is_valid: false,
                recovered_address: None,
                error: Some(error_message),
            }),
        );
    }

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
                Json(VerifyResponse {
                    is_valid: false,
                    recovered_address: None,
                    error: Some(format!("Failed to build typed data: {}", e)),
                }),
            )
        }
    };

    let signature = match Signature::from_str(&payload.signature) {
        Ok(s) => s,
        Err(e) => {
            return (
                StatusCode::BAD_REQUEST,
                Json(VerifyResponse {
                    is_valid: false,
                    recovered_address: None,
                    error: Some(format!("Invalid signature format: {}", e)),
                }),
            )
        }
    };

    match signature.recover_typed_data(&typed_data) {
        Ok(address) => (
            StatusCode::OK,
            Json(VerifyResponse {
                is_valid: true,
                recovered_address: Some(format!("{:?}", address)),
                error: None,
            }),
        ),
        Err(e) => (
            StatusCode::OK,
            Json(VerifyResponse {
                is_valid: false,
                recovered_address: None,
                error: Some(format!("Verification failed: {}", e)),
            }),
        ),
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use ethers::signers::{LocalWallet, Signer};

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
                "timestamp": 1_700_000_000u64
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
                timestamp: Some(1_700_000_000u64),
            },
            signature: signature_str,
        };

        let (status, Json(response)) = verify_signature(Json(req)).await;
        assert_eq!(status, StatusCode::OK);
        assert!(response.is_valid);
    }
}
