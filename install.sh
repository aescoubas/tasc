#!/bin/bash

# tasc Installation Script

set -e

# Detect OS
OS="$(uname -s)"
ARCH="$(uname -m)"

echo "Detected OS: $OS"
echo "Detected Arch: $ARCH"

# Check for Go
if ! command -v go &> /dev/null; then
    echo "Error: Go is not installed. Please install Go 1.24+ to build from source."
    exit 1
fi

echo "Building tasc..."

# Clean previous build
rm -f tasc

# Build command with fts5 support
go build -tags fts5 -ldflags "-s -w" -o tasc main.go

if [ ! -f "tasc" ]; then
    echo "Error: Build failed."
    exit 1
fi

echo "Build successful."

INSTALL_DIR="/usr/local/bin"

if [ -w "$INSTALL_DIR" ]; then
    echo "Installing to $INSTALL_DIR..."
    mv tasc "$INSTALL_DIR/"
else
    echo "Installing to $INSTALL_DIR (sudo required)..."
    sudo mv tasc "$INSTALL_DIR/"
fi

echo "--------------------------------------------------"
echo "Success! tasc has been installed to $INSTALL_DIR/tasc"
echo "Run 'tasc --help' to get started."
echo "--------------------------------------------------"
