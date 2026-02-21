#!/bin/bash
# Remo Installer - Server or Client
# Usage: curl -sL https://raw.githubusercontent.com/gleicon/remo/main/install.sh | bash

set -e

REMO_VERSION="${REMO_VERSION:-0.1.4}"
INSTALL_DIR="/usr/local/bin"
REMO_USER="remo"

info() { echo "[INFO] $*"; }
ask() { echo -n "$* "; }

# Read from terminal even when piped
read_input() {
    read -r "$1" </dev/tty
}

# Download remo
download_remo() {
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    ARCH=$(uname -m)
    case "$ARCH" in
        x86_64) ARCH="x86_64" ;;
        aarch64|arm64) ARCH="arm64" ;;
    esac
    
    URL="https://github.com/gleicon/remo/releases/download/v${REMO_VERSION}/remo_${OS}_${ARCH}.tar.gz"
    info "Downloading remo v${REMO_VERSION}..."
    curl -sL "$URL" | tar -xz -C /tmp
    chmod +x /tmp/remo
}

# Server Installation
install_server() {
    info "Installing Remo Server..."
    
    mv /tmp/remo "$INSTALL_DIR/remo"
    
    # Create remo user
    if ! id "$REMO_USER" &>/dev/null; then
        useradd --system --create-home --shell /bin/bash "$REMO_USER"
        info "Created user: $REMO_USER"
    fi
    
    mkdir -p /home/$REMO_USER/.ssh
    chmod 700 /home/$REMO_USER/.ssh
    chown $REMO_USER:$REMO_USER /home/$REMO_USER/.ssh
    
    # Configuration
    ask "Domain (e.g., remo.example.com):"
    read_input DOMAIN
    
    ask "Behind nginx proxy? [Y/n]:"
    read_input BEHIND_PROXY
    
    ADMIN_SECRET=$(openssl rand -base64 32 | tr -d '/+=' | head -c 32)
    
    mkdir -p /etc/remo /var/lib/remo
    
    if [[ ! "$BEHIND_PROXY" =~ ^[Nn]$ ]]; then
        cat > /etc/remo/server.yaml <<EOF
listen: "127.0.0.1:18080"
domain: "$DOMAIN"
mode: behind-proxy
trusted_proxies:
  - "127.0.0.1/32"
authorized: /etc/remo/authorized.keys
state: /var/lib/remo/state.db
reserve: true
admin_secret: "$ADMIN_SECRET"
EOF
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
    fi
    
    chmod 600 /etc/remo/server.yaml
    touch /etc/remo/authorized.keys
    chmod 600 /etc/remo/authorized.keys
    
    # SSH keys
    info ""
    info "Add SSH public keys for client authentication"
    info "Format: ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAID... user@host"
    info "These go in /home/remo/.ssh/authorized_keys"
    info "Press Enter when done"
    info ""
    
    while true; do
        ask "SSH public key (or Enter to finish):"
        read_input KEY
        [ -z "$KEY" ] && break
        echo "$KEY" >> /home/$REMO_USER/.ssh/authorized_keys
        info "Added SSH key"
    done
    
    chown -R $REMO_USER:$REMO_USER /home/$REMO_USER/.ssh
    
    # Systemd service
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
    info "✓ Server installed and running!"
    info "Config: /etc/remo/server.yaml"
    info "SSH keys: /home/remo/.ssh/authorized_keys"
    info "Remo auth: /etc/remo/authorized.keys"
    info "Logs: journalctl -u remo -f"
}

# Client Installation
install_client() {
    info "Installing Remo Client..."
    
    if [ -w "$INSTALL_DIR" ]; then
        mv /tmp/remo "$INSTALL_DIR/remo"
        BINARY="$INSTALL_DIR/remo"
    else
        mv /tmp/remo ./remo
        BINARY="./remo"
        info "Installed to current directory"
    fi
    
    # Configuration
    ask "Server domain:"
    read_input SERVER
    
    ask "SSH key for authentication [~/.ssh/id_ed25519]:"
    read_input SSH_KEY
    SSH_KEY=${SSH_KEY:-~/.ssh/id_ed25519}
    
    # Generate SSH key if doesn't exist
    if [ ! -f "$SSH_KEY" ]; then
        info "Generating SSH key..."
        ssh-keygen -t ed25519 -f "$SSH_KEY" -N "" -C "remo-client"
        info "SSH key created: $SSH_KEY"
    fi
    
    # Show public key
    info ""
    info "Your SSH public key (give this to server admin):"
    cat "${SSH_KEY}.pub"
    info ""
    
    # Create config
    mkdir -p ~/.remo
    cat > ~/.remo/config <<EOF
server: "$SERVER"
ssh_key: "$SSH_KEY"
EOF
    
    info "✓ Client installed!"
    info ""
    info "Usage:"
    info "  $BINARY connect --server $SERVER --subdomain myapp --upstream http://127.0.0.1:3000 --tui"
}

# Main
echo "Remo Installer v${REMO_VERSION}"
echo ""

if [ "$(id -u)" = "0" ]; then
    download_remo
    install_server
else
    download_remo
    install_client
fi
