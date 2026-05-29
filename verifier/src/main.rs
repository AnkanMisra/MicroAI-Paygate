use axum::extract::rejection::JsonRejection;
use axum::extract::{DefaultBodyLimit, State};
use axum::{
    extract::Json,
    http::{HeaderMap, StatusCode},
    routing::{get, post},
    Router,
};
use dashmap::{mapref::entry::Entry, DashMap};
use ethers::types::transaction::eip712::TypedData;
use ethers::types::Signature;

use ethers::utils::keccak256;

mod metrics;

use metrics_exporter_prometheus::{PrometheusBuilder, PrometheusHandle};

use serde::{Deserialize, Serialize};
use std::env;
use std::net::SocketAddr;
use std::str::FromStr;
use std::sync::{Arc, Mutex};
use std::time::{Duration, Instant, SystemTime, UNIX_EPOCH};

const MAX_BODY_SIZE: usize = 1024 * 1024; // 1MB
const DEFAULT_EXPECTED_CHAIN_ID: u64 = 84532;
const NONCE_SWEEP_INTERVAL_SECONDS: u64 = 60;

#[derive(Clone)]
struct AppState {
    max_body_size: usize,
    expected_chain_id: u64,
    used_nonces: Arc<DashMap<[u8; 32], Instant>>,
    last_nonce_sweep: Arc<Mutex<Instant>>,
    signature_expiry_seconds: u64,
    clock_skew_seconds: u64,
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

fn parse_chain_id_env(key: &str) -> Option<u64> {
    match std::env::var(key) {
        Ok(v) => match v.parse() {
            Ok(chain_id) if chain_id > 0 => Some(chain_id),
            Ok(_) => {
                eprintln!("Warning: {} must be > 0, ignoring value", key);
                None
            }
            Err(_) => {
                eprintln!("Warning: Invalid {} '{}', ignoring value", key, v);
                None
            }
        },
        Err(_) => None,
    }
}

fn get_expected_chain_id() -> u64 {
    if std::env::var("EXPECTED_CHAIN_ID").is_ok() {
        return parse_chain_id_env("EXPECTED_CHAIN_ID").unwrap_or_else(|| {
            eprintln!(
                "Warning: EXPECTED_CHAIN_ID invalid, using default {}",
                DEFAULT_EXPECTED_CHAIN_ID
            );
            DEFAULT_EXPECTED_CHAIN_ID
        });
    }

    parse_chain_id_env("CHAIN_ID").unwrap_or(DEFAULT_EXPECTED_CHAIN_ID)
}

#[tokio::main]
async fn main() {
    let limit = get_max_body_size();
    let expected_chain_id = get_expected_chain_id();
    let state = AppState {
        max_body_size: limit,
        expected_chain_id,
        used_nonces: Arc::new(DashMap::new()),
        last_nonce_sweep: Arc::new(Mutex::new(Instant::now())),
        signature_expiry_seconds: get_env_u64("SIGNATURE_EXPIRY_SECONDS", 300),
        clock_skew_seconds: get_env_u64("SIGNATURE_CLOCK_SKEW_SECONDS", 60),
    };
    let recorder = PrometheusBuilder::new()
        .install_recorder()
        .expect("failed to install recorder");
    spawn_metrics_upkeep(recorder.clone());

    let app = Router::new()
        .route("/health", get(health))
        .route("/verify", post(verify_signature))
        .route("/metrics", get(metrics_route(recorder)))
        .layer(DefaultBodyLimit::max(limit))
        .with_state(state);

    let addr = SocketAddr::from(([0, 0, 0, 0], 3002));
    println!("Rust Verifier listening on {}", addr);

    let listener = tokio::net::TcpListener::bind(addr).await.unwrap();
    axum::serve(listener, app).await.unwrap();
}

fn metrics_route(
    handle: PrometheusHandle,
) -> impl Fn() -> std::future::Ready<String> + Clone + Send + Sync + 'static {
    move || std::future::ready(handle.clone().render())
}

fn spawn_metrics_upkeep(handle: PrometheusHandle) {
    tokio::spawn(async move {
        let mut interval = tokio::time::interval(Duration::from_secs(60));
        loop {
            interval.tick().await;
            handle.run_upkeep();
        }
    });
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

#[derive(Deserialize, Debug, Clone)]
struct VerifyRequest {
    context: PaymentContext,
    signature: String,
}

#[derive(Deserialize, Debug, Clone)]
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
    #[serde(skip_serializing_if = "Option::is_none")]
    error_code: Option<String>,
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

fn validate_timestamp(
    timestamp: Option<u64>,
    window_seconds: u64,
    clock_skew_seconds: u64,
) -> Result<(), VerifyError> {
    let now = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap()
        .as_secs();

    validate_timestamp_internal(timestamp, window_seconds, clock_skew_seconds, now)
}

fn evict_expired_nonces(store: &DashMap<[u8; 32], Instant>, now: Instant, ttl: Duration) {
    store.retain(|_, inserted_at| now.saturating_duration_since(*inserted_at) <= ttl);
}

fn nonce_retention_ttl(state: &AppState) -> Duration {
    Duration::from_secs(
        state
            .signature_expiry_seconds
            .saturating_add(state.clock_skew_seconds)
            .saturating_add(1),
    )
}

fn maybe_evict_expired_nonces(state: &AppState, now: Instant, ttl: Duration) {
    let mut last_sweep = state
        .last_nonce_sweep
        .lock()
        .unwrap_or_else(|e| e.into_inner());
    if now.saturating_duration_since(*last_sweep)
        < Duration::from_secs(NONCE_SWEEP_INTERVAL_SECONDS)
    {
        return;
    }
    *last_sweep = now;
    drop(last_sweep);
    evict_expired_nonces(&state.used_nonces, now, ttl);
}

fn claim_nonce(state: &AppState, nonce: &str, now: Instant) -> bool {
    let ttl = nonce_retention_ttl(state);
    maybe_evict_expired_nonces(state, now, ttl);

    // Single-instance replay protection. Multi-replica production needs Redis.
    match state.used_nonces.entry(keccak256(nonce.as_bytes())) {
        Entry::Occupied(mut entry) => {
            if now.saturating_duration_since(*entry.get()) > ttl {
                entry.insert(now);
                true
            } else {
                false
            }
        }
        Entry::Vacant(entry) => {
            entry.insert(now);
            true
        }
    }
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

    let request_start = std::time::Instant::now();
    ::metrics::counter!("verifier_requests_total").increment(1);

    // 2. Security Check: Match the payload result immediately
    let payload = match payload {
        Ok(Json(p)) => p, // Everything is good, proceed with payload 'p'
        Err(JsonRejection::BytesRejection(_)) => {
            println!("[CID: {}] Rejected: Payload too large", cid);

            record_verification_failure(&request_start, "payload_too_large");

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
                    error_code: None,
                }),
            );
        }
        Err(e) => {
            println!("[CID: {}] Rejected: Invalid JSON or formatting", cid);

            record_verification_failure(&request_start, "invalid_json");

            return (
                StatusCode::BAD_REQUEST,
                res_headers,
                Json(VerifyResponse {
                    is_valid: false,
                    recovered_address: None,
                    error: Some(format!("Invalid request: {}", e)),
                    error_code: None,
                }),
            );
        }
    };

    // 3. Now that we have a safe payload, proceed with your existing logic
    println!("[CID: {}] Verify nonce={}", cid, payload.context.nonce);

    if payload.context.chain_id != state.expected_chain_id {
        record_verification_failure(&request_start, "chain_id_mismatch");

        return (
            StatusCode::BAD_REQUEST,
            res_headers,
            Json(VerifyResponse {
                is_valid: false,
                recovered_address: None,
                error: Some("chain ID mismatch".to_string()),
                error_code: Some("chain_id_mismatch".to_string()),
            }),
        );
    }

    if let Err(err) = validate_timestamp(
        payload.context.timestamp,
        state.signature_expiry_seconds,
        state.clock_skew_seconds,
    ) {
        let (msg, error_code) = match err {
            VerifyError::SignatureExpired {
                age_seconds,
                max_seconds,
            } => (
                format!("E007: expired (age={} max={})", age_seconds, max_seconds),
                "timestamp_expired",
            ),
            VerifyError::FutureTimestamp { timestamp, now } => (
                format!("E008: future ts={} now={}", timestamp, now),
                "timestamp_future",
            ),
            VerifyError::MissingTimestamp => {
                ("E009: missing timestamp".to_string(), "timestamp_missing")
            }
        };

        record_verification_failure(&request_start, error_code);

        return (
            StatusCode::OK,
            res_headers,
            Json(VerifyResponse {
                is_valid: false,
                recovered_address: None,
                error: Some(msg),
                error_code: Some(error_code.to_string()),
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
            record_verification_failure(&request_start, "typed_data_error");
            return (
                StatusCode::BAD_REQUEST,
                res_headers,
                Json(VerifyResponse {
                    is_valid: false,
                    recovered_address: None,
                    error: Some(format!("typed data error: {}", e)),
                    error_code: None,
                }),
            );
        }
    };

    let sig = match Signature::from_str(&payload.signature) {
        Ok(s) => s,
        Err(e) => {
            record_verification_failure(&request_start, "invalid_signature");
            return (
                StatusCode::BAD_REQUEST,
                res_headers,
                Json(VerifyResponse {
                    is_valid: false,
                    recovered_address: None,
                    error: Some(format!("bad signature: {}", e)),
                    error_code: Some("invalid_signature".to_string()),
                }),
            );
        }
    };

    //let start = std::time::Instant::now();

    let result = sig.recover_typed_data(&typed_data);
    let duration = request_start.elapsed().as_secs_f64();

    match result {
        Ok(addr) => {
            if !claim_nonce(&state, &payload.context.nonce, Instant::now()) {
                metrics::record_verification(false, duration, Some("nonce_already_used"));
                return (
                    StatusCode::CONFLICT,
                    res_headers,
                    Json(VerifyResponse {
                        is_valid: false,
                        recovered_address: Some(format!("{:?}", addr)),
                        error: Some("nonce already used".to_string()),
                        error_code: Some("nonce_already_used".to_string()),
                    }),
                );
            }
            metrics::record_verification(true, duration, None);
            (
                StatusCode::OK,
                res_headers,
                Json(VerifyResponse {
                    is_valid: true,
                    recovered_address: Some(format!("{:?}", addr)),
                    error: None,
                    error_code: None,
                }),
            )
        }
        Err(e) => {
            metrics::record_verification(false, duration, Some("invalid_signature"));
            (
                StatusCode::OK,
                res_headers,
                Json(VerifyResponse {
                    is_valid: false,
                    recovered_address: None,
                    error: Some(e.to_string()),
                    error_code: Some("invalid_signature".to_string()),
                }),
            )
        }
    }
}

fn record_verification_failure(request_start: &Instant, reason: &'static str) {
    metrics::record_verification(false, request_start.elapsed().as_secs_f64(), Some(reason));
}

/* =======================
   Tests
======================= */

#[cfg(test)]
mod tests {
    use super::*;
    use dashmap::DashMap;
    use ethers::signers::{LocalWallet, Signer};
    use ethers::types::transaction::eip712::TypedData;
    use std::sync::Arc;

    const BASE_SEPOLIA_CHAIN_ID: u64 = 84532;
    static ENV_LOCK: Mutex<()> = Mutex::new(());

    fn app_state() -> AppState {
        app_state_with_window(300, 60)
    }

    fn app_state_with_window(signature_expiry_seconds: u64, clock_skew_seconds: u64) -> AppState {
        AppState {
            max_body_size: MAX_BODY_SIZE,
            expected_chain_id: BASE_SEPOLIA_CHAIN_ID,
            used_nonces: Arc::new(DashMap::new()),
            last_nonce_sweep: Arc::new(Mutex::new(Instant::now())),
            signature_expiry_seconds,
            clock_skew_seconds,
        }
    }

    fn now() -> u64 {
        SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_secs()
    }

    fn with_chain_env(
        expected_chain_id: Option<&str>,
        chain_id: Option<&str>,
        test: impl FnOnce(),
    ) {
        let _guard = ENV_LOCK.lock().unwrap();
        let old_expected = env::var("EXPECTED_CHAIN_ID").ok();
        let old_chain = env::var("CHAIN_ID").ok();

        match expected_chain_id {
            Some(value) => env::set_var("EXPECTED_CHAIN_ID", value),
            None => env::remove_var("EXPECTED_CHAIN_ID"),
        }
        match chain_id {
            Some(value) => env::set_var("CHAIN_ID", value),
            None => env::remove_var("CHAIN_ID"),
        }

        test();

        match old_expected {
            Some(value) => env::set_var("EXPECTED_CHAIN_ID", value),
            None => env::remove_var("EXPECTED_CHAIN_ID"),
        }
        match old_chain {
            Some(value) => env::set_var("CHAIN_ID", value),
            None => env::remove_var("CHAIN_ID"),
        }
    }

    #[test]
    fn test_get_expected_chain_id_defaults_to_base_sepolia() {
        with_chain_env(None, None, || {
            assert_eq!(get_expected_chain_id(), BASE_SEPOLIA_CHAIN_ID);
        });
    }

    #[test]
    fn test_get_expected_chain_id_falls_back_to_chain_id_when_expected_unset() {
        with_chain_env(None, Some("8453"), || {
            assert_eq!(get_expected_chain_id(), 8453);
        });
    }

    #[test]
    fn test_get_expected_chain_id_prefers_expected_chain_id() {
        with_chain_env(Some("84532"), Some("8453"), || {
            assert_eq!(get_expected_chain_id(), BASE_SEPOLIA_CHAIN_ID);
        });
    }

    #[test]
    fn test_get_expected_chain_id_ignores_invalid_expected_chain_id() {
        with_chain_env(Some("0"), Some("8453"), || {
            assert_eq!(get_expected_chain_id(), BASE_SEPOLIA_CHAIN_ID);
        });
    }

    async fn signed_request(nonce: &str, chain_id: u64, timestamp: u64) -> VerifyRequest {
        let wallet: LocalWallet =
            "380eb0f3d505f087e438eca80bc4df9a7faa24f868e69fc0440261a0fc0567dc"
                .parse()
                .unwrap();
        let wallet = wallet.with_chain_id(chain_id);

        let typed = serde_json::json!({
            "domain": {
                "name": "MicroAI Paygate",
                "version": "1",
                "chainId": chain_id,
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
                "nonce": nonce,
                "timestamp": timestamp
            }
        });

        let typed: TypedData = serde_json::from_value(typed).unwrap();
        let sig = wallet.sign_typed_data(&typed).await.unwrap();

        VerifyRequest {
            context: PaymentContext {
                recipient: "0x1234567890123456789012345678901234567890".into(),
                token: "USDC".into(),
                amount: "100".into(),
                nonce: nonce.into(),
                chain_id,
                timestamp: Some(timestamp),
            },
            signature: format!("0x{}", hex::encode(sig.to_vec())),
        }
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

        let wallet = wallet.with_chain_id(BASE_SEPOLIA_CHAIN_ID);

        let ts = now();
        let typed = serde_json::json!({
            "domain": {
                "name": "MicroAI Paygate",
                "version": "1",
                "chainId": BASE_SEPOLIA_CHAIN_ID,
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
                chain_id: BASE_SEPOLIA_CHAIN_ID,
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
    async fn test_verify_signature_rejects_wrong_chain_id() {
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
                "nonce": "wrong-chain-nonce",
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
                nonce: "wrong-chain-nonce".into(),
                chain_id: 1,
                timestamp: Some(ts),
            },
            signature: format!("0x{}", hex::encode(sig.to_vec())),
        };

        let (status, _, Json(resp)) =
            verify_signature(State(app_state()), HeaderMap::new(), Ok(Json(req))).await;

        assert_eq!(status, StatusCode::BAD_REQUEST);
        assert!(!resp.is_valid);
        assert_eq!(resp.recovered_address, None);
        assert_eq!(resp.error.as_deref(), Some("chain ID mismatch"));
        assert_eq!(resp.error_code.as_deref(), Some("chain_id_mismatch"));
    }

    #[tokio::test]
    async fn test_verify_signature_returns_timestamp_error_codes() {
        let state = app_state();
        let cases = [
            (None, "timestamp_missing"),
            (Some(now() - 301), "timestamp_expired"),
            (Some(now() + 120), "timestamp_future"),
        ];

        for (timestamp, expected_code) in cases {
            let req = VerifyRequest {
                context: PaymentContext {
                    recipient: "0x1234567890123456789012345678901234567890".to_string(),
                    token: "USDC".to_string(),
                    amount: "100".to_string(),
                    nonce: format!("timestamp-{expected_code}"),
                    chain_id: BASE_SEPOLIA_CHAIN_ID,
                    timestamp,
                },
                signature: "0x1234567890".to_string(),
            };

            let (status, _, Json(resp)) =
                verify_signature(State(state.clone()), HeaderMap::new(), Ok(Json(req))).await;

            assert_eq!(status, StatusCode::OK);
            assert!(!resp.is_valid);
            assert_eq!(resp.error_code.as_deref(), Some(expected_code));
        }
    }

    #[tokio::test]
    async fn test_verify_signature_rejects_replayed_nonce() {
        let state = app_state();
        let req = signed_request("replay-nonce", BASE_SEPOLIA_CHAIN_ID, now()).await;

        let (first_status, _, Json(first_resp)) = verify_signature(
            State(state.clone()),
            HeaderMap::new(),
            Ok(Json(req.clone())),
        )
        .await;
        let (second_status, _, Json(second_resp)) =
            verify_signature(State(state), HeaderMap::new(), Ok(Json(req))).await;

        assert_eq!(first_status, StatusCode::OK);
        assert!(first_resp.is_valid);
        assert_eq!(second_status, StatusCode::CONFLICT);
        assert!(!second_resp.is_valid);
        assert_eq!(
            second_resp.error_code.as_deref(),
            Some("nonce_already_used")
        );
    }

    #[tokio::test]
    async fn test_verify_signature_allows_one_concurrent_duplicate_nonce() {
        let state = app_state();
        let req = signed_request("concurrent-replay-nonce", BASE_SEPOLIA_CHAIN_ID, now()).await;
        let mut handles = Vec::new();

        for _ in 0..100 {
            let state = state.clone();
            let req = req.clone();
            handles.push(tokio::spawn(async move {
                let (status, _, Json(resp)) =
                    verify_signature(State(state), HeaderMap::new(), Ok(Json(req))).await;
                (status, resp.error_code)
            }));
        }

        let mut successes = 0;
        let mut conflicts = 0;
        for handle in handles {
            let (status, error_code) = handle.await.unwrap();
            match status {
                StatusCode::OK => successes += 1,
                StatusCode::CONFLICT => {
                    assert_eq!(error_code.as_deref(), Some("nonce_already_used"));
                    conflicts += 1;
                }
                other => panic!("unexpected status: {}", other),
            }
        }

        assert_eq!(successes, 1);
        assert_eq!(conflicts, 99);
    }

    #[test]
    fn test_claim_nonce_retains_entries_through_clock_skew_window() {
        let state = app_state_with_window(1, 2);
        let start = Instant::now();

        assert!(claim_nonce(&state, "ttl-replay-nonce", start));
        assert!(!claim_nonce(
            &state,
            "ttl-replay-nonce",
            start + Duration::from_millis(1100)
        ));
        assert!(!claim_nonce(
            &state,
            "ttl-replay-nonce",
            start + Duration::from_millis(3100)
        ));
        assert!(!claim_nonce(
            &state,
            "ttl-replay-nonce",
            start + Duration::from_millis(4000)
        ));
        assert!(claim_nonce(
            &state,
            "ttl-replay-nonce",
            start + Duration::from_millis(4100)
        ));
    }

    #[tokio::test]
    async fn test_verify_signature_invalid_signature_does_not_burn_nonce() {
        let state = app_state();
        let mut bad_req =
            signed_request("invalid-does-not-burn", BASE_SEPOLIA_CHAIN_ID, now()).await;
        let good_req = bad_req.clone();
        bad_req.signature = format!("0x{}", "00".repeat(65));

        let (bad_status, _, Json(bad_resp)) =
            verify_signature(State(state.clone()), HeaderMap::new(), Ok(Json(bad_req))).await;
        let (good_status, _, Json(good_resp)) =
            verify_signature(State(state), HeaderMap::new(), Ok(Json(good_req))).await;

        assert_eq!(bad_status, StatusCode::OK);
        assert!(!bad_resp.is_valid);
        assert_eq!(bad_resp.error_code.as_deref(), Some("invalid_signature"));
        assert_eq!(good_status, StatusCode::OK);
        assert!(good_resp.is_valid);
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
                chain_id: BASE_SEPOLIA_CHAIN_ID,
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
                chain_id: BASE_SEPOLIA_CHAIN_ID,
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
                chain_id: BASE_SEPOLIA_CHAIN_ID,
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
        let wallet = wallet.with_chain_id(BASE_SEPOLIA_CHAIN_ID);

        let ts = now();
        let json_typed_data = serde_json::json!({
            "domain": {
                "name": "MicroAI Paygate",
                "version": "1",
                "chainId": BASE_SEPOLIA_CHAIN_ID,
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
                chain_id: BASE_SEPOLIA_CHAIN_ID,
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
                chain_id: BASE_SEPOLIA_CHAIN_ID,
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
            expected_chain_id: BASE_SEPOLIA_CHAIN_ID,
            used_nonces: Arc::new(DashMap::new()),
            last_nonce_sweep: Arc::new(Mutex::new(Instant::now())),
            signature_expiry_seconds: 300,
            clock_skew_seconds: 60,
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

    #[tokio::test(flavor = "current_thread")]
    async fn test_verify_signature_records_specific_failure_reasons() {
        let recorder = PrometheusBuilder::new().build_recorder();
        let handle = recorder.handle();
        let _guard = ::metrics::set_default_local_recorder(&recorder);

        let wrong_chain = signed_request("metrics-wrong-chain", 1, now()).await;
        let (status, _, Json(resp)) =
            verify_signature(State(app_state()), HeaderMap::new(), Ok(Json(wrong_chain))).await;
        assert_eq!(status, StatusCode::BAD_REQUEST);
        assert_eq!(resp.error_code.as_deref(), Some("chain_id_mismatch"));

        let missing_timestamp = VerifyRequest {
            context: PaymentContext {
                recipient: "0x1234567890123456789012345678901234567890".to_string(),
                token: "USDC".to_string(),
                amount: "100".to_string(),
                nonce: "metrics-missing-timestamp".to_string(),
                chain_id: BASE_SEPOLIA_CHAIN_ID,
                timestamp: None,
            },
            signature: "0x1234567890".to_string(),
        };
        let (status, _, Json(resp)) = verify_signature(
            State(app_state()),
            HeaderMap::new(),
            Ok(Json(missing_timestamp)),
        )
        .await;
        assert_eq!(status, StatusCode::OK);
        assert_eq!(resp.error_code.as_deref(), Some("timestamp_missing"));

        let malformed_signature = VerifyRequest {
            context: PaymentContext {
                recipient: "0x1234567890123456789012345678901234567890".to_string(),
                token: "USDC".to_string(),
                amount: "100".to_string(),
                nonce: "metrics-malformed-signature".to_string(),
                chain_id: BASE_SEPOLIA_CHAIN_ID,
                timestamp: Some(now()),
            },
            signature: "not-a-signature".to_string(),
        };
        let (status, _, Json(resp)) = verify_signature(
            State(app_state()),
            HeaderMap::new(),
            Ok(Json(malformed_signature)),
        )
        .await;
        assert_eq!(status, StatusCode::BAD_REQUEST);
        assert_eq!(resp.error_code.as_deref(), Some("invalid_signature"));

        let rendered = handle.render();
        assert!(
            rendered.contains("verifier_signature_invalid_total{reason=\"chain_id_mismatch\"} 1")
        );
        assert!(
            rendered.contains("verifier_signature_invalid_total{reason=\"timestamp_missing\"} 1")
        );
        assert!(
            rendered.contains("verifier_signature_invalid_total{reason=\"invalid_signature\"} 1")
        );
        assert!(
            !rendered.contains("verifier_signature_invalid_total{reason=\"payload_too_large\"}")
        );
    }

    #[tokio::test]
    async fn test_metrics_route_can_be_scraped_repeatedly() {
        use axum::{body::Body, http::Request};
        use tower::ServiceExt;

        let recorder = PrometheusBuilder::new().build_recorder();
        let app = Router::new().route("/metrics", get(metrics_route(recorder.handle())));

        for _ in 0..2 {
            let response = app
                .clone()
                .oneshot(
                    Request::builder()
                        .method("GET")
                        .uri("/metrics")
                        .body(Body::empty())
                        .unwrap(),
                )
                .await
                .unwrap();
            assert_eq!(response.status(), StatusCode::OK);
        }
    }
}
