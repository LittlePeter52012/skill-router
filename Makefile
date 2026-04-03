.PHONY: build test bench install clean release

# Build variables
BINARY := skrt
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "0.2.0-dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
BUILD_TIME := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS := -s -w \
	-X main.version=$(VERSION) \
	-X main.gitCommit=$(COMMIT) \
	-X main.buildTime=$(BUILD_TIME)

## build: Build the binary
build:
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/skrt

## test: Run all tests
test:
	go test -v -race ./...

## bench: Run benchmarks
bench:
	go test -bench=. -benchmem ./...

## install: Install to $GOPATH/bin
install:
	go install -ldflags "$(LDFLAGS)" ./cmd/skrt

## clean: Remove build artifacts
clean:
	rm -rf bin/ dist/
	rm -f coverage.out

## cover: Run tests with coverage
cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

## lint: Run go vet
lint:
	go vet ./...

## release: Build for all platforms
release:
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o dist/$(BINARY)-darwin-arm64 ./cmd/skrt
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o dist/$(BINARY)-darwin-amd64 ./cmd/skrt
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o dist/$(BINARY)-linux-amd64 ./cmd/skrt
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o dist/$(BINARY)-linux-arm64 ./cmd/skrt
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o dist/$(BINARY)-windows-amd64.exe ./cmd/skrt

## help: Show this help
help:
	@grep -E '^## ' Makefile | sed 's/## //' | column -t -s ':'
