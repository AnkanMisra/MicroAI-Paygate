# Verifier Micro-Benchmark

This benchmark measures the Rust verifier `/verify` endpoint only. It does not measure gateway, wallet UI, OpenRouter, Redis, or end-to-end x402 latency.

## Requirements

- `wrk`
- `bun`
- repo root dependencies installed with `bun install`
- verifier running on `http://127.0.0.1:3002` unless `VERIFIER_URL` is set

The script does not install `wrk` or any system package. Install tooling separately, then record the install method with your results.

## Run

```bash
cd /path/to/MicroAI-Paygate
cd verifier && cargo run
```

In another shell:

```bash
RESULTS_FILE=bench/RESULTS-2026-05-13.txt bench/bench.sh
```

Useful overrides:

```bash
THREADS=4 CONNECTIONS=64 DURATION=60s EXPECTED_MAX_RPS=3000 bench/bench.sh
PAYLOAD_COUNT=200000 bench/bench.sh
VERIFIER_URL=http://127.0.0.1:3002 bench/bench.sh
SIGNATURE_EXPIRY_SECONDS=300 bench/bench.sh
BENCH_REVEAL_HOST=true bench/bench.sh
```

## Method

`bench.sh` pre-generates one-time EIP-712 payment payloads with Bun and `ethers`, writes them to a temporary JSONL file, then gives `wrk` a Lua script that consumes each signed nonce at most once. This avoids benchmarking replay rejection instead of signature verification. By default, `PAYLOAD_COUNT` is derived from `DURATION * EXPECTED_MAX_RPS`; increase either value if the validity check reports exhausted payloads.

The script refuses runs where `DURATION` plus payload age would approach `SIGNATURE_EXPIRY_SECONDS`, and the Lua script aggregates per-thread counts for non-200 or `is_valid:false` responses. Set `SIGNATURE_EXPIRY_SECONDS` to the same value used by the verifier when benchmarking a non-default verifier configuration.

The default private key is a deterministic local test key used only for benchmark signing. Do not use a funded wallet.

## Reporting

Only cite numbers that appear in a committed `bench/RESULTS-*.txt` file. Keep raw `wrk` output, non-identifying hardware metadata, thread/connection settings, duration, payload count, and validity-check counters together in the results file. Hostname and full `uname -a` output are omitted by default; set `BENCH_REVEAL_HOST=true` only for private local diagnostics.
