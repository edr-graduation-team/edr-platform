# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Install dependencies
RUN apk add --no-cache git

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build
RUN CGO_ENABLED=0 GOOS=linux go build -o /sigma-engine ./cmd/sigma-engine-kafka/main.go

# Runtime stage
FROM alpine:3.19

RUN apk add --no-cache ca-certificates curl

WORKDIR /app

# Copy binary
COPY --from=builder /sigma-engine .

# Copy config
COPY --from=builder /app/config ./config

# Copy rules
COPY --from=builder /app/sigma_rules ./rules

# Health check
HEALTHCHECK --interval=30s --timeout=10s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

# Run
EXPOSE 8080
CMD ["./sigma-engine"]
