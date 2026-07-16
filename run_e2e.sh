#!/bin/bash

# Get the directory where the script is located
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

# Function to cleanup background processes on exit
cleanup() {
    echo "Stopping services..."
    # Use a portable check for jobs since xargs -r is not available on all macOS versions
    if [ -n "$(jobs -p)" ]; then
        jobs -p | xargs kill 2>/dev/null
    fi
    exit
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
    export OPENROUTER_API_KEY="e2e-test-key"
    export OPENROUTER_URL="http://127.0.0.1:3100/api/v1/chat/completions"
    (cd "$SCRIPT_DIR" && bun tests/mock-openrouter.ts) &
fi
go run . &
GATEWAY_PID=$!

# Wait for services to be ready
echo "Waiting for services to initialize (10s)..."
sleep 10

echo "Running E2E Tests..."
cd "$SCRIPT_DIR" || { echo "Error: Failed to change directory to $SCRIPT_DIR"; exit 1; }
bun test tests/e2e.test.ts
