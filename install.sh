#!/bin/bash
# Remo Installer - Server or Client
# Usage: curl -sL https://raw.githubusercontent.com/gleicon/remo/main/install.sh | bash

set -e

REMO_VERSION="${REMO_VERSION:-0.1.4}"
INSTALL_DIR="/usr/local/bin"
REMO_USER="remo"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
BOLD='\033[1m'
NC='\033[0m'

info() { printf "${GREEN}[INFO]${NC} %s\n" "$*"; }
warn() { printf "${YELLOW}[WARN]${NC} %s\n" "$*"; }
error() { printf "${RED}[ERROR]${NC} %s\n" "$*" >&2; }
ask() { printf "${BOLD}%s${NC} " "$*" >&2; }

# Read from terminal even when piped
read_input() {
    read -r "$1" </dev/tty
}

# Detect OS and arch
detect_platform() {
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    ARCH=$(uname -m)
    case "$ARCH" in
        x86_64) ARCH="x86_64" ;;
        aarch64|arm64) ARCH="arm64" ;;
        *) error "Unsupported arch: $ARCH"; exit 1 ;;
    esac
}

# Download remo binary
download_remo() {
    detect_platform
    local url="https://github.com/gleicon/remo/releases/download/v${REMO_VERSION}/remo_${OS}_${ARCH}.tar.gz"
    info "Downloading remo v${REMO_VERSION}..."
    curl -sL "$url" | tar -xz -C /tmp
    chmod +x /tmp/remo
}

# Server Installation
install_server() {
    info "Installing Remo Server..."
    
    # Move binary
    mv /tmp/remo "$INSTALL_DIR/remo"
    info "Binary installed to $INSTALL_DIR/remo"
    
    # Create user
    if ! id "$REMO_USER" &>/dev/null; then
        useradd --system --create-home --shell /bin/bash "$REMO_USER"
        info "Created user: $REMO_USER"
    fi
    
    mkdir -p /home/$REMO_USER/.ssh
    chmod 700 /home/$REMO_USER/.ssh
    chown $REMO_USER:$REMO_USER /home/$REMO_USER/.ssh
    
    # Ask configuration
    ask "Domain (e.g., remo.example.com):"
    read_input DOMAIN
    
    ask "Behind nginx proxy? [Y/n]:"
    read_input BEHIND_PROXY
    
    # Generate admin secret
    ADMIN_SECRET=$(openssl rand -base64 32 | tr -d '/+=' | head -c 32)
    
    # Create directories
    mkdir -p /etc/remo /var/lib/remo
    
    # Create config based on mode
    if [[ ! "$BEHIND_PROXY" =~ ^[Nn]$ ]]; then
        cat > /etc/remo/server.yaml <<EOF
listen: "127.0.0.1:18080"
domain: "$DOMAIN"
mode: behind-proxy
trtrusted_proxies:
  - "127.0.0.1/32"
authorized: /etc/remo/authorized.keys
state: /var/lib/remo/state.db
reserve: true
admin_secret: "$ADMIN_SECRET"
EOF
        info "Created behind-proxy config"
    else
        ask "Email for Let's Encrypt:"
        read_input EMAIL
        cat > /etc/remo/server.yaml <<EOF
listen: ":443"
domain: "$DOMAIN"
mode: standalone
tls_cert: /etc/remo/fullchain.pem
tls_key: /etc/remo/privkey.pem
authorized: /etc/remo/authorized.keys
state: /var/lib/remo/state.db
reserve: true
admin_secret: "$ADMIN_SECRET"
EOF
        info "Created standalone config"
    fi
    
    chmod 600 /etc/remo/server.yaml
    touch /etc/remo/authorized.keys
    chmod 600 /etc/remo/authorized.keys
    
    # Ask for client keys
    info ""
    info "Add client public keys (format: base64-key subdomain-rule)"
    info "Press Enter without input when done"
    info ""
    
    while true; do
        ask "Client key (or Enter to finish):"
        read_input KEY
        [ -z "$KEY" ] && break
        echo "$KEY" >> /etc/remo/authorized.keys
        info "Added key"
    done
    
    # Create systemd service
    cat > /etc/systemd/system/remo.service <<EOF
[Unit]
Description=Remo Server
After=network-online.target

[Service]
Type=simple
User=$REMO_USER
ExecStart=$INSTALL_DIR/remo server --config /etc/remo/server.yaml
Restart=on-failure

[Install]
WantedBy=multi-user.target
EOF
    
    systemctl daemon-reload
    systemctl enable remo
    systemctl start remo
    
    info ""
    info "${GREEN}Server installed and running!${NC}"
    info "Config: /etc/remo/server.yaml"
    info "Keys: /etc/remo/authorized.keys"
    info "Logs: journalctl -u remo -f"
}

# Client Installation
install_client() {
    info "Installing Remo Client..."
    
    # Install to local bin or current dir
    if [ -w "$INSTALL_DIR" ]; then
        mv /tmp/remo "$INSTALL_DIR/remo"
        BINARY="$INSTALL_DIR/remo"
    else
        mv /tmp/remo ./remo
        BINARY="./remo"
        warn "Installed to current directory (./remo)"
    fi
    
    # Create config dir
    mkdir -p ~/.remo
    
    # Ask configuration
    ask "Server domain (e.g., remo.example.com):"
    read_input SERVER
    
    ask "SSH key path [~/.ssh/id_rsa]:"
    read_input SSH_KEY
    SSH_KEY=${SSH_KEY:-~/.ssh/id_rsa}
    
    # Generate identity
    if [ ! -f ~/.remo/identity.json ]; then
        info "Generating identity..."
        $BINARY auth init -out ~/.remo/identity.json 2>/dev/null || true
    fi
    
    # Get public key
    PUBKEY=$($BINARY auth inspect -f ~/.remo/identity.json 2>/dev/null | grep "Public key:" | awk '{print $3}')
    
    info ""
    info "${YELLOW}Your public key:${NC} $PUBKEY *"
    info "${YELLOW}Give this to your server admin${NC}"
    info ""
    
    # Create client config
    cat > ~/.remo/config <<EOF
server: "$SERVER"
ssh_key: "$SSH_KEY"
identity: ~/.remo/identity.json
EOF
    
    # Test connection
    ask "Test connection now? [Y/n]:"
    read_input TEST
    if [[ ! "$TEST" =~ ^[Nn]$ ]]; then
        ask "Subdomain to test:"
        read_input SUBDOMAIN
        $BINARY connect --server $SERVER --subdomain $SUBDOMAIN --upstream http://127.0.0.1:3000
    fi
    
    info ""
    info "${GREEN}Client installed!${NC}"
    info "Config: ~/.remo/config"
    info "Identity: ~/.remo/identity.json"
    info ""
    info "Usage:"
    info "  $BINARY connect --server $SERVER --subdomain myapp --upstream http://127.0.0.1:3000 --tui"
}

# Main
echo "${BLUE}Remo Installer v${REMO_VERSION}${NC}"
echo ""

# Detect mode
if [ "$(id -u)" = "0" ]; then
    MODE="server"
    info "Detected: Server mode (running as root)"
else
    ask "Install as [s]erver or [c]lient? [c]:"
    read_input MODE_CHOICE
    MODE=${MODE_CHOICE:-c}
    if [[ "$MODE" =~ ^[Ss]$ ]]; then
        error "Server install requires root. Run: curl -sL ... | bash"
        exit 1
    fi
    MODE="client"
fi

download_remo

if [ "$MODE" = "server" ]; then
    install_server
else
    install_client
fi
