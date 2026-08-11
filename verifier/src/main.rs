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
use ethers::types::{Address, Signature};

use ethers::utils::keccak256;

mod metrics;

use metrics_exporter_prometheus::{PrometheusBuilder, PrometheusHandle};
use redis::AsyncCommands;

use serde::{Deserialize, Serialize};
use std::env;
use std::net::{IpAddr, SocketAddr};
use std::str::FromStr;
use std::sync::{Arc, Mutex};
use std::time::{Duration, Instant, SystemTime, UNIX_EPOCH};

const MAX_BODY_SIZE: usize = 1024 * 1024; // 1MB
const DEFAULT_EXPECTED_CHAIN_ID: u64 = 84532;
const NONCE_SWEEP_INTERVAL_SECONDS: u64 = 60;
const DEFAULT_PORT: u16 = 3002;
const DEFAULT_BIND_ADDRESS: &str = "0.0.0.0";

#[derive(Clone)]
struct AppState {
    max_body_size: usize,
    expected_chain_id: u64,
    nonce_store: Arc<NonceStore>,
    signature_expiry_seconds: u64,
    clock_skew_seconds: u64,
    minimum_authorization_version: u8,
}

struct MemoryNonceStore {
    used_nonces: Arc<DashMap<[u8; 32], Instant>>,
    last_nonce_sweep: Arc<Mutex<Instant>>,
}

#[derive(Clone)]
struct RedisNonceStore {
    client: redis::Client,
    key_prefix: String,
    timeout: Duration,
}

enum NonceStore {
    Memory(MemoryNonceStore),
    Redis(RedisNonceStore),
}

impl Clone for NonceStore {
    fn clone(&self) -> Self {
        match self {
            NonceStore::Memory(store) => NonceStore::Memory(MemoryNonceStore {
                used_nonces: store.used_nonces.clone(),
                last_nonce_sweep: store.last_nonce_sweep.clone(),
            }),
            NonceStore::Redis(store) => NonceStore::Redis(store.clone()),
        }
    }
}

#[derive(Debug)]
struct NonceStoreError {
    message: String,
}

impl std::fmt::Display for NonceStoreError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "{}", self.message)
    }
}

impl std::error::Error for NonceStoreError {}

impl From<redis::RedisError> for NonceStoreError {
    fn from(err: redis::RedisError) -> Self {
        Self {
            message: format!("redis nonce store unavailable: {}", err),
        }
    }
}

impl NonceStoreError {
    fn timeout(operation: &str) -> Self {
        Self {
            message: format!("redis nonce store timed out during {operation}"),
        }
    }
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

fn get_minimum_authorization_version() -> Result<u8, String> {
    let raw = env::var("MIN_AUTHORIZATION_VERSION").unwrap_or_else(|_| "2".to_string());
    match raw.trim().parse::<u8>() {
        Ok(version @ 1..=2) => Ok(version),
        _ => Err("MIN_AUTHORIZATION_VERSION must be 1 or 2".to_string()),
    }
}

fn get_port() -> u16 {
    match std::env::var("PORT") {
        Ok(v) => match v.parse::<u16>() {
            Ok(port) if port > 0 => port,
            Ok(_) => {
                eprintln!("Warning: PORT must be > 0, using default {}", DEFAULT_PORT);
                DEFAULT_PORT
            }
            Err(_) => {
                eprintln!(
                    "Warning: Invalid PORT '{}', using default {}",
                    v, DEFAULT_PORT
                );
                DEFAULT_PORT
            }
        },
        Err(_) => DEFAULT_PORT,
    }
}

fn get_bind_address() -> IpAddr {
    match std::env::var("BIND_ADDRESS") {
        Ok(v) => match v.parse::<IpAddr>() {
            Ok(addr) => addr,
            Err(_) => {
                eprintln!(
                    "Warning: Invalid BIND_ADDRESS '{}', using default {}",
                    v, DEFAULT_BIND_ADDRESS
                );
                DEFAULT_BIND_ADDRESS.parse().unwrap()
            }
        },
        Err(_) => DEFAULT_BIND_ADDRESS.parse().unwrap(),
    }
}

fn memory_nonce_store() -> Arc<NonceStore> {
    Arc::new(NonceStore::Memory(MemoryNonceStore {
        used_nonces: Arc::new(DashMap::new()),
        last_nonce_sweep: Arc::new(Mutex::new(Instant::now())),
    }))
}

fn normalize_redis_url(raw_url: &str) -> String {
    if raw_url.starts_with("redis://") || raw_url.starts_with("rediss://") {
        raw_url.to_string()
    } else {
        format!("redis://{raw_url}")
    }
}

fn redis_url_has_database(redis_url: &str) -> bool {
    let without_scheme = redis_url
        .split_once("://")
        .map(|(_, rest)| rest)
        .unwrap_or(redis_url);
    let path_end = without_scheme
        .find(['?', '#'])
        .unwrap_or(without_scheme.len());
    let Some(path_start) = without_scheme[..path_end].find('/') else {
        return false;
    };

    !without_scheme[path_start + 1..path_end].trim().is_empty()
}

fn get_non_empty_env(key: &str) -> Option<String> {
    env::var(key).ok().filter(|value| !value.trim().is_empty())
}

fn verifier_redis_connection_info(
    raw_url: &str,
) -> Result<redis::ConnectionInfo, redis::RedisError> {
    let redis_url = normalize_redis_url(raw_url);
    let has_database = redis_url_has_database(&redis_url);
    let mut connection_info: redis::ConnectionInfo = redis_url.as_str().parse()?;
    let mut redis_settings = connection_info.redis_settings().clone();

    if redis_settings.password().is_none() {
        if let Some(password) = get_non_empty_env("REDIS_PASSWORD") {
            redis_settings = redis_settings.set_password(password);
        }
    }
    if !has_database {
        if let Some(db) = get_non_empty_env("REDIS_DB").and_then(|value| value.parse::<i64>().ok())
        {
            redis_settings = redis_settings.set_db(db);
        }
    }
    connection_info = connection_info.set_redis_settings(redis_settings);

    Ok(connection_info)
}

fn get_redis_nonce_key_prefix() -> String {
    env::var("VERIFIER_NONCE_KEY_PREFIX")
        .ok()
        .filter(|value| !value.trim().is_empty())
        .unwrap_or_else(|| "microai:verifier:nonce:".to_string())
}

fn redis_nonce_timeout() -> Duration {
    const DEFAULT_REDIS_TIMEOUT_MS: u64 = 2_000;

    let timeout_ms = env::var("VERIFIER_REDIS_TIMEOUT_MS")
        .ok()
        .and_then(|value| value.trim().parse::<u64>().ok())
        .filter(|value| *value > 0)
        .unwrap_or(DEFAULT_REDIS_TIMEOUT_MS);

    Duration::from_millis(timeout_ms)
}

fn build_nonce_store_from_env() -> Result<Arc<NonceStore>, String> {
    let mode = env::var("VERIFIER_NONCE_STORE")
        .unwrap_or_else(|_| "memory".to_string())
        .to_ascii_lowercase();

    match mode.as_str() {
        "memory" => Ok(memory_nonce_store()),
        "redis" => {
            let redis_url = env::var("REDIS_URL")
                .map_err(|_| "VERIFIER_NONCE_STORE=redis requires REDIS_URL".to_string())?;
            let client = redis::Client::open(
                verifier_redis_connection_info(&redis_url)
                    .map_err(|err| format!("invalid REDIS_URL for verifier nonce store: {err}"))?,
            )
            .map_err(|err| format!("invalid REDIS_URL for verifier nonce store: {err}"))?;

            Ok(Arc::new(NonceStore::Redis(RedisNonceStore {
                client,
                key_prefix: get_redis_nonce_key_prefix(),
                timeout: redis_nonce_timeout(),
            })))
        }
        other => Err(format!(
            "unsupported VERIFIER_NONCE_STORE '{other}', expected 'memory' or 'redis'"
        )),
    }
}

#[tokio::main]
async fn main() {
    let limit = get_max_body_size();
    let expected_chain_id = get_expected_chain_id();
    let nonce_store =
        build_nonce_store_from_env().expect("failed to configure verifier nonce store");
    let state = AppState {
        max_body_size: limit,
        expected_chain_id,
        nonce_store,
        signature_expiry_seconds: get_env_u64("SIGNATURE_EXPIRY_SECONDS", 300),
        clock_skew_seconds: get_env_u64("SIGNATURE_CLOCK_SKEW_SECONDS", 60),
        minimum_authorization_version: get_minimum_authorization_version()
            .expect("failed to configure minimum authorization version"),
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

    let addr = SocketAddr::new(get_bind_address(), get_port());
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
    payer: Option<String>,
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
    #[serde(flatten)]
    authorization: AuthorizationBinding,
}

#[derive(Deserialize, Debug, Clone, Default)]
struct AuthorizationBinding {
    #[serde(
        rename = "authorizationVersion",
        default,
        deserialize_with = "deserialize_authorization_version"
    )]
    version: AuthorizationVersion,
    audience: Option<String>,
    method: Option<String>,
    resource: Option<String>,
    #[serde(rename = "contentType")]
    content_type: Option<String>,
    #[serde(rename = "requestHash")]
    request_hash: Option<String>,
}

#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
enum AuthorizationVersion {
    #[default]
    Legacy,
    V2,
    Unsupported(u8),
}

fn deserialize_authorization_version<'de, D>(
    deserializer: D,
) -> Result<AuthorizationVersion, D::Error>
where
    D: serde::Deserializer<'de>,
{
    let version = u8::deserialize(deserializer)?;
    Ok(match version {
        2 => AuthorizationVersion::V2,
        version => AuthorizationVersion::Unsupported(version),
    })
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

fn maybe_evict_expired_nonces(store: &MemoryNonceStore, now: Instant, ttl: Duration) {
    let mut last_sweep = store
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
    evict_expired_nonces(&store.used_nonces, now, ttl);
}

fn redis_nonce_key(prefix: &str, nonce: &str) -> String {
    format!("{}{}", prefix, hex::encode(keccak256(nonce.as_bytes())))
}

fn claim_memory_nonce(store: &MemoryNonceStore, nonce: &str, now: Instant, ttl: Duration) -> bool {
    maybe_evict_expired_nonces(store, now, ttl);

    match store.used_nonces.entry(keccak256(nonce.as_bytes())) {
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

async fn claim_redis_nonce(
    store: &RedisNonceStore,
    nonce: &str,
    ttl: Duration,
) -> Result<bool, NonceStoreError> {
    let mut conn = tokio::time::timeout(
        store.timeout,
        store.client.get_multiplexed_async_connection(),
    )
    .await
    .map_err(|_| NonceStoreError::timeout("connection acquisition"))??;
    let ttl_seconds = ttl.as_secs().max(1);
    let key = redis_nonce_key(&store.key_prefix, nonce);
    let result: Option<String> = tokio::time::timeout(
        store.timeout,
        conn.set_options(
            key,
            "1",
            redis::SetOptions::default()
                .conditional_set(redis::ExistenceCheck::NX)
                .with_expiration(redis::SetExpiry::EX(ttl_seconds)),
        ),
    )
    .await
    .map_err(|_| NonceStoreError::timeout("atomic nonce claim"))??;

    Ok(result.is_some())
}

async fn claim_nonce(state: &AppState, nonce: &str, now: Instant) -> Result<bool, NonceStoreError> {
    let ttl = nonce_retention_ttl(state);
    match state.nonce_store.as_ref() {
        NonceStore::Memory(store) => Ok(claim_memory_nonce(store, nonce, now, ttl)),
        NonceStore::Redis(store) => claim_redis_nonce(store, nonce, ttl).await,
    }
}

/* =======================
   Signature Verification
======================= */

#[derive(Debug)]
enum AuthorizationContextError {
    MissingVersion,
    UnsupportedVersion(u8),
    MissingField(&'static str),
}

impl std::fmt::Display for AuthorizationContextError {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Self::MissingVersion => write!(
                formatter,
                "authorizationVersion is required when request-binding fields are present"
            ),
            Self::UnsupportedVersion(version) => {
                write!(formatter, "unsupported authorizationVersion {version}")
            }
            Self::MissingField(field) => write!(formatter, "v2 context is missing {field}"),
        }
    }
}

fn required_binding_field<'a>(
    value: &'a Option<String>,
    name: &'static str,
) -> Result<&'a str, AuthorizationContextError> {
    value
        .as_deref()
        .filter(|value| !value.is_empty())
        .ok_or(AuthorizationContextError::MissingField(name))
}

fn build_payment_typed_data(
    context: &PaymentContext,
    payer: Option<&str>,
) -> Result<TypedData, AuthorizationContextError> {
    let typed_data_json = match context.authorization.version {
        AuthorizationVersion::Legacy => {
            if context.authorization.audience.is_some()
                || context.authorization.method.is_some()
                || context.authorization.resource.is_some()
                || context.authorization.content_type.is_some()
                || context.authorization.request_hash.is_some()
            {
                return Err(AuthorizationContextError::MissingVersion);
            }
            serde_json::json!({
                "domain": {
                    "name": "MicroAI Paygate",
                    "version": "1",
                    "chainId": context.chain_id,
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
                    "recipient": context.recipient,
                    "token": context.token,
                    "amount": context.amount,
                    "nonce": context.nonce,
                    "timestamp": context.timestamp
                }
            })
        }
        AuthorizationVersion::V2 => serde_json::json!({
            "domain": {
                "name": "MicroAI Paygate",
                "version": "2",
                "chainId": context.chain_id,
                "verifyingContract": "0x0000000000000000000000000000000000000000"
            },
            "types": {
                "PaymentAuthorization": [
                    { "name": "payer", "type": "address" },
                    { "name": "recipient", "type": "address" },
                    { "name": "token", "type": "string" },
                    { "name": "amount", "type": "string" },
                    { "name": "nonce", "type": "string" },
                    { "name": "timestamp", "type": "uint256" },
                    { "name": "audience", "type": "string" },
                    { "name": "method", "type": "string" },
                    { "name": "resource", "type": "string" },
                    { "name": "contentType", "type": "string" },
                    { "name": "requestHash", "type": "bytes32" }
                ]
            },
            "primaryType": "PaymentAuthorization",
            "message": {
                "payer": payer
                    .filter(|value| !value.is_empty())
                    .ok_or(AuthorizationContextError::MissingField("payer"))?,
                "recipient": context.recipient,
                "token": context.token,
                "amount": context.amount,
                "nonce": context.nonce,
                "timestamp": context.timestamp,
                "audience": required_binding_field(&context.authorization.audience, "audience")?,
                "method": required_binding_field(&context.authorization.method, "method")?,
                "resource": required_binding_field(&context.authorization.resource, "resource")?,
                "contentType": required_binding_field(&context.authorization.content_type, "contentType")?,
                "requestHash": required_binding_field(&context.authorization.request_hash, "requestHash")?
            }
        }),
        AuthorizationVersion::Unsupported(version) => {
            return Err(AuthorizationContextError::UnsupportedVersion(version));
        }
    };

    serde_json::from_value(typed_data_json)
        .map_err(|_| AuthorizationContextError::MissingField("valid EIP-712 fields"))
}

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
    println!("[CID: {}] Verify payment authorization", cid);

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

    let authorization_version = match payload.context.authorization.version {
        AuthorizationVersion::Legacy => 1,
        AuthorizationVersion::V2 => 2,
        AuthorizationVersion::Unsupported(version) => version,
    };
    if authorization_version < state.minimum_authorization_version {
        record_verification_failure(&request_start, "authorization_downgrade");
        return (
            StatusCode::BAD_REQUEST,
            res_headers,
            Json(VerifyResponse {
                is_valid: false,
                recovered_address: None,
                error: Some(format!(
                    "authorization version {authorization_version} is below the configured minimum"
                )),
                error_code: Some("authorization_version_too_old".to_string()),
            }),
        );
    }

    let typed_data = match build_payment_typed_data(&payload.context, payload.payer.as_deref()) {
        Ok(td) => td,
        Err(e) => {
            record_verification_failure(&request_start, "invalid_authorization_context");
            return (
                StatusCode::BAD_REQUEST,
                res_headers,
                Json(VerifyResponse {
                    is_valid: false,
                    recovered_address: None,
                    error: Some(e.to_string()),
                    error_code: Some("invalid_authorization_context".to_string()),
                }),
            );
        }
    };

    let expected_payer = if payload.context.authorization.version == AuthorizationVersion::V2 {
        let payer = match payload.payer.as_deref() {
            Some(payer) => payer,
            None => {
                record_verification_failure(&request_start, "invalid_authorization_context");
                return (
                    StatusCode::BAD_REQUEST,
                    res_headers,
                    Json(VerifyResponse {
                        is_valid: false,
                        recovered_address: None,
                        error: Some("v2 verification request is missing payer".to_string()),
                        error_code: Some("invalid_authorization_context".to_string()),
                    }),
                );
            }
        };
        match Address::from_str(payer) {
            Ok(address) => Some(address),
            Err(_) => {
                record_verification_failure(&request_start, "invalid_authorization_context");
                return (
                    StatusCode::BAD_REQUEST,
                    res_headers,
                    Json(VerifyResponse {
                        is_valid: false,
                        recovered_address: None,
                        error: Some("v2 verification request has invalid payer".to_string()),
                        error_code: Some("invalid_authorization_context".to_string()),
                    }),
                );
            }
        }
    } else {
        None
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
            if expected_payer.is_some_and(|expected| expected != addr) {
                metrics::record_verification(false, duration, Some("signer_mismatch"));
                return (
                    StatusCode::OK,
                    res_headers,
                    Json(VerifyResponse {
                        is_valid: false,
                        recovered_address: Some(format!("{:?}", addr)),
                        error: Some("signature does not match payer".to_string()),
                        error_code: Some("signer_mismatch".to_string()),
                    }),
                );
            }
            match claim_nonce(&state, &payload.context.nonce, Instant::now()).await {
                Ok(true) => {}
                Ok(false) => {
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
                Err(err) => {
                    metrics::record_verification(false, duration, Some("nonce_store_unavailable"));
                    return (
                        StatusCode::SERVICE_UNAVAILABLE,
                        res_headers,
                        Json(VerifyResponse {
                            is_valid: false,
                            recovered_address: None,
                            error: Some(err.to_string()),
                            error_code: Some("nonce_store_unavailable".to_string()),
                        }),
                    );
                }
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
    use ethers::signers::{LocalWallet, Signer};
    use ethers::types::transaction::eip712::{Eip712, TypedData};
    use std::sync::atomic::{AtomicU64, Ordering};
    use std::sync::Arc;

    const BASE_SEPOLIA_CHAIN_ID: u64 = 84532;
    static ENV_LOCK: Mutex<()> = Mutex::new(());
    static TEST_NONCE_COUNTER: AtomicU64 = AtomicU64::new(0);

    fn app_state() -> AppState {
        app_state_with_window(300, 60)
    }

    fn app_state_with_window(signature_expiry_seconds: u64, clock_skew_seconds: u64) -> AppState {
        app_state_with_nonce_store(
            memory_nonce_store(),
            signature_expiry_seconds,
            clock_skew_seconds,
        )
    }

    fn app_state_with_nonce_store(
        nonce_store: Arc<NonceStore>,
        signature_expiry_seconds: u64,
        clock_skew_seconds: u64,
    ) -> AppState {
        AppState {
            max_body_size: MAX_BODY_SIZE,
            expected_chain_id: BASE_SEPOLIA_CHAIN_ID,
            nonce_store,
            signature_expiry_seconds,
            clock_skew_seconds,
            minimum_authorization_version: 1,
        }
    }

    fn now() -> u64 {
        SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_secs()
    }

    fn unique_test_nonce() -> String {
        format!(
            "{}-{}",
            now(),
            TEST_NONCE_COUNTER.fetch_add(1, Ordering::Relaxed)
        )
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

    fn with_nonce_env(
        nonce_store: Option<&str>,
        redis_url: Option<&str>,
        key_prefix: Option<&str>,
        redis_timeout_ms: Option<&str>,
        test: impl FnOnce(),
    ) {
        let _guard = ENV_LOCK.lock().unwrap();
        let old_nonce_store = env::var("VERIFIER_NONCE_STORE").ok();
        let old_redis_url = env::var("REDIS_URL").ok();
        let old_key_prefix = env::var("VERIFIER_NONCE_KEY_PREFIX").ok();
        let old_redis_timeout_ms = env::var("VERIFIER_REDIS_TIMEOUT_MS").ok();

        match nonce_store {
            Some(value) => env::set_var("VERIFIER_NONCE_STORE", value),
            None => env::remove_var("VERIFIER_NONCE_STORE"),
        }
        match redis_url {
            Some(value) => env::set_var("REDIS_URL", value),
            None => env::remove_var("REDIS_URL"),
        }
        match key_prefix {
            Some(value) => env::set_var("VERIFIER_NONCE_KEY_PREFIX", value),
            None => env::remove_var("VERIFIER_NONCE_KEY_PREFIX"),
        }
        match redis_timeout_ms {
            Some(value) => env::set_var("VERIFIER_REDIS_TIMEOUT_MS", value),
            None => env::remove_var("VERIFIER_REDIS_TIMEOUT_MS"),
        }

        test();

        match old_nonce_store {
            Some(value) => env::set_var("VERIFIER_NONCE_STORE", value),
            None => env::remove_var("VERIFIER_NONCE_STORE"),
        }
        match old_redis_url {
            Some(value) => env::set_var("REDIS_URL", value),
            None => env::remove_var("REDIS_URL"),
        }
        match old_key_prefix {
            Some(value) => env::set_var("VERIFIER_NONCE_KEY_PREFIX", value),
            None => env::remove_var("VERIFIER_NONCE_KEY_PREFIX"),
        }
        match old_redis_timeout_ms {
            Some(value) => env::set_var("VERIFIER_REDIS_TIMEOUT_MS", value),
            None => env::remove_var("VERIFIER_REDIS_TIMEOUT_MS"),
        }
    }

    fn with_port_env(port: Option<&str>, test: impl FnOnce()) {
        let _guard = ENV_LOCK.lock().unwrap();

        let old_port = env::var("PORT").ok();

        match port {
            Some(value) => env::set_var("PORT", value),
            None => env::remove_var("PORT"),
        }

        test();

        match old_port {
            Some(value) => env::set_var("PORT", value),
            None => env::remove_var("PORT"),
        }
    }

    fn with_redis_auth_env(
        redis_password: Option<&str>,
        redis_db: Option<&str>,
        test: impl FnOnce(),
    ) {
        let _guard = ENV_LOCK.lock().unwrap();
        let old_redis_password = env::var("REDIS_PASSWORD").ok();
        let old_redis_db = env::var("REDIS_DB").ok();

        match redis_password {
            Some(value) => env::set_var("REDIS_PASSWORD", value),
            None => env::remove_var("REDIS_PASSWORD"),
        }
        match redis_db {
            Some(value) => env::set_var("REDIS_DB", value),
            None => env::remove_var("REDIS_DB"),
        }

        test();

        match old_redis_password {
            Some(value) => env::set_var("REDIS_PASSWORD", value),
            None => env::remove_var("REDIS_PASSWORD"),
        }
        match old_redis_db {
            Some(value) => env::set_var("REDIS_DB", value),
            None => env::remove_var("REDIS_DB"),
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

    #[test]
    fn test_minimum_authorization_version_defaults_to_v2_and_rejects_invalid_values() {
        let _guard = ENV_LOCK.lock().unwrap();
        let old = env::var("MIN_AUTHORIZATION_VERSION").ok();

        env::remove_var("MIN_AUTHORIZATION_VERSION");
        assert_eq!(get_minimum_authorization_version().unwrap(), 2);
        env::set_var("MIN_AUTHORIZATION_VERSION", "1");
        assert_eq!(get_minimum_authorization_version().unwrap(), 1);
        env::set_var("MIN_AUTHORIZATION_VERSION", "3");
        assert!(get_minimum_authorization_version().is_err());

        match old {
            Some(value) => env::set_var("MIN_AUTHORIZATION_VERSION", value),
            None => env::remove_var("MIN_AUTHORIZATION_VERSION"),
        }
    }

    #[test]
    fn test_normalize_redis_url_accepts_bare_host_port() {
        assert_eq!(normalize_redis_url("redis:6379"), "redis://redis:6379");
        assert_eq!(
            normalize_redis_url("redis://localhost:6379"),
            "redis://localhost:6379"
        );
        assert_eq!(
            normalize_redis_url("rediss://cache.example.com:6380"),
            "rediss://cache.example.com:6380"
        );
    }

    #[test]
    fn test_verifier_redis_connection_info_uses_env_fallbacks_for_bare_url() {
        with_redis_auth_env(Some("secret"), Some("2"), || {
            let connection_info = verifier_redis_connection_info("redis:6379").unwrap();

            assert_eq!(connection_info.redis_settings().password(), Some("secret"));
            assert_eq!(connection_info.redis_settings().db(), 2);
        });
    }

    #[test]
    fn test_verifier_redis_connection_info_preserves_explicit_url_auth_and_db() {
        with_redis_auth_env(Some("env-secret"), Some("2"), || {
            let connection_info =
                verifier_redis_connection_info("redis://user:url-secret@redis:6379/4").unwrap();

            assert_eq!(connection_info.redis_settings().username(), Some("user"));
            assert_eq!(
                connection_info.redis_settings().password(),
                Some("url-secret")
            );
            assert_eq!(connection_info.redis_settings().db(), 4);
        });
    }

    #[test]
    fn test_build_nonce_store_defaults_to_memory() {
        with_nonce_env(None, None, None, None, || {
            let store = build_nonce_store_from_env().unwrap();
            assert!(matches!(store.as_ref(), NonceStore::Memory(_)));
        });
    }

    #[test]
    fn test_build_redis_nonce_store_requires_redis_url() {
        with_nonce_env(Some("redis"), None, None, None, || {
            let err = match build_nonce_store_from_env() {
                Ok(_) => panic!("expected REDIS_URL error"),
                Err(err) => err,
            };
            assert!(err.contains("REDIS_URL"));
        });
    }

    #[test]
    fn test_redis_nonce_timeout_defaults_to_two_seconds() {
        with_nonce_env(None, None, None, None, || {
            assert_eq!(redis_nonce_timeout(), Duration::from_millis(2_000));
        });
    }

    #[test]
    fn test_redis_nonce_timeout_uses_env_milliseconds() {
        with_nonce_env(None, None, None, Some("750"), || {
            assert_eq!(redis_nonce_timeout(), Duration::from_millis(750));
        });
    }

    #[test]
    fn test_redis_nonce_timeout_rejects_invalid_env() {
        with_nonce_env(None, None, None, Some("not-a-number"), || {
            assert_eq!(redis_nonce_timeout(), Duration::from_millis(2_000));
        });
        with_nonce_env(None, None, None, Some("0"), || {
            assert_eq!(redis_nonce_timeout(), Duration::from_millis(2_000));
        });
    }

    #[test]
    fn test_redis_nonce_key_hashes_raw_nonce() {
        let key = redis_nonce_key("prefix:", "sensitive-nonce-value");
        assert!(key.starts_with("prefix:"));
        assert!(!key.contains("sensitive-nonce-value"));
        assert_eq!(key.len(), "prefix:".len() + 64);
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
                authorization: AuthorizationBinding::default(),
            },
            signature: format!("0x{}", hex::encode(sig.to_vec())),
            payer: None,
        }
    }

    #[test]
    fn test_v2_typed_data_matches_shared_request_binding_fixture() {
        let fixture: serde_json::Value = serde_json::from_str(include_str!(
            "../../tests/fixtures/payment-authorization-v2.json"
        ))
        .unwrap();
        let context: PaymentContext = serde_json::from_value(fixture["context"].clone()).unwrap();
        let typed_data =
            build_payment_typed_data(&context, Some(fixture["payer"].as_str().unwrap())).unwrap();

        assert_eq!(
            format!("0x{}", hex::encode(typed_data.encode_eip712().unwrap())),
            fixture["expectedTypedDataDigest"].as_str().unwrap()
        );

        let signature =
            Signature::from_str(fixture["expectedSignature"].as_str().unwrap()).unwrap();
        assert_eq!(
            signature.recover_typed_data(&typed_data).unwrap(),
            Address::from_str(fixture["expectedSigner"].as_str().unwrap()).unwrap()
        );
    }

    #[test]
    fn test_v2_context_rejects_missing_binding_fields_and_unknown_versions() {
        let fixture: serde_json::Value = serde_json::from_str(include_str!(
            "../../tests/fixtures/payment-authorization-v2.json"
        ))
        .unwrap();
        let mut missing_hash = fixture["context"].clone();
        missing_hash.as_object_mut().unwrap().remove("requestHash");
        let context: PaymentContext = serde_json::from_value(missing_hash).unwrap();
        assert!(build_payment_typed_data(
            &context,
            Some("0x14791697260E4c9A71f18484C9f997B308e59325")
        )
        .is_err());

        let mut unknown_version = fixture["context"].clone();
        unknown_version["authorizationVersion"] = serde_json::json!(3);
        let context: PaymentContext = serde_json::from_value(unknown_version).unwrap();
        assert!(build_payment_typed_data(
            &context,
            Some("0x14791697260E4c9A71f18484C9f997B308e59325")
        )
        .is_err());
    }

    async fn signed_v2_request(nonce: &str) -> VerifyRequest {
        let fixture: serde_json::Value = serde_json::from_str(include_str!(
            "../../tests/fixtures/payment-authorization-v2.json"
        ))
        .unwrap();
        let mut context: PaymentContext =
            serde_json::from_value(fixture["context"].clone()).unwrap();
        context.nonce = nonce.to_string();
        context.timestamp = Some(now());

        let wallet: LocalWallet =
            "0123456789012345678901234567890123456789012345678901234567890123"
                .parse()
                .unwrap();
        let payer = format!("{:?}", wallet.address());
        let signature = wallet
            .sign_typed_data(&build_payment_typed_data(&context, Some(&payer)).unwrap())
            .await
            .unwrap();

        VerifyRequest {
            context,
            signature: format!("0x{}", hex::encode(signature.to_vec())),
            payer: Some(payer),
        }
    }

    #[tokio::test]
    async fn test_verify_signature_accepts_v2_request_binding() {
        let request = signed_v2_request(&unique_test_nonce()).await;
        let (status, _, Json(response)) =
            verify_signature(State(app_state()), HeaderMap::new(), Ok(Json(request))).await;

        assert_eq!(status, StatusCode::OK);
        assert!(response.is_valid);
        assert_eq!(
            response.recovered_address.as_deref(),
            Some("0x14791697260e4c9a71f18484c9f997b308e59325")
        );
    }

    #[tokio::test]
    async fn test_verify_signature_rejects_legacy_authorization_when_v2_is_required() {
        let request = signed_request(&unique_test_nonce(), BASE_SEPOLIA_CHAIN_ID, now()).await;
        let mut state = app_state();
        state.minimum_authorization_version = 2;

        let (status, _, Json(response)) =
            verify_signature(State(state), HeaderMap::new(), Ok(Json(request))).await;

        assert_eq!(status, StatusCode::BAD_REQUEST);
        assert!(!response.is_valid);
        assert_eq!(
            response.error_code.as_deref(),
            Some("authorization_version_too_old")
        );
    }

    #[tokio::test]
    async fn test_verify_signature_rejects_tampered_v2_binding_for_claimed_payer() {
        let request = signed_v2_request(&unique_test_nonce()).await;
        let mut tampered_requests = Vec::new();

        let mut tampered = request.clone();
        tampered.context.authorization.audience = Some("https://attacker.example".to_string());
        tampered_requests.push(("audience", tampered));

        let mut tampered = request.clone();
        tampered.context.authorization.method = Some("GET".to_string());
        tampered_requests.push(("method", tampered));

        let mut tampered = request.clone();
        tampered.context.authorization.resource = Some("/api/ai/other".to_string());
        tampered_requests.push(("resource", tampered));

        let mut tampered = request.clone();
        tampered.context.authorization.content_type = Some("text/plain".to_string());
        tampered_requests.push(("content type", tampered));

        let mut tampered = request.clone();
        tampered.context.authorization.request_hash = Some(format!("0x{}", "00".repeat(32)));
        tampered_requests.push(("request hash", tampered));

        let mut tampered = request;
        tampered.payer = Some("0x0000000000000000000000000000000000000001".to_string());
        tampered_requests.push(("payer", tampered));

        for (field, tampered) in tampered_requests {
            let (status, _, Json(response)) =
                verify_signature(State(app_state()), HeaderMap::new(), Ok(Json(tampered))).await;

            assert_eq!(status, StatusCode::OK, "{field}");
            assert!(!response.is_valid, "{field}");
            assert_eq!(
                response.error_code.as_deref(),
                Some("signer_mismatch"),
                "{field}"
            );
        }
    }

    #[tokio::test]
    async fn test_verify_signature_rejects_v2_request_without_payer() {
        let mut request = signed_v2_request(&unique_test_nonce()).await;
        request.payer = None;

        let (status, _, Json(response)) =
            verify_signature(State(app_state()), HeaderMap::new(), Ok(Json(request))).await;

        assert_eq!(status, StatusCode::BAD_REQUEST);
        assert!(!response.is_valid);
        assert_eq!(
            response.error_code.as_deref(),
            Some("invalid_authorization_context")
        );
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

    #[test]
    fn test_get_port_defaults_when_unset() {
        with_port_env(None, || {
            assert_eq!(get_port(), DEFAULT_PORT);
        });
    }

    #[test]
    fn test_get_port_reads_valid_port() {
        with_port_env(Some("4000"), || {
            assert_eq!(get_port(), 4000);
        });
    }

    #[test]
    fn test_get_port_falls_back_on_invalid_value() {
        with_port_env(Some("abc"), || {
            assert_eq!(get_port(), DEFAULT_PORT);
        });
    }

    #[test]
    fn test_get_bind_address_defaults_when_unset() {
        let _guard = ENV_LOCK.lock().unwrap();

        let old = env::var("BIND_ADDRESS").ok();
        env::remove_var("BIND_ADDRESS");

        assert_eq!(get_bind_address().to_string(), "0.0.0.0");

        match old {
            Some(v) => env::set_var("BIND_ADDRESS", v),
            None => env::remove_var("BIND_ADDRESS"),
        }
    }

    #[test]
    fn test_get_bind_address_reads_valid_address() {
        let _guard = ENV_LOCK.lock().unwrap();

        let old = env::var("BIND_ADDRESS").ok();
        env::set_var("BIND_ADDRESS", "127.0.0.1");

        assert_eq!(get_bind_address().to_string(), "127.0.0.1");

        match old {
            Some(v) => env::set_var("BIND_ADDRESS", v),
            None => env::remove_var("BIND_ADDRESS"),
        }
    }

    #[test]
    fn test_get_bind_address_falls_back_on_invalid_value() {
        let _guard = ENV_LOCK.lock().unwrap();

        let old = env::var("BIND_ADDRESS").ok();
        env::set_var("BIND_ADDRESS", "not-an-ip");

        assert_eq!(get_bind_address().to_string(), "0.0.0.0");

        match old {
            Some(v) => env::set_var("BIND_ADDRESS", v),
            None => env::remove_var("BIND_ADDRESS"),
        }
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
                authorization: AuthorizationBinding::default(),
            },
            signature: format!("0x{}", hex::encode(sig.to_vec())),
            payer: None,
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
                authorization: AuthorizationBinding::default(),
            },
            signature: format!("0x{}", hex::encode(sig.to_vec())),
            payer: None,
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
                    authorization: AuthorizationBinding::default(),
                },
                signature: "0x1234567890".to_string(),
                payer: None,
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

    #[tokio::test]
    async fn test_claim_nonce_retains_entries_through_clock_skew_window() {
        let state = app_state_with_window(1, 2);
        let start = Instant::now();

        assert!(claim_nonce(&state, "ttl-replay-nonce", start)
            .await
            .unwrap());
        assert!(!claim_nonce(
            &state,
            "ttl-replay-nonce",
            start + Duration::from_millis(1100)
        )
        .await
        .unwrap());
        assert!(!claim_nonce(
            &state,
            "ttl-replay-nonce",
            start + Duration::from_millis(3100)
        )
        .await
        .unwrap());
        assert!(!claim_nonce(
            &state,
            "ttl-replay-nonce",
            start + Duration::from_millis(4000)
        )
        .await
        .unwrap());
        assert!(claim_nonce(
            &state,
            "ttl-replay-nonce",
            start + Duration::from_millis(4100)
        )
        .await
        .unwrap());
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
    async fn test_verify_signature_fails_closed_when_redis_nonce_store_unavailable() {
        let client = redis::Client::open("redis://127.0.0.1:1").unwrap();
        let state = app_state_with_nonce_store(
            Arc::new(NonceStore::Redis(RedisNonceStore {
                client,
                key_prefix: "test:verifier:nonce:".to_string(),
                timeout: redis_nonce_timeout(),
            })),
            300,
            60,
        );
        let req = signed_request("redis-unavailable-nonce", BASE_SEPOLIA_CHAIN_ID, now()).await;

        let result = tokio::time::timeout(
            Duration::from_secs(2),
            verify_signature(State(state), HeaderMap::new(), Ok(Json(req))),
        )
        .await
        .expect("redis-unavailable path should fail closed quickly");
        let (status, _, Json(resp)) = result;

        assert_eq!(status, StatusCode::SERVICE_UNAVAILABLE);
        assert!(!resp.is_valid);
        assert_eq!(resp.error_code.as_deref(), Some("nonce_store_unavailable"));
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
                authorization: AuthorizationBinding::default(),
            },
            signature: "0x1234567890".to_string(),
            payer: None,
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
                authorization: AuthorizationBinding::default(),
            },
            signature: "0x1234567890".to_string(),
            payer: None,
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
                authorization: AuthorizationBinding::default(),
            },
            signature: "0x1234567890".to_string(),
            payer: None,
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
                authorization: AuthorizationBinding::default(),
            },
            signature: signature_str,
            payer: None,
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
                authorization: AuthorizationBinding::default(),
            },
            signature: "0x1234567890".to_string(),
            payer: None,
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
            nonce_store: memory_nonce_store(),
            signature_expiry_seconds: 300,
            clock_skew_seconds: 60,
            minimum_authorization_version: 1,
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
                authorization: AuthorizationBinding::default(),
            },
            signature: "0x1234567890".to_string(),
            payer: None,
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
                authorization: AuthorizationBinding::default(),
            },
            signature: "not-a-signature".to_string(),
            payer: None,
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
