#!/bin/bash

# release.sh - Create a GitHub release locally using 'gh' and 'goreleaser'

set -e

# --- Helper Functions ---
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

usage() {
    echo "Usage: $0 <version>"
    echo "Example: $0 v1.0.0"
    exit 1
}

# --- Prerequisites Checks ---

# 1. Check version argument
if [ -z "$1" ]; then
    usage
fi
VERSION=$1

# Ensure version starts with 'v'
if [[ ! $VERSION =~ ^v ]]; then
    echo "Error: Version must start with 'v' (e.g., v1.0.0)"
    exit 1
fi

# 2. Check for required tools
if ! command_exists gh; then
    echo "Error: GitHub CLI (gh) is not installed."
    echo "Install it: https://cli.github.com/"
    exit 1
fi

if ! command_exists goreleaser; then
    echo "Error: GoReleaser is not installed."
    echo "Install it: https://goreleaser.com/install/"
    exit 1
fi

# 3. Check GitHub Auth Status
if ! gh auth status >/dev/null 2>&1; then
    echo "Error: You are not logged into GitHub CLI."
    echo "Run 'gh auth login' first."
    exit 1
fi

# 4. Check Git Status
CURRENT_BRANCH=$(git rev-parse --abbrev-ref HEAD)
if [ "$CURRENT_BRANCH" != "main" ]; then
    echo "Error: You must be on the 'main' branch to release."
    exit 1
fi

if ! git diff-index --quiet HEAD --; then
    echo "Error: You have uncommitted changes. Commit or stash them first."
    exit 1
fi

# 5. Check Cross-Compiler (for Linux ARM64)
if ! command_exists aarch64-linux-gnu-gcc; then
    echo "Warning: 'aarch64-linux-gnu-gcc' not found."
    echo "Linux ARM64 build might fail."
    echo "Install with: sudo apt-get install gcc-aarch64-linux-gnu"
    read -p "Continue anyway? [y/N] " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        exit 1
    fi
fi

# --- Execution ---

echo "--- Starting Release Process for $VERSION ---"

# 1. Pull latest
echo "[1/5] Pulling latest changes..."
git pull origin main

# 2. Create Tag Locally
if git rev-parse "$VERSION" >/dev/null 2>&1; then
    echo "Error: Tag '$VERSION' already exists locally."
    exit 1
fi
echo "[2/5] Creating local tag $VERSION..."
git tag -a "$VERSION" -m "Release $VERSION"

# 3. Build Artifacts with GoReleaser
# We skip 'publish' because we will do it manually with 'gh'.
# We skip 'validate' to avoid complaints about missing GITHUB_TOKEN env var if it's not set (since we use gh).

echo "[3/5] Building artifacts..."
goreleaser release --clean --skip=publish --skip=validate

# 4. Create Release on GitHub and Upload Assets
echo "[4/5] Creating GitHub Release and uploading assets..."

# Collect files to upload from dist/
# We exclude the folders and just get files.
ASSETS=$(find dist -maxdepth 1 -type f \( -name "*.tar.gz" -o -name "*.zip" -o -name "*.deb" -o -name "*.rpm" -o -name "checksums.txt" \))

# Create release
# --generate-notes: Auto-generate changelog from PRs
# --title: Release title
# --target: Target branch/commit (main)
# The tag is pushed automatically by 'gh release create' if it doesn't exist on remote.
gh release create "$VERSION" \
    --title "Release $VERSION" \
    --generate-notes \
    $ASSETS

echo "----------------------------------------------------------------"
echo "[5/5] Success! Release $VERSION created."
echo "View it here: $(gh repo view --json url -q .url)/releases/tag/$VERSION"
echo "----------------------------------------------------------------"