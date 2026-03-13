#!/bin/bash
# Build TallyBridge Connector for Windows ARM64
# Uses zig as C cross-compiler for CGO (go-sqlite3)
set -e

export PATH="$PATH:/usr/local/go/bin:$HOME/go/bin:/opt/homebrew/bin"

PROJ_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$PROJ_DIR"

echo "=== Building frontend ==="
cd frontend && npm run build && cd ..

echo "=== Cross-compiling for Windows ARM64 ==="
export GOOS=windows
export GOARCH=arm64
export CGO_ENABLED=1
export CC="zig cc -target aarch64-windows-gnu"
export CXX="zig c++ -target aarch64-windows-gnu"

# Build the Go binary (without Wails — Wails doesn't support cross-compilation directly)
# We use go build with the embedded frontend
go build -o build/bin/tallybridge-connector-arm64.exe -ldflags="-s -w" .

echo ""
echo "=== Build complete ==="
ls -lh build/bin/tallybridge-connector-arm64.exe
echo ""
echo "Output: build/bin/tallybridge-connector-arm64.exe"
