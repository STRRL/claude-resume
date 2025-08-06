.PHONY: build build-all build-linux build-linux-amd64 build-linux-arm64 build-darwin install clean test run

# Default build for current platform
build:
	go build -o claude-resume ./cmd/claude-resume

# Build for all supported platforms
build-all: build-linux build-darwin

# Linux builds (amd64/x86_64 and arm64)
build-linux: build-linux-amd64 build-linux-arm64

build-linux-amd64:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=1 go build -ldflags="-s -w" -o dist/claude-resume-linux-amd64 ./cmd/claude-resume

build-linux-arm64:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=1 CC=aarch64-linux-gnu-gcc CXX=aarch64-linux-gnu-g++ go build -ldflags="-s -w" -o dist/claude-resume-linux-arm64 ./cmd/claude-resume

# macOS builds
build-darwin:
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=1 go build -ldflags="-s -w" -o dist/claude-resume-darwin-amd64 ./cmd/claude-resume
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=1 go build -ldflags="-s -w" -o dist/claude-resume-darwin-arm64 ./cmd/claude-resume

install:
	go install ./cmd/claude-resume

clean:
	rm -f claude-resume
	rm -rf dist/

test:
	go test ./...

run: build
	./claude-resume

# Release using GoReleaser
release:
	goreleaser release --clean

# Test release build locally
release-snapshot:
	goreleaser release --snapshot --clean