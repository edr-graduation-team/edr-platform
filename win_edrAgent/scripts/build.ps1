# EDR Windows Agent Build Script
# ==============================

param(
    [string]$Version = "1.0.0",
    [string]$Output = "bin\agent.exe",
    [switch]$Release,
    [switch]$Test,
    [switch]$Clean
)

$ErrorActionPreference = "Stop"

# Set build variables
$BuildTime = Get-Date -Format "yyyy-MM-ddTHH:mm:ssZ"
$GitCommit = git rev-parse --short HEAD 2>$null
if (-not $GitCommit) { $GitCommit = "unknown" }

$LDFlags = "-X main.Version=$Version -X main.BuildTime=$BuildTime -X main.GitCommit=$GitCommit"

if ($Release) {
    $LDFlags += " -s -w"  # Strip debug info for smaller binary
}

Write-Host "========================================" -ForegroundColor Cyan
Write-Host "EDR Windows Agent Build" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "Version:    $Version"
Write-Host "Build Time: $BuildTime"
Write-Host "Git Commit: $GitCommit"
Write-Host "Output:     $Output"
Write-Host "Release:    $Release"
Write-Host ""

# Clean
if ($Clean) {
    Write-Host "Cleaning..." -ForegroundColor Yellow
    Remove-Item -Path "bin" -Recurse -Force -ErrorAction SilentlyContinue
    go clean -cache
    Write-Host "Clean complete" -ForegroundColor Green
    exit 0
}

# Run tests
if ($Test) {
    Write-Host "Running tests..." -ForegroundColor Yellow
    go test ./... -v -cover
    if ($LASTEXITCODE -ne 0) {
        Write-Host "Tests failed!" -ForegroundColor Red
        exit 1
    }
    Write-Host "Tests passed" -ForegroundColor Green
}

# Create output directory
$OutputDir = Split-Path -Parent $Output
if (-not (Test-Path $OutputDir)) {
    New-Item -ItemType Directory -Path $OutputDir -Force | Out-Null
}

# Download dependencies
Write-Host "Downloading dependencies..." -ForegroundColor Yellow
go mod download
go mod tidy

# Build
Write-Host "Building agent..." -ForegroundColor Yellow

$env:CGO_ENABLED = "0"
$env:GOOS = "windows"
$env:GOARCH = "amd64"

go build -ldflags "$LDFlags" -o $Output ./cmd/agent

if ($LASTEXITCODE -ne 0) {
    Write-Host "Build failed!" -ForegroundColor Red
    exit 1
}

# Get file info
$FileInfo = Get-Item $Output
$FileSizeMB = [math]::Round($FileInfo.Length / 1MB, 2)

Write-Host ""
Write-Host "========================================" -ForegroundColor Green
Write-Host "Build successful!" -ForegroundColor Green
Write-Host "========================================" -ForegroundColor Green
Write-Host "Output: $Output"
Write-Host "Size:   $FileSizeMB MB"
Write-Host ""

# Generate checksum
$Hash = Get-FileHash -Path $Output -Algorithm SHA256
$Hash.Hash | Out-File -FilePath "$Output.sha256" -Encoding ASCII
Write-Host "Checksum: $($Hash.Hash)"
Write-Host "Saved to: $Output.sha256"
Write-Host ""

# Usage instructions
Write-Host "Usage:" -ForegroundColor Cyan
Write-Host "  Install:   .\$Output -install"
Write-Host "  Uninstall: .\$Output -uninstall"
Write-Host "  Run:       .\$Output -debug"
Write-Host "  Version:   .\$Output -version"
