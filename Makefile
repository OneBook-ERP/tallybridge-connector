.PHONY: build dev test lint lint-fix clean

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

clean:
	rm -rf build/bin
