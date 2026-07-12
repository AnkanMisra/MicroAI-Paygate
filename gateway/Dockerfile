# Build Stage
FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o gateway .

# Runtime Stage
FROM alpine:latest
RUN apk add --no-cache ca-certificates wget && adduser -D -u 1000 appuser
WORKDIR /home/appuser

# Copy binary
COPY --from=builder /app/gateway /home/appuser/gateway

# Copy OpenAPI spec for /docs and /openapi.yaml endpoints
COPY --from=builder /app/openapi.yaml /home/appuser/openapi.yaml

RUN chown -R appuser:appuser /home/appuser
USER appuser
EXPOSE 3000
CMD ["./gateway"]
