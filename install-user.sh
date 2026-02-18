#!/bin/bash

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;92m'
YELLOW='\033[0;93m'
NC='\033[0m' # No Color

# Configuration
BINARY_NAME="gci"
INSTALL_DIR="$HOME/.local/bin"

echo -e "${GREEN}Installing GCI to user directory ($INSTALL_DIR)...${NC}"

# Check if Go is installed
if ! command -v go &> /dev/null; then
    echo -e "${RED}Error: Go is not installed. Please install Go first.${NC}"
    echo "Visit: https://golang.org/doc/install"
    exit 1
fi

# Get the directory where this script is located
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"

# Change to the script directory
cd "$SCRIPT_DIR"

echo -e "${YELLOW}Building binary...${NC}"

# Fetch tags from remote so version reflects the latest release
git fetch --tags --quiet 2>/dev/null || true

# Inject version from git tag if available, otherwise "dev"
GIT_VERSION=$(git describe --tags --abbrev=0 2>/dev/null || echo "dev")
GIT_VERSION="${GIT_VERSION#v}"  # strip leading v
GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
GIT_DATE=$(git log -1 --format=%cd --date=iso-strict 2>/dev/null || date -u +"%Y-%m-%dT%H:%M:%SZ")

LDFLAGS="-X gci/internal/version.Version=${GIT_VERSION} -X gci/internal/version.Commit=${GIT_COMMIT} -X gci/internal/version.Date=${GIT_DATE}"
go build -ldflags "$LDFLAGS" -o "$BINARY_NAME" .

if [ $? -ne 0 ]; then
    echo -e "${RED}Error: Failed to build the binary${NC}"
    exit 1
fi

echo -e "${YELLOW}Creating $INSTALL_DIR if it doesn't exist...${NC}"

# Create ~/.local/bin if it doesn't exist
mkdir -p "$INSTALL_DIR"

echo -e "${YELLOW}Installing to $INSTALL_DIR...${NC}"

# Install to user directory (no sudo needed)
mv "$BINARY_NAME" "$INSTALL_DIR/"
chmod +x "$INSTALL_DIR/$BINARY_NAME"

# Check if ~/.local/bin is in PATH and add it if needed
if [[ ":$PATH:" != *":$INSTALL_DIR:"* ]]; then
    echo -e "${YELLOW}$INSTALL_DIR is not in your PATH. Adding it automatically...${NC}"
    
    # Determine which shell config file to use
    SHELL_CONFIG=""
    if [[ "$SHELL" == *"zsh"* ]] || [[ -f "$HOME/.zshrc" ]]; then
        SHELL_CONFIG="$HOME/.zshrc"
    elif [[ "$SHELL" == *"bash"* ]] || [[ -f "$HOME/.bashrc" ]]; then
        SHELL_CONFIG="$HOME/.bashrc"
    elif [[ -f "$HOME/.bash_profile" ]]; then
        SHELL_CONFIG="$HOME/.bash_profile"
    else
        SHELL_CONFIG="$HOME/.zshrc"  # Default fallback
    fi
    
    # Check if the PATH export already exists in the config file
    PATH_EXPORT="export PATH=\"\$HOME/.local/bin:\$PATH\""
    if grep -q "HOME/.local/bin" "$SHELL_CONFIG" 2>/dev/null; then
        echo -e "${GREEN}PATH export already exists in $SHELL_CONFIG${NC}"
    else
        echo -e "${YELLOW}Adding PATH export to $SHELL_CONFIG${NC}"
        echo "" >> "$SHELL_CONFIG"
        echo "# Added by gci installer" >> "$SHELL_CONFIG"
        echo "$PATH_EXPORT" >> "$SHELL_CONFIG"
        echo -e "${GREEN}✅ Added $INSTALL_DIR to PATH in $SHELL_CONFIG${NC}"
    fi
    
    echo
    echo -e "${YELLOW}To use gci immediately, either:${NC}"
    echo -e "${GREEN}  1. Restart your terminal${NC}"
    echo -e "${GREEN}  2. Run: source $SHELL_CONFIG${NC}"
    echo -e "${GREEN}  3. Or run: export PATH=\"\$HOME/.local/bin:\$PATH\"${NC}"
    echo
fi

# Verify installation
if [ -f "$INSTALL_DIR/$BINARY_NAME" ]; then
    echo -e "${GREEN}✅ Successfully installed $BINARY_NAME to $INSTALL_DIR!${NC}"
    echo
    echo -e "${GREEN}Usage:${NC}"
    echo -e "  ${YELLOW}$BINARY_NAME${NC}              # Query your configured projects"
    echo -e "  ${YELLOW}$BINARY_NAME -a${NC}           # Query all issues (not just yours)"
    echo -e "  ${YELLOW}$BINARY_NAME -p PROJECT${NC}   # Query a specific project"
echo -e "  ${YELLOW}$BINARY_NAME board${NC}        # Open the personal Kanban TUI"
    echo -e "  ${YELLOW}$BINARY_NAME --help${NC}       # Show help"
    
    if command -v "$BINARY_NAME" &> /dev/null; then
        echo
        echo -e "${GREEN}You can now run '${YELLOW}$BINARY_NAME${GREEN}' from anywhere!${NC}"
    else
        echo
        echo -e "${YELLOW}After adding $INSTALL_DIR to your PATH, you'll be able to run '${BINARY_NAME}' from anywhere!${NC}"
    fi
else
    echo -e "${RED}❌ Installation failed${NC}"
    exit 1
fi