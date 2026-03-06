#!/usr/bin/env bash
# Generate Go and gRPC code from internal/proto/v1/edr.proto into internal/pb/.
# Requires: protoc, protoc-gen-go, protoc-gen-go-grpc.
# Install: go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
#          go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
PROTO_DIR="$ROOT_DIR/internal/proto"
OUT_DIR="$ROOT_DIR/internal/pb"
MODULE="github.com/edr-platform/win-agent"

# Optional: point to protoc include for google/protobuf/*.proto (e.g. timestamp.proto).
# If unset, protoc uses its default include path.
PROTO_INCLUDE="${PROTO_INCLUDE:-}"

cd "$ROOT_DIR"

if ! command -v protoc &>/dev/null; then
  echo "error: protoc not found. Install Protocol Buffers compiler." >&2
  exit 1
fi

INC=(-I "$PROTO_DIR")
if [[ -n "${PROTO_INCLUDE}" ]]; then
  INC+=(-I "$PROTO_INCLUDE")
fi

mkdir -p "$OUT_DIR"
# Use repo root as go_out so that module strip produces internal/pb/ (not internal/pb/internal/pb/)
protoc "${INC[@]}" \
  --go_out="$ROOT_DIR" --go_opt=module="$MODULE" \
  --go-grpc_out="$ROOT_DIR" --go-grpc_opt=module="$MODULE" \
  "$PROTO_DIR/v1/edr.proto"

echo "Generated code in $OUT_DIR"
