#!/bin/sh
# =============================================================================
# Docker Entrypoint for Connection Manager
# =============================================================================
# This script runs BEFORE the main application starts. It ensures that all
# required crypto material (CA, server cert, JWT keys) exists. The Go binary
# handles the actual generation via EnsureFullPKI(), but we need to make sure
# the certs directory is writable and exists.
# =============================================================================

set -e

CERTS_DIR="/app/certs"

echo "=== Connection Manager Entrypoint ==="

# Ensure certs directory exists and is writable
mkdir -p "${CERTS_DIR}"

echo "Certs directory ready: ${CERTS_DIR}"
echo "Starting connection-manager..."

# Execute the main application (replaces this shell process)
exec /app/connection-manager "$@"
