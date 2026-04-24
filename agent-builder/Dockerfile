# =============================================================================
# Agent Builder — Cross-compilation service for EDR Agent (with CGO/MinGW)
# =============================================================================
#
# This container provides:
#   - Go 1.24 toolchain
#   - MinGW-w64 cross-compiler (for CGO: ETW uses C headers + tdh/advapi32)
#   - A lightweight HTTP server that accepts build requests from the CM
#
# The agent source code is mounted at runtime via docker-compose volume.
#
# PERFORMANCE: On container startup, `go mod download` pre-fetches all agent
# dependencies before the builder HTTP server starts. Combined with the
# `agent_build_cache` Docker volume, this ensures fast builds (~30-60s)
# instead of the 3+ minute cold-start that caused timeout failures.
# =============================================================================

FROM golang:1.24-alpine

# Some environments start containers with an empty PATH. Ensure the Go toolchain
# and standard Unix utilities are always resolvable.
ENV PATH="/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

# Install build dependencies:
#   - git: for Go module downloads
#   - ca-certificates, tzdata: standard
#   - mingw-w64-gcc: Windows cross-compiler for CGO (ETW collector uses C)
RUN apk add --no-cache \
    git \
    ca-certificates \
    tzdata \
    mingw-w64-gcc

WORKDIR /app

# Copy and build the builder service itself
COPY go.mod ./
COPY main.go ./
RUN go build -o /app/builder .

# Agent source is mounted at /app/agent-src at runtime
WORKDIR /app/agent-src

EXPOSE 8090

ENV BUILDER_PORT=8090
ENV AGENT_SRC_DIR=/app/agent-src
# MinGW cross-compiler for Windows CGO
ENV CC_FOR_windows_amd64=x86_64-w64-mingw32-gcc
ENV CC=x86_64-w64-mingw32-gcc

HEALTHCHECK --interval=15s --timeout=5s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8090/healthz || exit 1

# ── Startup: pre-warm module cache, then start builder ──────────────────────
# The entrypoint runs `go mod download` against the bind-mounted agent source
# BEFORE starting the HTTP server. This ensures:
#   1. All Go modules are cached on disk (in the agent_build_cache volume)
#   2. The first build request only compiles — no network downloads needed
#   3. Builds complete in ~30-60s instead of ~3 min (no more timeout failures)
CMD sh -c '\
    echo "[INIT] Pre-warming Go module cache from /app/agent-src..." && \
    cd /app/agent-src && \
    GOOS=windows GOARCH=amd64 go mod download 2>&1 && \
    echo "[INIT] Module cache ready. Starting builder..." && \
    exec /app/builder'
