#!/bin/bash
#
# remo-client-setup.sh - Interactive Remo client setup
#
# Usage:
#   curl -sL https://raw.githubusercontent.com/gleicon/remo/main/scripts/remo-client-setup.sh | bash
#
# This script will:
#   1. Download remo binary for your OS/architecture
#   2. Install to /usr/local/bin (or current directory)
#   3. Generate Ed25519 identity key pair
#   4. Display your public key for server registration
#   5. Test the connection to your remo server
#

set -euo pipefail

# Configuration
REMO_VERSION="${REMO_VERSION:-latest}"
INSTALL_DIR="/usr/local/bin"
IDENTITY_DIR="${HOME}/.remo"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
BOLD='\033[1m'
NC='\033[0m'

# Logging functions
info()  { printf "${GREEN}[INFO]${NC}  %s\n" "$*"; }
warn()  { printf "${YELLOW}[WARN]${NC}  %s\n" "$*"; }
error() { printf "${RED}[ERROR]${NC} %s\n" "$*" >&2; }
die()   { error "$@"; exit 1; }
step()  { printf "\n${BLUE}[STEP]${NC} %s\n" "$*"; }
prompt() { printf "${BOLD}%s${NC} " "$*"; }

# Detect OS
detect_os() {
    case "$(uname -s)" in
        Linux) echo "linux" ;;
        Darwin) echo "darwin" ;;
        *) die "Unsupported OS: $(uname -s) (only Linux and macOS supported)" ;;
    esac
}

# Detect architecture
detect_arch() {
    case "$(uname -m)" in
        x86_64|amd64) echo "x86_64" ;;
        aarch64|arm64) echo "arm64" ;;
        *) die "Unsupported architecture: $(uname -m)" ;;
    esac
}

# Download and install remo binary
install_remo() {
    step "Downloading Remo..."
    
    local os=$(detect_os)
    local arch=$(detect_arch)
    local download_url
    
    if [ "${REMO_VERSION}" = "latest" ]; then
        download_url=$(curl -sL https://api.github.com/repos/gleicon/remo/releases/latest | \
            jq -r ".assets[] | select(.name | contains(\"${os}_${arch}\")) | .browser_download_url")
    else
        download_url="https://github.com/gleicon/remo/releases/download/v${REMO_VERSION}/remo_${os}_${arch}.tar.gz"
    fi
    
    if [ -z "${download_url}" ] || [ "${download_url}" = "null" ]; then
        die "Could not find remo binary for ${os}/${arch}"
    fi
    
    info "Downloading from: ${download_url}"
    
    local tmpdir
    tmpdir=$(mktemp -d)
    trap "rm -rf ${tmpdir}" EXIT
    
    curl -sL "${download_url}" -o "${tmpdir}/remo.tar.gz"
    tar -xzf "${tmpdir}/remo.tar.gz" -C "${tmpdir}"
    
    # Try to install to /usr/local/bin, fallback to current directory
    if [ -w "${INSTALL_DIR}" ]; then
        cp "${tmpdir}/remo" "${INSTALL_DIR}/remo"
        chmod +x "${INSTALL_DIR}/remo"
        info "Installed to: ${INSTALL_DIR}/remo"
        REMO_BIN="${INSTALL_DIR}/remo"
    else
        cp "${tmpdir}/remo" "./remo"
        chmod +x "./remo"
        warn "Installed to current directory: ./remo"
        warn "To install system-wide, run: sudo mv ./remo ${INSTALL_DIR}/"
        REMO_BIN="./remo"
    fi
}

# Generate identity key pair
generate_identity() {
    step "Generating identity key pair..."
    
    # Create identity directory
    mkdir -p "${IDENTITY_DIR}"
    chmod 700 "${IDENTITY_DIR}"
    
    local identity_file="${IDENTITY_DIR}/identity.json"
    
    if [ -f "${identity_file}" ]; then
        warn "Identity file already exists at ${identity_file}"
        read -p "Overwrite? [y/N]: " overwrite
        if [[ ! "${overwrite}" =~ ^[Yy]$ ]]; then
            info "Keeping existing identity"
            return
        fi
        cp "${identity_file}" "${identity_file}.backup.$(date +%s)"
    fi
    
    # Generate identity using remo auth init
    ${REMO_BIN} auth init -out "${identity_file}" 2>/dev/null || true
    
    if [ ! -f "${identity_file}" ]; then
        die "Failed to generate identity"
    fi
    
    chmod 600 "${identity_file}"
    info "Identity saved to: ${identity_file}"
}

# Display public key for server registration
show_public_key() {
    step "Your Public Key (Copy this to your server)"
    echo ""
    
    local identity_file="${IDENTITY_DIR}/identity.json"
    local public_key
    
    public_key=$(${REMO_BIN} auth inspect -f "${identity_file}" 2>/dev/null | grep "Public key:" | awk '{print $3}')
    
    if [ -z "${public_key}" ]; then
        die "Could not extract public key from identity"
    fi
    
    echo "${GREEN}╔════════════════════════════════════════════════════════════════╗${NC}"
    echo "${GREEN}║  Add this key to your server's /etc/remo/authorized.keys       ║${NC}"
    echo "${GREEN}╚════════════════════════════════════════════════════════════════╝${NC}"
    echo ""
    echo "${BOLD}${public_key} *${NC}"
    echo ""
    echo "${YELLOW}Instructions for server admin:${NC}"
    echo "  1. SSH into your remo server"
    echo "  2. Run: echo '${public_key} *' | sudo tee -a /etc/remo/authorized.keys"
    echo "  3. Or use the interactive setup script on the server"
    echo ""
    
    # Also save to clipboard if available
    if command -v pbcopy &>/dev/null; then
        echo "${public_key} *" | pbcopy
        info "Key copied to clipboard! (macOS)"
    elif command -v xclip &>/dev/null; then
        echo "${public_key} *" | xclip -selection clipboard
        info "Key copied to clipboard! (Linux)"
    elif command -v wl-copy &>/dev/null; then
        echo "${public_key} *" | wl-copy
        info "Key copied to clipboard! (Wayland)"
    fi
}

# Test connection to server
test_connection() {
    step "Test Connection"
    echo ""
    
    read -p "Enter your remo server (e.g., remo.example.com): " server
    
    if [ -z "${server}" ]; then
        warn "No server provided, skipping connection test"
        return
    fi
    
    read -p "Enter subdomain to claim [test]: " subdomain
    subdomain="${subdomain:-test}"
    
    read -p "Enter local service URL [http://127.0.0.1:3000]: " upstream
    upstream="${upstream:-http://127.0.0.1:3000}"
    
    echo ""
    info "Testing connection to ${server}..."
    info "Subdomain: ${subdomain}"
    info "Upstream: ${upstream}"
    echo ""
    
    echo "${YELLOW}Running: ${REMO_BIN} connect --server ${server} --subdomain ${subdomain} --upstream ${upstream}${NC}"
    echo ""
    
    # Offer to start the connection
    read -p "Start the connection now? [Y/n]: " start_now
    if [[ ! "${start_now}" =~ ^[Nn]$ ]]; then
        ${REMO_BIN} connect --server "${server}" --subdomain "${subdomain}" --upstream "${upstream}" --tui
    fi
}

# Show help and examples
show_help() {
    echo ""
    echo "${GREEN}╔════════════════════════════════════════════════════════════╗${NC}"
    echo "${GREEN}║          Remo Client Setup Complete!                       ║${NC}"
    echo "${GREEN}╚════════════════════════════════════════════════════════════╝${NC}"
    echo ""
    echo "${BOLD}Quick Start Commands:${NC}"
    echo ""
    echo "1. Connect to your server:"
    echo "   ${REMO_BIN} connect --server yourdomain.com --subdomain myapp --upstream http://127.0.0.1:3000 --tui"
    echo ""
    echo "2. Connection with custom SSH user:"
    echo "   ${REMO_BIN} connect --server ubuntu@yourdomain.com --subdomain myapp --upstream http://127.0.0.1:3000"
    echo ""
    echo "3. Show your public key anytime:"
    echo "   ${REMO_BIN} auth inspect -f ~/.remo/identity.json"
    echo ""
    echo "${BOLD}Files:${NC}"
    echo "   Binary:     ${REMO_BIN}"
    echo "   Identity:   ${IDENTITY_DIR}/identity.json"
    echo ""
    echo "${BOLD}Help:${NC}"
    echo "   ${REMO_BIN} --help"
    echo "   ${REMO_BIN} connect --help"
    echo ""
}

# Main setup flow
main() {
    echo "${GREEN}Remo Client Setup${NC}"
    echo "================="
    echo ""
    
    install_remo
    generate_identity
    show_public_key
    test_connection
    show_help
}

# Run main
main "$@"
