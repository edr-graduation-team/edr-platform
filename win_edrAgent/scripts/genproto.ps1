# Generate Go and gRPC code from internal/proto/v1/edr.proto into internal/pb/.
# Requires: protoc, protoc-gen-go, protoc-gen-go-grpc in PATH (e.g. $env:GOPATH\bin or $env:GOBIN).
# Install: go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
#          go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
$ErrorActionPreference = "Stop"
$Module = "github.com/edr-platform/win-agent"
$RootDir = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
$ProtoDir = Join-Path $RootDir "internal\proto"
$OutDir = Join-Path $RootDir "internal\pb"

if (-not (Get-Command protoc -ErrorAction SilentlyContinue)) {
    Write-Error "protoc not found. Install Protocol Buffers compiler."
    exit 1
}

$GoPath = go env GOPATH
if ($env:GOBIN) { $BinPath = $env:GOBIN } else { $BinPath = Join-Path $GoPath "bin" }
$env:PATH = "$BinPath;$env:PATH"

New-Item -ItemType Directory -Force -Path $OutDir | Out-Null

# Include path for google/protobuf/duration.proto and empty.proto (optional; protoc may have built-in)
$IncludeDir = Join-Path $RootDir "internal\.protoc\include"
# Use repo root as go_out so that module strip produces internal\pb\ (not internal\pb\internal\pb\)
& protoc -I $ProtoDir -I $IncludeDir `
    --go_out=$RootDir --go_opt=module=$Module `
    --go-grpc_out=$RootDir --go-grpc_opt=module=$Module `
    (Join-Path $ProtoDir "v1\edr.proto")

Write-Host "Generated code in $OutDir"
