#!/bin/bash
# Build TallyBridge Connector for Windows (all architectures)
# Uses zig as C cross-compiler for CGO (go-sqlite3)
#
# Usage:
#   ./build-windows.sh              # build all (arm64, amd64, 386)
#   ./build-windows.sh arm64        # build only ARM64
#   ./build-windows.sh amd64        # build only x64
#   ./build-windows.sh 386          # build only x86 (32-bit)
set -e

export PATH="$PATH:/usr/local/go/bin:$HOME/go/bin:/opt/homebrew/bin"

PROJ_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$PROJ_DIR"

VERSION="${VERSION:-1.0.0}"
LDFLAGS="-s -w -H windowsgui -X main.Version=${VERSION}"

# Build frontend once (shared across all architectures)
echo "=== Building frontend ==="
cd frontend && npm run build && cd ..

build_target() {
    local arch="$1"
    local zig_target="$2"
    local output_name="$3"

    echo ""
    echo "=== Building for Windows ${arch} ==="

    export GOOS=windows
    export GOARCH="$arch"
    export CGO_ENABLED=1
    export CC="zig cc -target ${zig_target}"
    export CXX="zig c++ -target ${zig_target}"

    go build -o "build/bin/${output_name}" -ldflags="${LDFLAGS}" .

    ls -lh "build/bin/${output_name}"
    echo "  Output: build/bin/${output_name}"
}

# Determine which architectures to build
TARGETS="${1:-all}"

case "$TARGETS" in
    arm64)
        build_target "arm64" "aarch64-windows-gnu" "tallybridge-connector-arm64.exe"
        ;;
    amd64|x64)
        build_target "amd64" "x86_64-windows-gnu" "tallybridge-connector-x64.exe"
        ;;
    386|x86)
        build_target "386" "i386-windows-gnu" "tallybridge-connector-x86.exe"
        ;;
    all)
        build_target "arm64" "aarch64-windows-gnu" "tallybridge-connector-arm64.exe"
        build_target "amd64" "x86_64-windows-gnu" "tallybridge-connector-x64.exe"
        build_target "386" "i386-windows-gnu" "tallybridge-connector-x86.exe"
        ;;
    *)
        echo "Unknown target: $TARGETS"
        echo "Usage: $0 [arm64|amd64|x64|386|x86|all]"
        exit 1
        ;;
esac

echo ""
echo "=== All builds complete ==="
ls -lh build/bin/tallybridge-connector-*.exe 2>/dev/null || true
