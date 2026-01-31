#!/bin/bash

# build-packages.sh - Build binaries and packages (deb, rpm) locally using GoReleaser

set -e

# Function to check command existence
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# 1. Check for GoReleaser
if ! command_exists goreleaser; then
    echo "Error: goreleaser is not installed."
    echo "Please install it: https://goreleaser.com/install/"
    echo "For example (Go): go install github.com/goreleaser/goreleaser/v2@latest"
    exit 1
fi

# 2. Check for Cross-Compiler (needed for ARM64 linux build defined in .goreleaser.yaml)
# The config uses aarch64-linux-gnu-gcc for linux/arm64
if ! command_exists aarch64-linux-gnu-gcc; then
    echo "Warning: 'aarch64-linux-gnu-gcc' not found."
    echo "The build configuration requires this for Linux ARM64 support (due to SQLite CGO)."
    echo "On Debian/Ubuntu, install it with: sudo apt-get install gcc-aarch64-linux-gnu"
    
    # We check if we are actually building for linux/arm64. If so, this will likely fail.
    # But we'll let GoReleaser run and error out properly if needed.
    echo "Proceeding... (build may fail)"
fi

echo "Starting local snapshot build..."
echo "This will create binaries, archives, and packages in the 'dist/' directory."

# Run GoReleaser
# --snapshot: Build unreleased version (e.g. v0.0.0-SNAPSHOT...)
# --clean: Remove dist/ before starting
# --skip=publish: Don't upload anything
goreleaser release --snapshot --clean --skip=publish

echo ""
echo "--------------------------------------------------"
echo "Build complete!"
echo "Artifacts are in the 'dist/' directory."
echo ""
echo "Generated Packages:"
find dist -maxdepth 2 -name "*.deb" -o -name "*.rpm"
echo "--------------------------------------------------"
