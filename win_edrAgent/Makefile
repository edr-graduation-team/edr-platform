# EDR Windows Agent - build and code generation

.PHONY: proto build test clean signatures-fetch

# Generate Go and gRPC code from .proto into internal/pb/
# Requires: protoc, protoc-gen-go, protoc-gen-go-grpc (see scripts/genproto.sh)
proto:
	@./scripts/genproto.sh

# Refresh embedded MalwareBazaar recent hashes (abuse.ch; see their FAQ/TOS).
signatures-fetch:
	go run ./tools/gen-malwarebazaar-seed -limit 500 -out internal/signatures/builtin_hashes.ndjson

build:
	go build -o bin/agent.exe ./cmd/agent

test:
	go test ./...

clean:
	rm -rf bin/
