# Build Stage
FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o gateway main.go

# Runtime Stage
FROM alpine:latest
RUN apk add --no-cache ca-certificates && adduser -D -u 1000 appuser
WORKDIR /home/appuser

# Copy binary
COPY --from=builder /app/gateway /home/appuser/gateway

# 🔴 THIS IS THE IMPORTANT LINE
COPY --from=builder /app/openapi.yaml /home/appuser/openapi.yaml

RUN chown -R appuser:appuser /home/appuser
USER appuser
EXPOSE 3000
CMD ["./gateway"]
