#!/bin/bash
#
# TUI Style Linter Script
# Wrapper script for the Go-based TUI style linter
#
# Usage:
#   ./scripts/lint-tui.sh              # Lint all internal/ui/*.go files
#   ./scripts/lint-tui.sh file1.go     # Lint specific files
#
# Exit codes:
#   0 - No violations found (or only warnings)
#   1 - Errors found
#

set -e

# Get the directory where this script is located
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Check if the linter binary exists, if not build it
LINTER_BIN="$PROJECT_ROOT/bin/lint-tui"

build_linter() {
    echo -e "${YELLOW}Building TUI style linter...${NC}"
    cd "$PROJECT_ROOT"
    go build -o "$LINTER_BIN" ./cmd/lint-tui
    echo -e "${GREEN}Linter built successfully${NC}"
}

# Check if we need to build
if [ ! -f "$LINTER_BIN" ]; then
    build_linter
else
    # Check if source is newer than binary
    LINTER_SRC="$PROJECT_ROOT/cmd/lint-tui/main.go"
    if [ "$LINTER_SRC" -nt "$LINTER_BIN" ]; then
        echo -e "${YELLOW}Linter source changed, rebuilding...${NC}"
        build_linter
    fi
fi

# Change to project root for consistent paths
cd "$PROJECT_ROOT"

# Run the linter
if [ $# -eq 0 ]; then
    # No arguments - lint default paths
    "$LINTER_BIN" "$@"
else
    # Pass arguments through
    "$LINTER_BIN" "$@"
fi

exit $?