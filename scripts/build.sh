#!/usr/bin/env bash

# Build script for GCI with version information
# Usage: ./scripts/build.sh [version]

set -euo pipefail

# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

# Default version if not provided
VERSION="${1:-dev}"

# Get git information if available
COMMIT="unknown"
DATE="unknown"

if command -v git >/dev/null 2>&1 && [ -d "$PROJECT_ROOT/.git" ]; then
    COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
    DATE=$(git log -1 --format=%cd --date=iso-strict 2>/dev/null || date -u +"%Y-%m-%dT%H:%M:%SZ")
else
    DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
fi

echo "Building GCI..."
echo "  Version: $VERSION"
echo "  Commit:  $COMMIT"
echo "  Date:    $DATE"
echo

# Build with ldflags
cd "$PROJECT_ROOT"

go build \
    -ldflags "-X gci/internal/version.Version=$VERSION -X gci/internal/version.Commit=$COMMIT -X gci/internal/version.Date=$DATE" \
    -o gci \
    .

echo "✅ Build complete: ./gci"
echo "Run './gci version' to see version information"