#!/bin/bash

# Get the directory where the script is located
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
E2E_TMP_DIR="$(mktemp -d)"

# Function to cleanup background processes on exit
cleanup() {
    local exit_code=$?
    echo "Stopping services..."
    # Use a portable check for jobs since xargs -r is not available on all macOS versions
    if [ -n "$(jobs -p)" ]; then
        jobs -p | xargs kill 2>/dev/null
    fi
    rm -rf "$E2E_TMP_DIR"
    exit "$exit_code"
}

trap cleanup EXIT

echo "Building Verifier..."
cd "$SCRIPT_DIR/verifier" && cargo build --quiet
if [ $? -ne 0 ]; then
    echo "Verifier build failed"
    exit 1
fi
echo "Starting Verifier..."
PORT=3002 MIN_AUTHORIZATION_VERSION=2 cargo run --quiet &
VERIFIER_PID=$!

echo "Starting Gateway..."
cd "$SCRIPT_DIR/gateway"
export RECEIPT_STORE="${RECEIPT_STORE:-memory}"
export CACHE_ENABLED="${CACHE_ENABLED:-false}"
# The gateway now requires VERIFIER_URL at startup; point it at the verifier
# we just spawned on localhost above. Honors any caller-supplied override.
export VERIFIER_URL="${VERIFIER_URL:-http://127.0.0.1:3002}"
export PAYGATE_AUDIENCE="${PAYGATE_AUDIENCE:-http://localhost:3000}"
export SERVER_WALLET_PRIVATE_KEY="${SERVER_WALLET_PRIVATE_KEY:-0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef}"
export RECIPIENT_ADDRESS="${RECIPIENT_ADDRESS:-0x2cAF48b4BA1C58721a85dFADa5aC01C2DFa62219}"
if [ -z "$OPENROUTER_API_KEY" ]; then
    echo "Starting deterministic OpenRouter mock..."
    export AI_PROVIDER="openrouter"
    export OPENROUTER_API_KEY="e2e-test-key"
    export OPENROUTER_URL="http://127.0.0.1:3100/api/v1/chat/completions"
    (cd "$SCRIPT_DIR" && bun tests/mock-openrouter.ts) &
fi
echo "Building Gateway..."
go build -o "$E2E_TMP_DIR/gateway" . || { echo "Gateway build failed"; exit 1; }
"$E2E_TMP_DIR/gateway" &
GATEWAY_PID=$!

# Wait for both services to be ready instead of assuming a fixed build/start time.
echo "Waiting for services to initialize..."
SERVICES_READY=false
for _ in $(seq 1 60); do
    if curl --fail --silent "http://127.0.0.1:3002/health" >/dev/null 2>&1 && \
       curl --fail --silent "http://localhost:3000/healthz" >/dev/null 2>&1; then
        SERVICES_READY=true
        break
    fi
    if ! kill -0 "$VERIFIER_PID" 2>/dev/null || ! kill -0 "$GATEWAY_PID" 2>/dev/null; then
        echo "A service exited before becoming ready"
        exit 1
    fi
    sleep 1
done

if [ "$SERVICES_READY" != true ]; then
    echo "Services did not become ready within 60 seconds"
    exit 1
fi

echo "Running E2E Tests..."
cd "$SCRIPT_DIR" || { echo "Error: Failed to change directory to $SCRIPT_DIR"; exit 1; }
bun test tests/e2e.test.ts
