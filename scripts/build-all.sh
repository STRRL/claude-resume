#!/bin/bash
set -e

echo "Building claude-resume for multiple platforms..."

# Create dist directory
mkdir -p dist

# Build for macOS (requires local build)
echo "Building for macOS..."
GOOS=darwin GOARCH=amd64 CGO_ENABLED=1 go build -ldflags="-s -w" -o dist/claude-resume-darwin-amd64 ./cmd/claude-resume
GOOS=darwin GOARCH=arm64 CGO_ENABLED=1 go build -ldflags="-s -w" -o dist/claude-resume-darwin-arm64 ./cmd/claude-resume

# Build for Linux using Docker (for CGO dependencies)
echo "Building for Linux using Docker..."
docker buildx build --platform linux/amd64 \
    -f Dockerfile.build \
    --target export \
    --output type=local,dest=. \
    .

echo "Build complete! Binaries are in the dist/ directory:"
ls -lh dist/