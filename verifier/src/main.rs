use axum::extract::rejection::JsonRejection;
use axum::extract::{DefaultBodyLimit, State};
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

const MAX_BODY_SIZE: usize = 1024 * 1024; // 1MB

#[derive(Clone)]
struct AppState {
    max_body_size: usize,
}

fn get_max_body_size() -> usize {
    match std::env::var("MAX_REQUEST_BODY_BYTES") {
        Ok(v) => match v.parse() {
            Ok(size) if size > 0 => size, // Only accept positive numbers
            Ok(_) => {
                eprintln!(
                    "Warning: MAX_REQUEST_BODY_BYTES must be > 0, using default {}",
                    MAX_BODY_SIZE
                );
                MAX_BODY_SIZE
            }
            Err(_) => {
                eprintln!(
                    "Warning: Invalid MAX_REQUEST_BODY_BYTES '{}', using default {}",
                    v, MAX_BODY_SIZE
                );
                MAX_BODY_SIZE
            }
        },
        Err(_) => MAX_BODY_SIZE,
    }
}

#[tokio::main]
async fn main() {
    let limit = get_max_body_size();
    let state = AppState {
        max_body_size: limit,
    };
    let app = Router::new()
        .route("/health", get(health))
        .route("/verify", post(verify_signature))
        .layer(DefaultBodyLimit::max(limit))
        .with_state(state);

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
    State(state): State<AppState>,
    headers: HeaderMap,
    payload: Result<Json<VerifyRequest>, JsonRejection>,
) -> (StatusCode, HeaderMap, Json<VerifyResponse>) {
    // 1. Get correlation ID headers first so we can use them in error responses
    let (cid, res_headers) = correlation_id_headers(&headers);

    // 2. Security Check: Match the payload result immediately
    let payload = match payload {
        Ok(Json(p)) => p, // Everything is good, proceed with payload 'p'
        Err(JsonRejection::BytesRejection(_)) => {
            println!("[CID: {}] Rejected: Payload too large", cid);
            return (
                StatusCode::PAYLOAD_TOO_LARGE,
                res_headers,
                Json(VerifyResponse {
                    is_valid: false,
                    recovered_address: None,
                    error: Some(format!(
                        "Request body too large (max {} bytes)",
                        state.max_body_size
                    )),
                }),
            );
        }
        Err(e) => {
            println!("[CID: {}] Rejected: Invalid JSON or formatting", cid);
            return (
                StatusCode::BAD_REQUEST,
                res_headers,
                Json(VerifyResponse {
                    is_valid: false,
                    recovered_address: None,
                    error: Some(format!("Invalid request: {}", e)),
                }),
            );
        }
    };

    // 3. Now that we have a safe payload, proceed with your existing logic
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
    use ethers::types::transaction::eip712::TypedData;

    fn app_state() -> AppState {
        AppState {
            max_body_size: MAX_BODY_SIZE,
        }
    }

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

    #[test]
    fn test_timestamp_future() {
        let n = now();
        // Timestamp 120 seconds in the future (beyond 60s clock skew grace)
        let res = validate_timestamp_internal(Some(n + 120), 300, 60, n);
        assert!(matches!(res, Err(VerifyError::FutureTimestamp { .. })));
    }

    #[test]
    fn test_timestamp_missing() {
        let n = now();
        // No timestamp provided
        let res = validate_timestamp_internal(None, 300, 60, n);
        assert!(matches!(res, Err(VerifyError::MissingTimestamp)));
    }

    #[test]
    fn test_timestamp_within_clock_skew() {
        let n = now();
        // Timestamp 30 seconds in the future (within 60s grace period) - should be valid
        let res = validate_timestamp_internal(Some(n + 30), 300, 60, n);
        assert!(res.is_ok());
    }

    #[test]
    fn test_timestamp_boundary() {
        let n = now();
        // Exactly at 300s window boundary - should be valid
        let res = validate_timestamp_internal(Some(n - 300), 300, 60, n);
        assert!(res.is_ok());

        // One second past boundary (301s) - should be expired
        let res = validate_timestamp_internal(Some(n - 301), 300, 60, n);
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

        let (status, _, Json(resp)) =
            verify_signature(State(app_state()), HeaderMap::new(), Ok(Json(req))).await;

        assert_eq!(status, StatusCode::OK);
        assert!(resp.is_valid);
    }

    #[tokio::test]
    async fn test_health_endpoint() {
        let (_headers, Json(response)) = health(HeaderMap::new()).await;

        assert_eq!(response.status, "healthy");
        assert_eq!(response.service, "verifier");
        assert_eq!(response.version, env!("CARGO_PKG_VERSION"));
    }

    #[tokio::test]
    async fn test_health_endpoint_correlation_id() {
        let mut headers = HeaderMap::new();
        headers.insert("X-Correlation-ID", "health-check-id".parse().unwrap());

        let (res_headers, Json(response)) = health(headers).await;

        assert_eq!(response.status, "healthy");

        let response_id = res_headers.get("X-Correlation-ID");
        assert!(response_id.is_some());
        assert_eq!(response_id.unwrap().to_str().unwrap(), "health-check-id");
    }

    #[tokio::test]
    async fn test_verify_signature_invalid() {
        let ts = now();
        let req = VerifyRequest {
            context: PaymentContext {
                recipient: "0x1234567890123456789012345678901234567890".to_string(),
                token: "USDC".to_string(),
                amount: "100".to_string(),
                nonce: "nonce".to_string(),
                chain_id: 1,
                timestamp: Some(ts),
            },
            signature: "0x1234567890".to_string(),
        };

        let (status, _headers, Json(_response)) =
            verify_signature(State(app_state()), HeaderMap::new(), Ok(Json(req))).await;
        assert_eq!(status, StatusCode::BAD_REQUEST);
    }

    #[tokio::test]
    async fn test_correlation_id_preserved_in_response() {
        let mut headers = HeaderMap::new();
        headers.insert(
            "X-Correlation-ID",
            "test-correlation-id-12345".parse().unwrap(),
        );

        let ts = now();
        let req = VerifyRequest {
            context: PaymentContext {
                recipient: "0x1234567890123456789012345678901234567890".to_string(),
                token: "USDC".to_string(),
                amount: "100".to_string(),
                nonce: "nonce".to_string(),
                chain_id: 1,
                timestamp: Some(ts),
            },
            signature: "0x1234567890".to_string(),
        };

        let (_status, response_headers, _json) =
            verify_signature(State(app_state()), headers, Ok(Json(req))).await;

        let response_id = response_headers.get("X-Correlation-ID");
        assert!(
            response_id.is_some(),
            "Expected X-Correlation-ID in response headers"
        );
        assert_eq!(
            response_id.unwrap().to_str().unwrap(),
            "test-correlation-id-12345",
            "Correlation ID should be preserved from request"
        );
    }

    #[tokio::test]
    async fn test_correlation_id_unknown_when_missing() {
        let headers = HeaderMap::new();

        let ts = now();
        let req = VerifyRequest {
            context: PaymentContext {
                recipient: "0x1234567890123456789012345678901234567890".to_string(),
                token: "USDC".to_string(),
                amount: "100".to_string(),
                nonce: "nonce".to_string(),
                chain_id: 1,
                timestamp: Some(ts),
            },
            signature: "0x1234567890".to_string(),
        };

        let (_status, response_headers, _json) =
            verify_signature(State(app_state()), headers, Ok(Json(req))).await;

        let response_id = response_headers.get("X-Correlation-ID");
        assert!(
            response_id.is_some(),
            "Expected X-Correlation-ID header even with unknown value"
        );
        assert_eq!(
            response_id.unwrap().to_str().unwrap(),
            "unknown",
            "Should use 'unknown' as fallback correlation ID"
        );
    }

    #[tokio::test]
    async fn test_correlation_id_with_valid_signature() {
        let wallet: LocalWallet =
            "380eb0f3d505f087e438eca80bc4df9a7faa24f868e69fc0440261a0fc0567dc"
                .parse()
                .unwrap();
        let wallet = wallet.with_chain_id(1u64);

        let ts = now();
        let json_typed_data = serde_json::json!({
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
                "nonce": "correlation-test-nonce",
                "timestamp": ts
            }
        });

        let typed_data: TypedData = serde_json::from_value(json_typed_data).unwrap();
        let signature = wallet.sign_typed_data(&typed_data).await.unwrap();
        let signature_str = format!("0x{}", hex::encode(signature.to_vec()));

        let mut headers = HeaderMap::new();
        headers.insert(
            "X-Correlation-ID",
            "valid-sig-correlation-id".parse().unwrap(),
        );

        let req = VerifyRequest {
            context: PaymentContext {
                recipient: "0x1234567890123456789012345678901234567890".to_string(),
                token: "USDC".to_string(),
                amount: "100".to_string(),
                nonce: "correlation-test-nonce".to_string(),
                chain_id: 1,
                timestamp: Some(ts),
            },
            signature: signature_str,
        };

        let (status, response_headers, Json(response)) =
            verify_signature(State(app_state()), headers, Ok(Json(req))).await;

        assert_eq!(status, StatusCode::OK);
        assert!(response.is_valid);

        let response_id = response_headers.get("X-Correlation-ID");
        assert!(
            response_id.is_some(),
            "Expected X-Correlation-ID in successful response"
        );
        assert_eq!(
            response_id.unwrap().to_str().unwrap(),
            "valid-sig-correlation-id",
            "Correlation ID should be preserved in successful response"
        );
    }

    #[tokio::test]
    async fn test_correlation_id_uuid_format() {
        let mut headers = HeaderMap::new();
        headers.insert(
            "X-Correlation-ID",
            "550e8400-e29b-41d4-a716-446655440000".parse().unwrap(),
        );

        let ts = now();
        let req = VerifyRequest {
            context: PaymentContext {
                recipient: "0x1234567890123456789012345678901234567890".to_string(),
                token: "USDC".to_string(),
                amount: "100".to_string(),
                nonce: "nonce".to_string(),
                chain_id: 1,
                timestamp: Some(ts),
            },
            signature: "0x1234567890".to_string(),
        };

        let (_status, response_headers, _json) =
            verify_signature(State(app_state()), headers, Ok(Json(req))).await;

        let response_id = response_headers.get("X-Correlation-ID");
        assert!(response_id.is_some());
        assert_eq!(
            response_id.unwrap().to_str().unwrap(),
            "550e8400-e29b-41d4-a716-446655440000",
            "UUID correlation ID should be preserved exactly"
        );
    }
    #[tokio::test]
    async fn test_verify_signature_rejection_paths() {
        use axum::extract::rejection::JsonRejection;

        // 1. Test a generic JSON rejection (e.g., bad formatting)
        // We simulate a "Missing Content-Type" style error
        let body_rejection = axum::extract::rejection::MissingJsonContentType::default();
        let rejection = JsonRejection::from(body_rejection);

        let (status, _, Json(resp)) =
            verify_signature(State(app_state()), HeaderMap::new(), Err(rejection)).await;

        assert_eq!(status, StatusCode::BAD_REQUEST);
        assert!(resp.error.unwrap().contains("Invalid request"));
    }
    #[tokio::test]
    async fn test_verify_signature_oversized_payload() {
        use axum::{
            body::Body,
            http::{Request, StatusCode},
        };
        use tower::ServiceExt; // for `oneshot`

        // 1. Force the limit to our constant (1MB) instead of reading the environment.
        // This makes the test deterministic.
        let limit = MAX_BODY_SIZE;
        let state = AppState {
            max_body_size: limit,
        };
        let app = Router::new()
            .route("/verify", post(verify_signature))
            .layer(DefaultBodyLimit::max(limit))
            .with_state(state);

        // 2. Create a "too large" payload (2MB) which is guaranteed to exceed 1MB.
        let large_data = vec![b'a'; 2 * 1024 * 1024];
        let req = Request::builder()
            .method("POST")
            .uri("/verify")
            .header("content-type", "application/json")
            .header("x-correlation-id", "test-oversized")
            .body(Body::from(large_data))
            .unwrap();

        // 3. Send the request through the app.
        let response = app.oneshot(req).await.unwrap();

        // 4. Verify the results
        assert_eq!(response.status(), StatusCode::PAYLOAD_TOO_LARGE); // 413
        assert!(response.headers().contains_key("x-correlation-id")); // Header check
    }
}
