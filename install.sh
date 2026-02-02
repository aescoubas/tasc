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

echo "Installing shell autocompletion..."

# Bash
BASH_COMP_DIR="$HOME/.local/share/bash-completion/completions"
if [ ! -d "$BASH_COMP_DIR" ]; then
    mkdir -p "$BASH_COMP_DIR"
fi
"$INSTALL_DIR/tasc" completion bash > "$BASH_COMP_DIR/tasc"
echo "Bash completion installed to $BASH_COMP_DIR/tasc"

# Zsh
ZSH_COMP_DIR="$HOME/.zsh/completion"
if [ ! -d "$ZSH_COMP_DIR" ]; then
    mkdir -p "$ZSH_COMP_DIR"
fi
"$INSTALL_DIR/tasc" completion zsh > "$ZSH_COMP_DIR/_tasc"
echo "Zsh completion installed to $ZSH_COMP_DIR/_tasc"
echo "Note: For Zsh, ensure '$ZSH_COMP_DIR' is in your \$fpath in .zshrc:"
echo "      fpath=($ZSH_COMP_DIR \$fpath)"
echo "      autoload -U compinit; compinit"
echo "--------------------------------------------------"

# Systemd Integration
if command -v systemctl &> /dev/null; then
    echo
    read -p "Do you want to install and enable the 'tasc' background service (user-level)? [y/N] " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        # Ensure user systemd dir exists
        SYSTEMD_DIR="$HOME/.config/systemd/user"
        mkdir -p "$SYSTEMD_DIR"
        
        echo "Installing service file to $SYSTEMD_DIR/tasc.service..."
        # Check if source file exists
        if [ -f "dist/systemd/tasc.service" ]; then
            cp dist/systemd/tasc.service "$SYSTEMD_DIR/"
            
            echo "Reloading systemd..."
            systemctl --user daemon-reload
            
            echo "Enabling and starting tasc.service..."
            systemctl --user enable --now tasc
            
            echo "Service active. Check status with: systemctl --user status tasc"
        else
            echo "Error: Service file 'dist/systemd/tasc.service' not found. Skipping service installation."
        fi
    fi
fi

