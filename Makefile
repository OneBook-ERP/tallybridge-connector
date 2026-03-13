.PHONY: build dev test lint lint-fix clean build-windows build-windows-arm64 build-windows-x64 build-windows-x86

build:
	wails build

dev:
	wails dev

test:
	CGO_ENABLED=1 go test ./...

lint:
	golangci-lint run

lint-fix:
	golangci-lint run --fix

# Cross-compile for Windows (all architectures)
build-windows:
	./build-windows.sh all

build-windows-arm64:
	./build-windows.sh arm64

build-windows-x64:
	./build-windows.sh amd64

build-windows-x86:
	./build-windows.sh 386

clean:
	rm -rf build/bin
