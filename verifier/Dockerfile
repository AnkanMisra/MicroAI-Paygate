# Build Stage
FROM rust:1.90-alpine AS builder
WORKDIR /app
RUN apk add --no-cache musl-dev
COPY . .
RUN cargo build --release

# Runtime Stage
FROM alpine:latest
RUN apk add --no-cache ca-certificates wget && adduser -D -u 1000 appuser
WORKDIR /home/appuser
COPY --from=builder /app/target/release/verifier /home/appuser/verifier
RUN chown -R appuser:appuser /home/appuser
USER appuser
EXPOSE 3002
CMD ["./verifier"]
