.PHONY: all build test lint clean dev help

help:
	@echo "MicroAI-Paygate Makefile targets:"
	@echo ""
	@echo "  make all                - Default target (alias for build)"
	@echo "  make build              - Build all services"
	@echo "  make test               - Run tests for all services"
	@echo "  make lint               - Run all linters"
	@echo "  make dev                - Start full stack (gateway:3000, web:3001, verifier:3002)"
	@echo "  make clean              - Remove build artifacts and generated files"
	@echo "  make help               - Show this help message"

# Show help for unknown targets
.DEFAULT:
	@echo "Unknown target: $@"
	@$(MAKE) help

# Global
all: build

# Sub-projects
build-gateway:
	cd gateway && go build -o bin/gateway

build-verifier:
	cd verifier && cargo build --release

build-web:
	cd web && bun run build

# Composite
build: build-gateway build-verifier build-web

test:
	cd gateway && go test -v ./...
	cd verifier && cargo test
	cd web && bun run test

lint:
	cd gateway && go fmt ./... && go vet ./...
	cd verifier && cargo fmt -- --check && cargo clippy -- -D warnings
	cd web && bun run lint

dev:
	bun run stack

clean:
	cd gateway && rm -rf bin/
	cd verifier && cargo clean
	cd web && rm -rf .next out
