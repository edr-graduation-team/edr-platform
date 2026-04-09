# Build stage
FROM golang:1.24-alpine AS builder

# Install dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# 🚀 السطر السحري لتسريع التحميل وتجاوز اختناق الشبكة
ENV GOPROXY="https://goproxy.io,direct"

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s -X main.version=${VERSION:-dev}" \
    -o /app/bin/connection-manager \
    ./cmd/server

# Final stage — lightweight runtime (build handled by agent-builder service)
FROM alpine:3.19

# Install CA certificates and timezone data
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN adduser -D -u 1000 edr
USER edr

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/bin/connection-manager /app/connection-manager

# Copy config template
COPY --from=builder /app/config/config.example.yaml /app/config/config.yaml

# Expose ports
# gRPC port
EXPOSE 50051
# HTTP API/health port (override with HTTP_PORT in compose)
EXPOSE 8082

# Health check (uses HTTP_PORT at runtime; default 8082 for root docker-compose)
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8082/healthz || exit 1

# Set environment variables
ENV LOG_LEVEL=info
ENV GRPC_PORT=50051
ENV HTTP_PORT=8082

# Run the application
ENTRYPOINT ["/app/connection-manager"]
CMD ["-config", "/app/config/config.yaml"]
