BINARY   := kinlyze
VERSION  := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT   := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE     := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS  := -s -w \
	-X github.com/talhakhalidmtk/kinlyze-library/cmd.version=$(VERSION) \
	-X github.com/talhakhalidmtk/kinlyze-library/cmd.commit=$(COMMIT) \
	-X github.com/talhakhalidmtk/kinlyze-library/cmd.date=$(DATE)

# Directories
DIST     := dist
COVERAGE := coverage.out

.PHONY: all build clean test install release lint snapshot help

## all: Build for current platform
all: build

## build: Compile binary for current OS/arch
build:
	@echo "→ Building $(BINARY) $(VERSION)..."
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) .
	@echo "✓ Built ./$(BINARY)"

## install: Install to /usr/local/bin
install: build
	@echo "→ Installing to /usr/local/bin/$(BINARY)..."
	cp $(BINARY) /usr/local/bin/$(BINARY)
	@echo "✓ Installed. Run: kinlyze --help"

## test: Run all tests
test:
	go test ./... -v -count=1

## test-cover: Run tests with coverage report
test-cover:
	go test ./... -coverprofile=$(COVERAGE) -covermode=atomic
	go tool cover -html=$(COVERAGE)

## lint: Run golangci-lint
lint:
	golangci-lint run ./...

## snapshot: Build all platform binaries locally (requires goreleaser)
snapshot:
	goreleaser release --snapshot --clean

## release: Tag and trigger GitHub Actions release
release:
	@if [ -z "$(TAG)" ]; then echo "Usage: make release TAG=v0.1.0"; exit 1; fi
	git tag $(TAG)
	git push origin $(TAG)
	@echo "✓ Tagged $(TAG) — GitHub Actions will build and publish the release."

## clean: Remove build artifacts
clean:
	rm -f $(BINARY) $(COVERAGE)
	rm -rf $(DIST)

## cross: Build for all platforms manually (without goreleaser)
cross:
	@echo "→ Building for all platforms..."
	@mkdir -p $(DIST)
	GOOS=darwin  GOARCH=amd64  go build -ldflags "$(LDFLAGS)" -o $(DIST)/$(BINARY)-darwin-amd64  .
	GOOS=darwin  GOARCH=arm64  go build -ldflags "$(LDFLAGS)" -o $(DIST)/$(BINARY)-darwin-arm64  .
	GOOS=linux   GOARCH=amd64  go build -ldflags "$(LDFLAGS)" -o $(DIST)/$(BINARY)-linux-amd64   .
	GOOS=linux   GOARCH=arm64  go build -ldflags "$(LDFLAGS)" -o $(DIST)/$(BINARY)-linux-arm64   .
	GOOS=windows GOARCH=amd64  go build -ldflags "$(LDFLAGS)" -o $(DIST)/$(BINARY)-windows-amd64.exe .
	@echo "✓ Built all platforms in ./$(DIST)/"

## help: Show this help
help:
	@grep -E '^## ' Makefile | sed 's/## //' | column -t -s ':'
