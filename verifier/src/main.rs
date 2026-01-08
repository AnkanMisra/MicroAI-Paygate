use axum::{
    extract::Json,
    http::{HeaderMap, StatusCode}, // VIBE FIX: Added HeaderMap to read headers
    routing::{get, post},
    Router,
};
use ethers::types::transaction::eip712::TypedData;
use ethers::types::Signature;
use serde::{Deserialize, Serialize};
use std::net::SocketAddr;
use std::str::FromStr;

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

#[derive(Serialize)]
struct VerifyResponse {
    is_valid: bool,
    recovered_address: Option<String>,
    error: Option<String>,
}

async fn verify_signature(
    headers: HeaderMap,
    Json(payload): Json<VerifyRequest>,
) -> (StatusCode, HeaderMap, Json<VerifyResponse>) {
    
    // Extract ID
    let correlation_id = headers
        .get("X-Correlation-ID")
        .and_then(|v| v.to_str().ok())
        .unwrap_or("unknown");

    // Prepare response header
    let mut res_headers = HeaderMap::new();
    if let Ok(header_value) = correlation_id.parse() {
        res_headers.insert("X-Correlation-ID", header_value);
    }

    println!(
        "[CorrelationID: {}] Received verification request for nonce: {}",
        correlation_id, payload.context.nonce
    );

    // Reconstruct Typed Data (Domain, Types, Value)
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

    let typed_data_json = serde_json::json!({
        "domain": domain,
        "types": types,
        "primaryType": "Payment",
        "message": value
    });

    // Parse TypedData
    let typed_data: TypedData = match serde_json::from_value(typed_data_json) {
        Ok(td) => td,
        Err(e) => {
            return (
                StatusCode::BAD_REQUEST,
                res_headers, // Header added
                Json(VerifyResponse {
                    is_valid: false,
                    recovered_address: None,
                    error: Some(format!("Failed to build typed data: {}", e)),
                }),
            )
        }
    };

    // Parse Signature
    let signature = match Signature::from_str(&payload.signature) {
        Ok(s) => s,
        Err(e) => {
            return (
                StatusCode::BAD_REQUEST,
                res_headers, // Header added
                Json(VerifyResponse {
                    is_valid: false,
                    recovered_address: None,
                    error: Some(format!("Invalid signature format: {}", e)),
                }),
            )
        }
    };

    // Final Verification
    match signature.recover_typed_data(&typed_data) {
        Ok(address) => {
            println!("[CorrelationID: {}] Signature valid! Recovered: {:?}", correlation_id, address);
            (
                StatusCode::OK,
                res_headers, // Header added
                Json(VerifyResponse {
                    is_valid: true,
                    recovered_address: Some(format!("{:?}", address)),
                    error: None,
                }),
            )
        }
        Err(e) => {
            println!("[CorrelationID: {}] Verification failed: {}", correlation_id, e);
            (
                StatusCode::OK,
                res_headers, // Header added
                Json(VerifyResponse {
                    is_valid: false,
                    recovered_address: None,
                    error: Some(format!("Verification failed: {}", e)),
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
        let wallet: LocalWallet =
            "380eb0f3d505f087e438eca80bc4df9a7faa24f868e69fc0440261a0fc0567dc"
                .parse()
                .unwrap();
        let wallet = wallet.with_chain_id(1u64);

        // Construct TypedData via JSON (easiest way without derive macros)
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

        // For tests, we pass empty headers
        let (status, _headers, Json(response)) = verify_signature(HeaderMap::new(), Json(req)).await;

        assert_eq!(status, StatusCode::OK);
        assert!(response.is_valid);
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
            },
            signature: "0x1234567890".to_string(),
        };

        let (status, _headers, Json(response)) = verify_signature(HeaderMap::new(), Json(req)).await;
        assert_eq!(status, StatusCode::BAD_REQUEST);
    }
}