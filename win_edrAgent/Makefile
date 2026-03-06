# EDR Windows Agent - build and code generation

.PHONY: proto build test clean

# Generate Go and gRPC code from .proto into internal/pb/
# Requires: protoc, protoc-gen-go, protoc-gen-go-grpc (see scripts/genproto.sh)
proto:
	@./scripts/genproto.sh

build:
	go build -o bin/agent.exe ./cmd/agent

test:
	go test ./...

clean:
	rm -rf bin/
