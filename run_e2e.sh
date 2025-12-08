#!/bin/bash

# Get the directory where the script is located
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

# Function to cleanup background processes on exit
cleanup() {
    echo "Stopping services..."
    kill $(jobs -p) 2>/dev/null
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
cargo run --quiet &
VERIFIER_PID=$!

echo "Starting Gateway..."
cd "$SCRIPT_DIR/gateway"
go run main.go &
GATEWAY_PID=$!

# Wait for services to be ready
echo "Waiting for services to initialize (10s)..."
sleep 10

echo "Running E2E Tests..."
cd "$SCRIPT_DIR"
bun test tests/e2e.test.ts
