#!/bin/sh
# =============================================================================
# Docker Entrypoint for Connection Manager
# =============================================================================
# This script runs BEFORE the main application starts. It ensures that all
# required crypto material (CA, server cert, JWT keys) exists. The Go binary
# handles the actual generation via EnsureFullPKI(), but we need to make sure
# the certs directory is writable and has correct permissions.
# =============================================================================

set -e

CERTS_DIR="/app/certs"

echo "=== Connection Manager Entrypoint ==="

# Ensure certs directory exists and is writable
mkdir -p "${CERTS_DIR}"

# Fix permissions on existing cert files (bind mount may have restrictive perms)
# This is critical for the PKI bootstrap: EnsureFullPKI() reads/writes these files.
if [ -d "${CERTS_DIR}" ]; then
    chmod 755 "${CERTS_DIR}" 2>/dev/null || true
    # Ensure existing cert files are readable/writable by the process
    for f in "${CERTS_DIR}"/*.crt "${CERTS_DIR}"/*.key "${CERTS_DIR}"/*.pem; do
        [ -f "$f" ] && chmod 644 "$f" 2>/dev/null || true
    done
fi

# Diagnostic: log existing cert state for troubleshooting
echo "Certs directory ready: ${CERTS_DIR}"
if [ -f "${CERTS_DIR}/ca.crt" ]; then
    echo "  CA cert:     EXISTS ($(wc -c < "${CERTS_DIR}/ca.crt") bytes)"
else
    echo "  CA cert:     MISSING (will be generated)"
fi
if [ -f "${CERTS_DIR}/server.crt" ]; then
    echo "  Server cert: EXISTS ($(wc -c < "${CERTS_DIR}/server.crt") bytes)"
else
    echo "  Server cert: MISSING (will be generated)"
fi

echo "Starting connection-manager..."

# Execute the main application (replaces this shell process)
exec /app/connection-manager "$@"

