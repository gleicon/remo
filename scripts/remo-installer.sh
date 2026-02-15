#!/usr/bin/env bash
#
# remo-installer.sh — install remo binary
#
# Usage:
#   curl -sL https://raw.githubusercontent.com/gleicon/remo/main/scripts/remo-installer.sh | sh
#   curl -sL https://raw.githubusercontent.com/gleicon/remo/main/scripts/remo-installer.sh | sh -s -- -b /usr/local/bin
#
set -euo pipefail

INSTALL_DIR="/usr/local/bin"
FORCE=false

while [ $# -gt 0 ]; do
    case "$1" in
        -b|--bin-dir) INSTALL_DIR="$2"; shift 2 ;;
        -f|--force) FORCE=true; shift ;;
        -h|--help)
            echo "Usage: curl -sL https://raw.githubusercontent.com/gleicon/remo/main/scripts/remo-installer.sh | sh [-s] [options]"
            echo ""
            echo "Options:"
            echo "  -b, --bin-dir <dir>    Install binary to <dir> (default: /usr/local/bin)"
            echo "  -f, --force           Overwrite existing binary"
            echo "  -h, --help           Show this help"
            exit 0
            ;;
        *) die "Unknown option: $1" ;;
    esac
done

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BOLD='\033[1m'
NC='\033[0m'

info()  { printf "${GREEN}[INFO]${NC}  %s\n" "$*"; }
warn()  { printf "${YELLOW}[WARN]${NC}  %s\n" "$*"; }
error() { printf "${RED}[ERROR]${NC} %s\n" "$*" >&2; }
die()   { error "$@"; exit 1; }

# Detect architecture and OS
case "$(uname -m)" in
    x86_64) arch="x86_64" ;;
    aarch64|arm64) arch="arm64" ;;
    *) die "Unsupported architecture: $(uname -m)" ;;
esac

case "$(uname -s)" in
    Linux) os="linux" ;;
    Darwin) os="darwin" ;;
    MINGW*|MSYS*|CYGWIN*) os="windows" ;;
    *) die "Unsupported OS: $(uname -s)" ;;
esac

# Get latest release version
info "Checking for latest remo release..."
version=$(curl -sL https://api.github.com/repos/gleicon/remo/releases/latest | grep '"tag_name"' | sed 's/.*"v\([^"]*\)".*/\1/')

if [ -z "$version" ]; then
    die "Failed to get latest release version"
fi

info "Latest version: $version"

# Check if already installed
if [ -f "$INSTALL_DIR/remo" ] && [ "$FORCE" = false ]; then
    current=$("$INSTALL_DIR/remo" version 2>/dev/null | head -1 || echo "unknown")
    if [ "$current" = "v$version" ]; then
        info "remo $version is already installed"
        exit 0
    fi
    warn "remo is already installed (version: $current)"
    warn "Use --force to overwrite"
fi

# Download URL
filename="remo_${os}_${arch}.tar.gz"
url="https://github.com/gleicon/remo/releases/download/v${version}/${filename}"

info "Downloading $filename from $url..."
tmpdir=$(mktemp -d)
trap "rm -rf $tmpdir" EXIT

if ! curl -sL "$url" -o "$tmpdir/remo.tar.gz"; then
    die "Failed to download remo"
fi

info "Extracting..."
tar -xzf "$tmpdir/remo.tar.gz" -C "$tmpdir"

# Install
if [ "$(id -u)" = "0" ]; then
    cp "$tmpdir/remo" "$INSTALL_DIR/remo"
    chmod 755 "$INSTALL_DIR/remo"
else
    if [ ! -w "$INSTALL_DIR" ]; then
        warn "Not running as root and $INSTALL_DIR is not writable"
        warn "Installing to current directory instead"
        INSTALL_DIR="$(pwd)"
    fi
    cp "$tmpdir/remo" "$INSTALL_DIR/remo"
    chmod 755 "$INSTALL_DIR/remo"
fi

info "Installed remo $version to $INSTALL_DIR/remo"

echo ""
echo "Next steps:"
echo "  Client: remo auth init"
echo "  Server: curl -sL https://raw.githubusercontent.com/gleicon/remo/main/scripts/remo-setup.sh | bash -s -- server --domain yourdomain.tld --email you@example.com"
echo "or check the README for more install options as behind a proxy and no certificate provisioning"
echo ""
