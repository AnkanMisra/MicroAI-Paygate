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

export RECEIPT_STORE="${RECEIPT_STORE:-memory}"
export CACHE_ENABLED="${CACHE_ENABLED:-false}"
export AI_PROVIDER="${AI_PROVIDER:-mock}"
# Deterministic fixture values for local/CI E2E only. Real deployments must
# provide their own receipt signing key and recipient.
export SERVER_WALLET_PRIVATE_KEY="${SERVER_WALLET_PRIVATE_KEY:-0123456789012345678901234567890123456789012345678901234567890123}"
export RECIPIENT_ADDRESS="${RECIPIENT_ADDRESS:-0x2cAF48b4BA1C58721a85dFADa5aC01C2DFa62219}"
export CHAIN_ID="${CHAIN_ID:-84532}"
export EXPECTED_CHAIN_ID="${EXPECTED_CHAIN_ID:-84532}"

wait_for_service() {
    local name="$1"
    local url="$2"
    local pid="$3"
    local attempts="${4:-30}"
    local attempt=1

    while [ "$attempt" -le "$attempts" ]; do
        if curl -fsS "$url" >/dev/null 2>&1; then
            echo "$name is ready"
            return 0
        fi

        if ! kill -0 "$pid" 2>/dev/null; then
            echo "$name exited before becoming ready"
            return 1
        fi

        sleep 1
        attempt=$((attempt + 1))
    done

    echo "$name did not become ready at $url"
    return 1
}

echo "Building Verifier..."
cd "$SCRIPT_DIR/verifier" && cargo build --quiet
if [ $? -ne 0 ]; then
    echo "Verifier build failed"
    exit 1
fi

echo "Building Gateway..."
cd "$SCRIPT_DIR/gateway" && go build -o gateway .
if [ $? -ne 0 ]; then
    echo "Gateway build failed"
    exit 1
fi

echo "Starting Verifier..."
"$SCRIPT_DIR/verifier/target/debug/verifier" &
VERIFIER_PID=$!

echo "Starting Gateway..."
cd "$SCRIPT_DIR/gateway"
# The gateway now requires VERIFIER_URL at startup; point it at the verifier
# we just spawned on localhost above. Honors any caller-supplied override.
export VERIFIER_URL="${VERIFIER_URL:-http://127.0.0.1:3002}"
./gateway &
GATEWAY_PID=$!

echo "Waiting for services to initialize..."
wait_for_service "Verifier" "http://127.0.0.1:3002/health" "$VERIFIER_PID" || exit 1
wait_for_service "Gateway" "http://127.0.0.1:3000/healthz" "$GATEWAY_PID" || exit 1

echo "Running E2E Tests..."
cd "$SCRIPT_DIR" || { echo "Error: Failed to change directory to $SCRIPT_DIR"; exit 1; }
bun test tests/e2e.test.ts
