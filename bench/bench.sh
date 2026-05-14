#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERIFIER_URL="${VERIFIER_URL:-http://127.0.0.1:3002}"
TARGET_URL="${VERIFIER_URL%/}/verify"
THREADS="${THREADS:-2}"
CONNECTIONS="${CONNECTIONS:-32}"
DURATION="${DURATION:-30s}"
PAYLOAD_COUNT="${PAYLOAD_COUNT:-1000}"
SIGNATURE_EXPIRY_SECONDS="${SIGNATURE_EXPIRY_SECONDS:-300}"
EXPIRY_SAFETY_SECONDS="${EXPIRY_SAFETY_SECONDS:-15}"
CHAIN_ID="${CHAIN_ID:-84532}"
RECIPIENT_ADDRESS="${RECIPIENT_ADDRESS:-0x1234567890123456789012345678901234567890}"
PAYMENT_TOKEN="${PAYMENT_TOKEN:-USDC}"
PAYMENT_AMOUNT="${PAYMENT_AMOUNT:-0.001}"
BENCH_WALLET_PRIVATE_KEY="${BENCH_WALLET_PRIVATE_KEY:-0x380eb0f3d505f087e438eca80bc4df9a7faa24f868e69fc0440261a0fc0567dc}"
RESULTS_FILE="${RESULTS_FILE:-}"
# Hostname and full uname are omitted by default from committed results.
# Set BENCH_REVEAL_HOST=true for private local diagnostics.

require_positive_int() {
  local name="$1"
  local value="$2"
  if ! [[ "$value" =~ ^[0-9]+$ ]] || (( value <= 0 )); then
    echo "$name must be a positive integer" >&2
    return 1
  fi
}

require_non_negative_int() {
  local name="$1"
  local value="$2"
  if ! [[ "$value" =~ ^[0-9]+$ ]]; then
    echo "$name must be a non-negative integer" >&2
    return 1
  fi
}

duration_to_seconds() {
  local value="$1"
  local number
  local multiplier

  case "$value" in
    *ms)
      echo "DURATION must use whole seconds, minutes, or hours, not milliseconds" >&2
      return 1
      ;;
    *s)
      number="${value%s}"
      multiplier=1
      ;;
    *m)
      number="${value%m}"
      multiplier=60
      ;;
    *h)
      number="${value%h}"
      multiplier=3600
      ;;
    *)
      number="$value"
      multiplier=1
      ;;
  esac

  require_positive_int "DURATION" "$number"
  echo $((number * multiplier))
}

assert_validity_check() {
  local output_file="$1"
  if grep -Eq '^validity_check_(invalid|non_200)_responses=[1-9][0-9]*$' "$output_file"; then
    echo "Benchmark validity check failed: verifier returned invalid or non-200 responses" >&2
    return 1
  fi
}

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "Missing required command: $1" >&2
    return 1
  fi
}

DURATION_SECONDS="$(duration_to_seconds "$DURATION")"
require_positive_int "THREADS" "$THREADS"
require_positive_int "CONNECTIONS" "$CONNECTIONS"
require_positive_int "PAYLOAD_COUNT" "$PAYLOAD_COUNT"
require_positive_int "SIGNATURE_EXPIRY_SECONDS" "$SIGNATURE_EXPIRY_SECONDS"
require_non_negative_int "EXPIRY_SAFETY_SECONDS" "$EXPIRY_SAFETY_SECONDS"
if (( DURATION_SECONDS + EXPIRY_SAFETY_SECONDS >= SIGNATURE_EXPIRY_SECONDS )); then
  echo "DURATION plus EXPIRY_SAFETY_SECONDS must be less than SIGNATURE_EXPIRY_SECONDS" >&2
  echo "Set SIGNATURE_EXPIRY_SECONDS to match the verifier, reduce DURATION, or lower EXPIRY_SAFETY_SECONDS." >&2
  exit 1
fi

require_cmd wrk
require_cmd bun

if ! (cd "$ROOT_DIR" && bun -e 'import { Wallet } from "ethers"; void Wallet;' >/dev/null 2>&1); then
  echo "Unable to import ethers with Bun. Run 'bun install' from the repo root, then retry." >&2
  exit 1
fi

TMP_DIR="$(mktemp -d)"
PAYLOADS_FILE="$TMP_DIR/payloads.jsonl"
LUA_FILE="$TMP_DIR/rotate-payloads.lua"
GENERATOR_FILE="$ROOT_DIR/bench/.generate-payloads.tmp.mjs"
trap 'rm -rf "$TMP_DIR"; rm -f "$GENERATOR_FILE"' EXIT
BENCH_TIMESTAMP="$(date +%s)"

cat >"$GENERATOR_FILE" <<'JS'
import { randomUUID } from "node:crypto";
import { writeFileSync } from "node:fs";
import { Wallet } from "ethers";

const count = Number.parseInt(process.env.PAYLOAD_COUNT ?? "1000", 10);
if (!Number.isInteger(count) || count <= 0) {
  throw new Error("PAYLOAD_COUNT must be a positive integer");
}

const chainId = Number.parseInt(process.env.CHAIN_ID ?? "84532", 10);
const wallet = new Wallet(process.env.BENCH_WALLET_PRIVATE_KEY);
const domain = {
  name: "MicroAI Paygate",
  version: "1",
  chainId,
  verifyingContract: "0x0000000000000000000000000000000000000000",
};
const types = {
  Payment: [
    { name: "recipient", type: "address" },
    { name: "token", type: "string" },
    { name: "amount", type: "string" },
    { name: "nonce", type: "string" },
    { name: "timestamp", type: "uint256" },
  ],
};
const timestamp = Number.parseInt(process.env.BENCH_TIMESTAMP ?? `${Math.floor(Date.now() / 1000)}`, 10);
if (!Number.isInteger(timestamp) || timestamp <= 0) {
  throw new Error("BENCH_TIMESTAMP must be a positive Unix timestamp");
}
const lines = [];

for (let i = 0; i < count; i += 1) {
  const message = {
    recipient: process.env.RECIPIENT_ADDRESS,
    token: process.env.PAYMENT_TOKEN,
    amount: process.env.PAYMENT_AMOUNT,
    nonce: `bench-${randomUUID()}-${i}`,
    timestamp,
  };
  const signature = await wallet.signTypedData(domain, types, message);
  lines.push(JSON.stringify({
    context: {
      recipient: message.recipient,
      token: message.token,
      amount: message.amount,
      nonce: message.nonce,
      chainId,
      timestamp,
    },
    signature,
  }));
}

writeFileSync(process.env.PAYLOADS_FILE, `${lines.join("\n")}\n`);
JS

(
  cd "$ROOT_DIR"
  PAYLOADS_FILE="$PAYLOADS_FILE" \
  PAYLOAD_COUNT="$PAYLOAD_COUNT" \
  CHAIN_ID="$CHAIN_ID" \
  RECIPIENT_ADDRESS="$RECIPIENT_ADDRESS" \
  PAYMENT_TOKEN="$PAYMENT_TOKEN" \
  PAYMENT_AMOUNT="$PAYMENT_AMOUNT" \
  BENCH_WALLET_PRIVATE_KEY="$BENCH_WALLET_PRIVATE_KEY" \
  BENCH_TIMESTAMP="$BENCH_TIMESTAMP" \
  bun "$GENERATOR_FILE"
)

PAYLOAD_AGE_BEFORE_RUN_SECONDS=$(($(date +%s) - BENCH_TIMESTAMP))
if (( PAYLOAD_AGE_BEFORE_RUN_SECONDS + DURATION_SECONDS + EXPIRY_SAFETY_SECONDS >= SIGNATURE_EXPIRY_SECONDS )); then
  echo "Generated payloads are too old for the configured verifier expiry window." >&2
  echo "payload_age=${PAYLOAD_AGE_BEFORE_RUN_SECONDS}s duration=${DURATION_SECONDS}s safety=${EXPIRY_SAFETY_SECONDS}s expiry=${SIGNATURE_EXPIRY_SECONDS}s" >&2
  exit 1
fi

cat >"$LUA_FILE" <<LUA
local payloads = {}
local threads = {}
local thread_total = $THREADS
local next_thread_id = 0

for line in io.lines("$PAYLOADS_FILE") do
  if line ~= "" then
    table.insert(payloads, line)
  end
end

setup = function(thread)
  next_thread_id = next_thread_id + 1
  thread:set("thread_id", next_thread_id)
  thread:set("thread_total", thread_total)
  table.insert(threads, thread)
end

counter = 0
invalid_responses = 0
non_200_responses = 0

request = function()
  counter = counter + 1
  local offset = ((counter - 1) * thread_total) + (thread_id - 1)
  local body = payloads[(offset % #payloads) + 1]
  return wrk.format("POST", "/verify", {["Content-Type"] = "application/json"}, body)
end

response = function(status, headers, body)
  if status ~= 200 then
    non_200_responses = non_200_responses + 1
  end
  if not string.find(body or "", '"is_valid"%s*:%s*true') then
    invalid_responses = invalid_responses + 1
  end
end

done = function(summary, latency, requests)
  local total_invalid = 0
  local total_non_200 = 0
  for _, thread in ipairs(threads) do
    total_invalid = total_invalid + (thread:get("invalid_responses") or 0)
    total_non_200 = total_non_200 + (thread:get("non_200_responses") or 0)
  end
  io.write(string.format("validity_check_invalid_responses=%d\\n", total_invalid))
  io.write(string.format("validity_check_non_200_responses=%d\\n", total_non_200))
end
LUA

run_benchmark() {
  echo "# MicroAI Paygate verifier wrk benchmark"
  echo "date_utc=$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
  echo "os=$(uname -s)"
  echo "arch=$(uname -m)"
  if [[ "${BENCH_REVEAL_HOST:-false}" == "true" ]]; then
    echo "host=$(hostname)"
    echo "uname=$(uname -a)"
  fi
  if command -v sysctl >/dev/null 2>&1; then
    sysctl -n machdep.cpu.brand_string hw.ncpu hw.memsize 2>/dev/null | awk 'NR==1{print "cpu="$0} NR==2{print "hw_ncpu="$0} NR==3{print "hw_memsize_bytes="$0}' || true
  fi
  echo "wrk=$(wrk --version 2>&1 | head -n 1)"
  echo "target=$TARGET_URL"
  echo "threads=$THREADS"
  echo "connections=$CONNECTIONS"
  echo "duration=$DURATION"
  echo "duration_seconds=$DURATION_SECONDS"
  echo "payload_count=$PAYLOAD_COUNT"
  echo "signature_expiry_seconds=$SIGNATURE_EXPIRY_SECONDS"
  echo "expiry_safety_seconds=$EXPIRY_SAFETY_SECONDS"
  echo "payload_timestamp=$BENCH_TIMESTAMP"
  echo "payload_age_before_run_seconds=$PAYLOAD_AGE_BEFORE_RUN_SECONDS"
  echo
  wrk --latency -t "$THREADS" -c "$CONNECTIONS" -d "$DURATION" -s "$LUA_FILE" "$TARGET_URL"
}

BENCH_OUTPUT="$TMP_DIR/bench-output.txt"
if [[ -n "$RESULTS_FILE" ]]; then
  mkdir -p "$(dirname "$RESULTS_FILE")"
  run_benchmark | tee "$RESULTS_FILE" | tee "$BENCH_OUTPUT"
else
  run_benchmark | tee "$BENCH_OUTPUT"
fi
assert_validity_check "$BENCH_OUTPUT"
