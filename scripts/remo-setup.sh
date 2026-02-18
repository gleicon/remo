#!/usr/bin/env bash
#
# remo-setup.sh — remo client/server setup
#
# Usage:
#   # Client (installs binary)
#   curl -sL https://raw.githubusercontent.com/gleicon/remo/main/scripts/remo-setup.sh | bash
#   curl -sL https://raw.githubusercontent.com/gleicon/remo/main/scripts/remo-setup.sh | bash -s -- client
#
#   # Server (full setup - requires root)
#   sudo curl -sL https://raw.githubusercontent.com/gleicon/remo/main/scripts/remo-setup.sh | bash -s -- server \
#     --domain yourdomain.tld
#
set -euo pipefail

REMO_VERSION="${REMO_VERSION:-}"
INSTALL_DIR="/usr/local/bin"
REMO_USER="remo"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BOLD='\033[1m'
NC='\033[0m'

info()  { printf "${GREEN}[INFO]${NC}  %s\n" "$*"; }
warn()  { printf "${YELLOW}[WARN]${NC}  %s\n" "$*"; }
error() { printf "${RED}[ERROR]${NC} %s\n" "$*" >&2; }
die()   { error "$@"; exit 1; }

usage() {
    cat <<'EOF'
Usage: remo-setup.sh [command] [options]

Commands:
  client               Install remo client binary (default)
  server               Setup remo server (requires root)

Server Options:
  --domain <domain>    Base domain (required for server)
  --behind-proxy      Use behind-proxy mode
  --skip-certs        Skip TLS certificate setup
  --email <email>     Email for certbot (required for certs)
  --admin-secret <s> Admin secret (auto-generated if omitted)

Examples:
  # Client only
  curl -sL https://raw.githubusercontent.com/gleicon/remo/main/scripts/remo-setup.sh | bash

  # Server (full setup)
  sudo curl -sL https://raw.githubusercontent.com/gleicon/remo/main/scripts/remo-setup.sh | bash -s -- server \
    --domain yourdomain.tld

  # Server behind nginx
  sudo curl -sL https://raw.githubusercontent.com/gleicon/remo/main/scripts/remo-setup.sh | bash -s -- server \
    --domain yourdomain.tld --behind-proxy --skip-certs
EOF
    exit 0
}

detect_os() {
    case "$(uname -s)" in
        Linux) echo "linux" ;;
        Darwin) echo "darwin" ;;
        *) die "Unsupported OS: $(uname -s)"
    esac
}

detect_arch() {
    case "$(uname -m)" in
        x86_64) echo "x86_64" ;;
        aarch64|arm64) echo "arm64" ;;
        *) die "Unsupported architecture: $(uname -m)"
    esac
}

get_version() {
    if [ -n "$REMO_VERSION" ]; then
        echo "$REMO_VERSION"
        return
    fi
    curl -sL https://api.github.com/repos/gleicon/remo/releases/latest | grep '"tag_name"' | sed 's/.*"v\([^"]*\)".*/\1/'
}

download_binary() {
    local os=$(detect_os)
    local arch=$(detect_arch)
    local version=$(get_version)

    [ -z "$version" ] && die "Failed to get remo version"

    info "Downloading remo $version for $os/$arch..."

    local filename="remo_${os}_${arch}.tar.gz"
    local url="https://github.com/gleicon/remo/releases/download/v${version}/${filename}"

    local tmpdir
    tmpdir=$(mktemp -d)
    trap "rm -rf $tmpdir" RETURN

    curl -sL "$url" -o "$tmpdir/remo.tar.gz" || die "Failed to download remo"
    tar -xzf "$tmpdir/remo.tar.gz" -C "$tmpdir"

    if [ "$(id -u)" = "0" ]; then
        cp "$tmpdir/remo" "$INSTALL_DIR/remo"
        chmod 755 "$INSTALL_DIR/remo"
        info "Installed to $INSTALL_DIR/remo"
    else
        if [ -w "$INSTALL_DIR" ]; then
            cp "$tmpdir/remo" "$INSTALL_DIR/remo"
            chmod 755 "$INSTALL_DIR/remo"
            info "Installed to $INSTALL_DIR/remo"
        else
            cp "$tmpdir/remo" "./remo"
            chmod 755 "./remo"
            INSTALL_DIR="$(pwd)"
            info "Installed to ./remo (not writable: $INSTALL_DIR)"
        fi
    fi
}

setup_client() {
    info "Setting up remo client..."
    download_binary

    local identity_dir="$HOME/.remo"
    if [ ! -d "$identity_dir" ]; then
        mkdir -p "$identity_dir"
        chmod 700 "$identity_dir"
    fi

    local identity_file="$identity_dir/identity.json"
    if [ ! -f "$identity_file" ]; then
        info "Generating identity..."
        "$INSTALL_DIR/remo" auth init -out "$identity_file" 2>/dev/null || true
    fi

    if [ -f "$identity_file" ]; then
        local pubkey
        pubkey=$("$INSTALL_DIR/remo" auth inspect -f "$identity_file" 2>/dev/null | grep "Public key:" | awk '{print $3}' || echo "")
        if [ -n "$pubkey" ]; then
            echo ""
            echo "${BOLD}=== Your public key (add to server) ===${NC}"
            echo "$pubkey *"
            echo ""
            echo "Add the above line to your server's /home/remo/.ssh/authorized_keys"
        fi
    fi

    echo ""
    echo "${GREEN}Client setup complete!${NC}"
    echo ""
    echo "Connect to a server:"
    echo "  remo connect --server yourserver.com --subdomain myapp --upstream http://127.0.0.1:3000"
    echo ""
}

setup_server() {
    [ "$(id -u)" = "0" ] || die "Server setup requires root (sudo)"

    local domain=""
    local mode="standalone"
    local skip_certs=false
    local email=""
    local admin_secret=""

    while [ $# -gt 0 ]; do
        case "$1" in
            --domain) domain="$2"; shift 2 ;;
            --behind-proxy) mode="behind-proxy"; shift ;;
            --skip-certs) skip_certs=true; shift ;;
            --email) email="$2"; shift 2 ;;
            --admin-secret) admin_secret="$2"; shift 2 ;;
            *) die "Unknown option: $1" ;;
        esac
    done

    [ -z "$domain" ] && die "--domain is required for server setup"

    info "Setting up remo server..."
    download_binary

    # Create remo user
    if ! id "$REMO_USER" >/dev/null 2>&1; then
        info "Creating system user '$REMO_USER'..."
        useradd --system --no-create-home --shell /bin/false "$REMO_USER" || true
    fi

    # SSH directory
    local ssh_dir="/home/$REMO_USER/.ssh"
    mkdir -p "$ssh_dir"
    chmod 700 "$ssh_dir"
    chown "$REMO_USER:$REMO_USER" "$ssh_dir"

    # Config directories
    mkdir -p /etc/remo /var/lib/remo
    chmod 755 /etc/remo /var/lib/remo

    # Generate admin secret
    if [ -z "$admin_secret" ]; then
        admin_secret=$(openssl rand -base64 32 | tr -d '/+=' | head -c 32)
    fi

    # Write server config
    if [ "$mode" = "behind-proxy" ]; then
        cat > /etc/remo/server.yaml <<YAML
listen: "127.0.0.1:18080"
domain: "$domain"
mode: behind-proxy
trusted_proxies:
  - "127.0.0.1/32"
authorized: /etc/remo/authorized.keys
state: /var/lib/remo/state.db
reserve: true
admin_secret: $admin_secret
YAML
    else
        [ -z "$email" ] && die "--email required for standalone mode (or use --skip-certs)"
        if [ "$skip_certs" = false ]; then
            if ! command -v certbot >/dev/null 2>&1; then
                warn "certbot not found, skipping certificate setup"
                skip_certs=true
            fi
        fi

        cat > /etc/remo/server.yaml <<YAML
listen: ":443"
domain: "$domain"
mode: standalone
tls_cert: /etc/remo/fullchain.pem
tls_key: /etc/remo/privkey.pem
authorized: /etc/remo/authorized.keys
state: /var/lib/remo/state.db
reserve: true
admin_secret: $admin_secret
YAML
    fi

    chmod 600 /etc/remo/server.yaml

    # Create authorized keys file
    touch /etc/remo/authorized.keys
    chmod 600 /etc/remo/authorized.keys

    # Systemd service
    cat > /etc/systemd/system/remo.service <<SERVICE
[Unit]
Description=Remo reverse tunnel server
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=$INSTALL_DIR/remo server --config /etc/remo/server.yaml
Restart=on-failure
RestartSec=5
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
SERVICE

    systemctl daemon-reload

    echo ""
    echo "${GREEN}=== Server setup complete! ===${NC}"
    echo ""
    echo "Config:        /etc/remo/server.yaml"
    echo "Authorized:    /home/$REMO_USER/.ssh/authorized_keys"
    echo "Admin secret:  $admin_secret"
    echo ""
    echo "Add client keys:"
    echo "  echo 'CLIENT_PUBLIC_KEY *' | sudo tee -a /home/$REMO_USER/.ssh/authorized_keys"
    echo ""
    echo "Start server:"
    echo "  sudo systemctl enable --now remo"
    echo ""
}

COMMAND="${1:-client}"
shift || true

case "$COMMAND" in
    client) setup_client ;;
    server) setup_server "$@" ;;
    -h|--help|help) usage ;;
    *) die "Unknown command: $COMMAND. Use: client, server, or --help" ;;
esac
