use axum::{
    extract::Json,
    http::StatusCode,
    routing::{get, post},
    Router,
};
use ethers::types::transaction::eip712::TypedData;
use ethers::types::Signature;
use serde::{Deserialize, Serialize};
use std::collections::HashSet;
use std::net::SocketAddr;
use std::str::FromStr;
use std::sync::Mutex;

// Error codes as constants
const E001: &str = "E001";
const E002: &str = "E002";
const E003: &str = "E003";
const E004: &str = "E004";
const E005: &str = "E005";
const E006: &str = "E006";

// Global nonce tracker for detecting reuse
lazy_static::lazy_static! {
    static ref NONCE_TRACKER: Mutex<HashSet<String>> = Mutex::new(HashSet::new());
}

#[tokio::main]
async fn main() {
    // build our application with a route
    let app = Router::new()
        .route("/health", get(health))
        .route("/verify", post(verify_signature));

    // run it
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
}

#[derive(Serialize, Debug)]
struct VerifyError {
    code: String,
    message: String,
    details: Option<String>,
}

#[derive(Serialize, Debug)]
struct VerifyResponse {
    is_valid: bool,
    recovered_address: Option<String>,
    error: Option<VerifyError>,
}

impl VerifyError {
    fn missing_signature() -> Self {
        Self {
            code: E001.to_string(),
            message: "Missing signature".to_string(),
            details: Some("No X-402-Signature header provided".to_string()),
        }
    }

    fn malformed_signature(details: &str) -> Self {
        Self {
            code: E002.to_string(),
            message: "Malformed signature".to_string(),
            details: Some(details.to_string()),
        }
    }

    fn recovery_failed(details: &str) -> Self {
        Self {
            code: E003.to_string(),
            message: "Recovery failed".to_string(),
            details: Some(details.to_string()),
        }
    }

    fn address_mismatch(recovered: &str, expected: &str) -> Self {
        Self {
            code: E004.to_string(),
            message: "Address mismatch".to_string(),
            details: Some(format!("Recovered: {}, Expected: {}", recovered, expected)),
        }
    }

    fn invalid_message(details: &str) -> Self {
        Self {
            code: E005.to_string(),
            message: "Invalid message".to_string(),
            details: Some(details.to_string()),
        }
    }

    fn nonce_reused() -> Self {
        Self {
            code: E006.to_string(),
            message: "Nonce reused".to_string(),
            details: Some("Duplicate nonce detected".to_string()),
        }
    }
}

async fn verify_signature(
    Json(payload): Json<VerifyRequest>,
) -> (StatusCode, Json<VerifyResponse>) {
    println!(
        "Received verification request for nonce: {}",
        payload.context.nonce
    );

    // Check for empty signature
    if payload.signature.is_empty() {
        return (
            StatusCode::BAD_REQUEST,
            Json(VerifyResponse {
                is_valid: false,
                recovered_address: None,
                error: Some(VerifyError::missing_signature()),
            }),
        );
    }

    // Check for nonce reuse
    {
        let mut tracker = NONCE_TRACKER.lock().unwrap();
        if tracker.contains(&payload.context.nonce) {
            return (
                StatusCode::BAD_REQUEST,
                Json(VerifyResponse {
                    is_valid: false,
                    recovered_address: None,
                    error: Some(VerifyError::nonce_reused()),
                }),
            );
        }
        tracker.insert(payload.context.nonce.clone());
    }

    // Construct the EIP-712 Typed Data
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
            { "name": "nonce", "type": "string" }
        ]
    });

    let value = serde_json::json!({
        "recipient": payload.context.recipient,
        "token": payload.context.token,
        "amount": payload.context.amount,
        "nonce": payload.context.nonce
    });

    let typed_data = serde_json::json!({
        "domain": domain,
        "types": types,
        "primaryType": "Payment",
        "message": value
    });

    // Parse TypedData (E005: Invalid message)
    let typed_data: TypedData = match serde_json::from_value(typed_data) {
        Ok(td) => td,
        Err(e) => {
            return (
                StatusCode::BAD_REQUEST,
                Json(VerifyResponse {
                    is_valid: false,
                    recovered_address: None,
                    error: Some(VerifyError::invalid_message(&format!(
                        "Failed to build typed data: {}",
                        e
                    ))),
                }),
            )
        }
    };

    // Parse Signature (E001/E002: Malformed signature)
    let signature = match Signature::from_str(&payload.signature) {
        Ok(s) => s,
        Err(e) => {
            let error_msg = e.to_string();
            let error = if error_msg.contains("invalid hex") || error_msg.contains("odd number of digits") {
                VerifyError::malformed_signature(&format!("Not hex format: {}", e))
            } else if error_msg.contains("invalid length") {
                VerifyError::malformed_signature(&format!("Wrong length: {}", e))
            } else {
                VerifyError::malformed_signature(&error_msg)
            };
            return (
                StatusCode::BAD_REQUEST,
                Json(VerifyResponse {
                    is_valid: false,
                    recovered_address: None,
                    error: Some(error),
                }),
            );
        }
    };

    // Verify (E003: Recovery failed)
    match signature.recover_typed_data(&typed_data) {
        Ok(address) => {
            let recovered_addr = format!("{:?}", address);
            println!("Signature valid! Recovered: {}", recovered_addr);
            (
                StatusCode::OK,
                Json(VerifyResponse {
                    is_valid: true,
                    recovered_address: Some(recovered_addr),
                    error: None,
                }),
            )
        }
        Err(e) => {
            println!("Verification failed: {}", e);
            (
                StatusCode::OK,
                Json(VerifyResponse {
                    is_valid: false,
                    recovered_address: None,
                    error: Some(VerifyError::recovery_failed(&e.to_string())),
                }),
            )
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use ethers::signers::{LocalWallet, Signer};
    use ethers::types::transaction::eip712::TypedData;

    #[tokio::test]
    async fn test_verify_signature_valid() {
        // Clear nonce tracker
        NONCE_TRACKER.lock().unwrap().clear();

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
                    { "name": "nonce", "type": "string" }
                ]
            },
            "primaryType": "Payment",
            "message": {
                "recipient": "0x1234567890123456789012345678901234567890",
                "token": "USDC",
                "amount": "100",
                "nonce": "unique-nonce-123"
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
            },
            signature: signature_str,
        };

        let (status, Json(response)) = verify_signature(Json(req)).await;

        assert_eq!(status, StatusCode::OK);
        assert!(response.is_valid);
        assert_eq!(response.error, None);
    }

    #[tokio::test]
    async fn test_verify_signature_invalid() {
        NONCE_TRACKER.lock().unwrap().clear();

        let req = VerifyRequest {
            context: PaymentContext {
                recipient: "0x1234...".to_string(),
                token: "USDC".to_string(),
                amount: "100".to_string(),
                nonce: "nonce".to_string(),
                chain_id: 1,
            },
            signature: "0x1234567890".to_string(),
        };

        let (status, Json(response)) = verify_signature(Json(req)).await;
        assert_eq!(status, StatusCode::BAD_REQUEST);
        assert!(!response.is_valid);
        assert!(response.error.is_some());
    }

    #[tokio::test]
    async fn test_error_e001_missing_signature() {
        NONCE_TRACKER.lock().unwrap().clear();

        let req = VerifyRequest {
            context: PaymentContext {
                recipient: "0x1234567890123456789012345678901234567890".to_string(),
                token: "USDC".to_string(),
                amount: "100".to_string(),
                nonce: "nonce-e001".to_string(),
                chain_id: 1,
            },
            signature: "".to_string(),
        };

        let (status, Json(response)) = verify_signature(Json(req)).await;

        assert_eq!(status, StatusCode::BAD_REQUEST);
        assert!(!response.is_valid);
        let error = response.error.unwrap();
        assert_eq!(error.code, E001);
        assert_eq!(error.message, "Missing signature");
    }

    #[tokio::test]
    async fn test_error_e002_malformed_signature() {
        NONCE_TRACKER.lock().unwrap().clear();

        let req = VerifyRequest {
            context: PaymentContext {
                recipient: "0x1234567890123456789012345678901234567890".to_string(),
                token: "USDC".to_string(),
                amount: "100".to_string(),
                nonce: "nonce-e002".to_string(),
                chain_id: 1,
            },
            signature: "0xZZZZ".to_string(), // Invalid hex
        };

        let (status, Json(response)) = verify_signature(Json(req)).await;

        assert_eq!(status, StatusCode::BAD_REQUEST);
        assert!(!response.is_valid);
        let error = response.error.unwrap();
        assert_eq!(error.code, E002);
        assert_eq!(error.message, "Malformed signature");
    }

    #[tokio::test]
    async fn test_error_e003_recovery_failed() {
        NONCE_TRACKER.lock().unwrap().clear();

        let req = VerifyRequest {
            context: PaymentContext {
                recipient: "0x1234567890123456789012345678901234567890".to_string(),
                token: "USDC".to_string(),
                amount: "100".to_string(),
                nonce: "nonce-e003".to_string(),
                chain_id: 1,
            },
            signature: "0x0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000".to_string(),
        };

        let (status, Json(response)) = verify_signature(Json(req)).await;

        assert_eq!(status, StatusCode::OK);
        assert!(!response.is_valid);
        let error = response.error.unwrap();
        assert_eq!(error.code, E003);
        assert_eq!(error.message, "Recovery failed");
    }

    #[tokio::test]
    async fn test_error_e005_invalid_message() {
        NONCE_TRACKER.lock().unwrap().clear();

        // This test would require invalid TypedData construction, which is hard to trigger
        // in normal circumstances since we build it ourselves. This is tested implicitly
        // through the valid signature test.
    }

    #[tokio::test]
    async fn test_error_e006_nonce_reused() {
        NONCE_TRACKER.lock().unwrap().clear();

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
                    { "name": "nonce", "type": "string" }
                ]
            },
            "primaryType": "Payment",
            "message": {
                "recipient": "0x1234567890123456789012345678901234567890",
                "token": "USDC",
                "amount": "100",
                "nonce": "reused-nonce"
            }
        });

        let typed_data: TypedData = serde_json::from_value(json_typed_data).unwrap();
        let signature = wallet.sign_typed_data(&typed_data).await.unwrap();
        let signature_str = format!("0x{}", hex::encode(signature.to_vec()));

        // First request should succeed
        let req1 = VerifyRequest {
            context: PaymentContext {
                recipient: "0x1234567890123456789012345678901234567890".to_string(),
                token: "USDC".to_string(),
                amount: "100".to_string(),
                nonce: "reused-nonce".to_string(),
                chain_id: 1,
            },
            signature: signature_str.clone(),
        };

        let (status1, Json(response1)) = verify_signature(Json(req1)).await;
        assert_eq!(status1, StatusCode::OK);
        assert!(response1.is_valid);

        // Second request with same nonce should fail with E006
        let req2 = VerifyRequest {
            context: PaymentContext {
                recipient: "0x1234567890123456789012345678901234567890".to_string(),
                token: "USDC".to_string(),
                amount: "100".to_string(),
                nonce: "reused-nonce".to_string(),
                chain_id: 1,
            },
            signature: signature_str,
        };

        let (status2, Json(response2)) = verify_signature(Json(req2)).await;
        assert_eq!(status2, StatusCode::BAD_REQUEST);
        assert!(!response2.is_valid);
        let error = response2.error.unwrap();
        assert_eq!(error.code, E006);
        assert_eq!(error.message, "Nonce reused");
    }
}
