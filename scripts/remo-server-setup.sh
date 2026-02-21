#!/bin/bash
#
# remo-server-setup.sh - Complete Remo server setup for fresh Ubuntu VPS
# 
# Usage:
#   curl -sL https://raw.githubusercontent.com/gleicon/remo/main/scripts/remo-server-setup.sh | sudo bash
#
# This script will:
#   1. Update system packages
#   2. Install required dependencies
#   3. Create remo user with proper permissions
#   4. Download and install remo binary
#   5. Configure remo server
#   6. Prompt for client keys interactively
#   7. Start the remo service
#

set -euo pipefail

# Configuration
REMO_VERSION="${REMO_VERSION:-latest}"
REMO_USER="remo"
REMO_HOME="/home/${REMO_USER}"
REMO_CONFIG_DIR="/etc/remo"
REMO_DATA_DIR="/var/lib/remo"
INSTALL_DIR="/usr/local/bin"

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

# Check if running as root
check_root() {
    if [ "$(id -u)" -ne 0 ]; then
        die "This script must be run as root (use sudo)"
    fi
}

# Detect architecture
detect_arch() {
    case "$(uname -m)" in
        x86_64) echo "x86_64" ;;
        aarch64|arm64) echo "arm64" ;;
        *) die "Unsupported architecture: $(uname -m)" ;;
    esac
}

# Update system packages
update_system() {
    step "Updating system packages..."
    apt-get update
    apt-get upgrade -y
    apt-get install -y curl wget jq openssl
    info "System packages updated"
}

# Create remo user with proper permissions
create_remo_user() {
    step "Creating remo user..."
    
    if id "${REMO_USER}" &>/dev/null; then
        warn "User '${REMO_USER}' already exists"
    else
        # Create user with home directory and bash shell
        useradd --system --create-home --home-dir "${REMO_HOME}" --shell /bin/bash "${REMO_USER}"
        info "Created user: ${REMO_USER}"
    fi
    
    # Ensure home directory exists with correct permissions
    mkdir -p "${REMO_HOME}"
    chown "${REMO_USER}:${REMO_USER}" "${REMO_HOME}"
    chmod 755 "${REMO_HOME}"
    
    # Create SSH directory
    local ssh_dir="${REMO_HOME}/.ssh"
    mkdir -p "${ssh_dir}"
    chmod 700 "${ssh_dir}"
    chown "${REMO_USER}:${REMO_USER}" "${ssh_dir}"
    
    info "SSH directory ready: ${ssh_dir}"
}

# Download and install remo binary
install_remo() {
    step "Installing Remo server..."
    
    local arch=$(detect_arch)
    local download_url
    
    if [ "${REMO_VERSION}" = "latest" ]; then
        # Get latest release URL
        download_url=$(curl -sL https://api.github.com/repos/gleicon/remo/releases/latest | \
            jq -r ".assets[] | select(.name | contains(\"linux_${arch}")) | .browser_download_url")
    else
        download_url="https://github.com/gleicon/remo/releases/download/v${REMO_VERSION}/remo_linux_${arch}.tar.gz"
    fi
    
    if [ -z "${download_url}" ] || [ "${download_url}" = "null" ]; then
        die "Could not find remo binary for architecture: ${arch}"
    fi
    
    info "Downloading from: ${download_url}"
    
    local tmpdir
    tmpdir=$(mktemp -d)
    trap "rm -rf ${tmpdir}" EXIT
    
    curl -sL "${download_url}" -o "${tmpdir}/remo.tar.gz"
    tar -xzf "${tmpdir}/remo.tar.gz" -C "${tmpdir}"
    
    cp "${tmpdir}/remo" "${INSTALL_DIR}/remo"
    chmod +x "${INSTALL_DIR}/remo"
    
    info "Remo installed to: ${INSTALL_DIR}/remo"
    "${INSTALL_DIR}/remo" --help | head -5
}

# Setup remo directories and config
setup_remo_config() {
    step "Configuring Remo server..."
    
    # Create directories
    mkdir -p "${REMO_CONFIG_DIR}" "${REMO_DATA_DIR}"
    chmod 755 "${REMO_CONFIG_DIR}" "${REMO_DATA_DIR}"
    
    # Generate admin secret
    local admin_secret
    admin_secret=$(openssl rand -base64 32 | tr -d '/+=' | head -c 32)
    
    # Prompt for domain
    local domain
    read -p "Enter your domain (e.g., remoapps.site): " domain
    
    if [ -z "${domain}" ]; then
        die "Domain is required"
    fi
    
    # Ask for deployment mode
    echo ""
    echo "Select deployment mode:"
    echo "  1) Standalone (Remo handles SSL directly with Let's Encrypt)"
    echo "  2) Behind Nginx (Use nginx as reverse proxy - recommended for production)"
    read -p "Choice [1-2]: " mode_choice
    
    case "${mode_choice}" in
        1)
            setup_standalone_config "${domain}" "${admin_secret}"
            ;;
        2)
            setup_behind_proxy_config "${domain}" "${admin_secret}"
            ;;
        *)
            warn "Invalid choice, defaulting to behind-proxy mode"
            setup_behind_proxy_config "${domain}" "${admin_secret}"
            ;;
    esac
    
    # Create authorized keys file
    touch "${REMO_CONFIG_DIR}/authorized.keys"
    chmod 600 "${REMO_CONFIG_DIR}/authorized.keys"
    
    info "Configuration complete"
    echo "  Config: ${REMO_CONFIG_DIR}/server.yaml"
    echo "  Admin Secret: ${admin_secret}"
}

# Setup standalone configuration
setup_standalone_config() {
    local domain="$1"
    local admin_secret="$2"
    
    info "Setting up standalone mode..."
    
    # Prompt for email
    local email
    read -p "Enter email for Let's Encrypt: " email
    
    if [ -z "${email}" ]; then
        die "Email is required for Let's Encrypt"
    fi
    
    # Install certbot if not present
    if ! command -v certbot &>/dev/null; then
        info "Installing certbot..."
        apt-get install -y certbot
    fi
    
    # Create config
    cat > "${REMO_CONFIG_DIR}/server.yaml" <<EOF
listen: ":443"
domain: "${domain}"
mode: standalone
tls_cert: "${REMO_CONFIG_DIR}/fullchain.pem"
tls_key: "${REMO_CONFIG_DIR}/privkey.pem"
authorized: "${REMO_CONFIG_DIR}/authorized.keys"
state: "${REMO_DATA_DIR}/state.db"
reserve: true
admin_secret: "${admin_secret}"
EOF
    
    chmod 600 "${REMO_CONFIG_DIR}/server.yaml"
    info "Standalone config created"
}

# Setup behind-proxy configuration
setup_behind_proxy_config() {
    local domain="$1"
    local admin_secret="$2"
    
    info "Setting up behind-proxy mode..."
    
    # Create config
    cat > "${REMO_CONFIG_DIR}/server.yaml" <<EOF
listen: "127.0.0.1:18080"
domain: "${domain}"
mode: behind-proxy
trusted_proxies:
  - "127.0.0.1/32"
authorized: "${REMO_CONFIG_DIR}/authorized.keys"
state: "${REMO_DATA_DIR}/state.db"
reserve: true
admin_secret: "${admin_secret}"
EOF
    
    chmod 600 "${REMO_CONFIG_DIR}/server.yaml"
    info "Behind-proxy config created"
    warn "Remember to configure nginx as reverse proxy"
    echo "  See: https://github.com/gleicon/remo/blob/main/docs/nginx.md"
}

# Interactive client key setup
setup_client_keys() {
    step "Client Key Setup"
    
    echo ""
    echo "You can add client public keys now. Clients will use these keys to authenticate."
    echo "Keys should be in the format: <base64-public-key> <subdomain-rule>"
    echo ""
    echo "Subdomain rules:"
    echo "  *           - Allow any subdomain"
    echo "  prefix-*    - Allow subdomains starting with 'prefix-'"
    echo "  exact-name  - Allow only 'exact-name' subdomain"
    echo ""
    
    while true; do
        echo ""
        read -p "Enter client public key (or 'done' to finish): " client_key
        
        if [ "${client_key}" = "done" ] || [ -z "${client_key}" ]; then
            break
        fi
        
        # Validate key format (should be base64)
        if ! echo "${client_key}" | grep -qE '^[A-Za-z0-9+/=]+'; then
            warn "Invalid key format. Key should be base64 encoded."
            continue
        fi
        
        read -p "Enter subdomain rule [default: *]: " subdomain_rule
        subdomain_rule="${subdomain_rule:-*}"
        
        # Add to authorized keys
        echo "${client_key} ${subdomain_rule}" >> "${REMO_CONFIG_DIR}/authorized.keys"
        info "Added key with rule: ${subdomain_rule}"
    done
    
    info "Client keys configured"
    echo "  Total keys: $(wc -l < "${REMO_CONFIG_DIR}/authorized.keys")"
}

# Create systemd service
create_systemd_service() {
    step "Creating systemd service..."
    
    cat > /etc/systemd/system/remo.service <<EOF
[Unit]
Description=Remo reverse tunnel server
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=${REMO_USER}
Group=${REMO_USER}
ExecStart=${INSTALL_DIR}/remo server --config ${REMO_CONFIG_DIR}/server.yaml
Restart=on-failure
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=remo

# Security hardening
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=${REMO_DATA_DIR} ${REMO_CONFIG_DIR}

[Install]
WantedBy=multi-user.target
EOF
    
    systemctl daemon-reload
    info "Systemd service created"
}

# Start remo service
start_remo() {
    step "Starting Remo service..."
    
    systemctl enable remo
    systemctl start remo
    
    sleep 2
    
    if systemctl is-active --quiet remo; then
        info "✓ Remo service is running"
        systemctl status remo --no-pager | head -20
    else
        error "✗ Remo service failed to start"
        systemctl status remo --no-pager
        exit 1
    fi
}

# Display completion message
show_completion() {
    local domain
    domain=$(grep "^domain:" "${REMO_CONFIG_DIR}/server.yaml" | awk '{print $2}')
    
    echo ""
    echo "${GREEN}╔════════════════════════════════════════════════════════════╗${NC}"
    echo "${GREEN}║          Remo Server Setup Complete!                       ║${NC}"
    echo "${GREEN}╚════════════════════════════════════════════════════════════╝${NC}"
    echo ""
    echo "${BOLD}Configuration:${NC}"
    echo "  Domain:        ${domain}"
    echo "  Config:        ${REMO_CONFIG_DIR}/server.yaml"
    echo "  Authorized:    ${REMO_CONFIG_DIR}/authorized.keys"
    echo "  Data:          ${REMO_DATA_DIR}"
    echo ""
    echo "${BOLD}Management Commands:${NC}"
    echo "  View logs:     journalctl -u remo -f"
    echo "  Status:        systemctl status remo"
    echo "  Restart:       systemctl restart remo"
    echo "  Stop:          systemctl stop remo"
    echo ""
    echo "${BOLD}Add more client keys:${NC}"
    echo "  echo '<public-key> <rule>' | sudo tee -a ${REMO_CONFIG_DIR}/authorized.keys"
    echo ""
    echo "${BOLD}Client Connection Example:${NC}"
    echo "  remo connect --server ${domain} --subdomain myapp --upstream http://127.0.0.1:3000 --tui"
    echo ""
}

# Main setup flow
main() {
    echo "${GREEN}Remo Server Setup${NC}"
    echo "=================="
    echo ""
    
    check_root
    update_system
    create_remo_user
    install_remo
    setup_remo_config
    setup_client_keys
    create_systemd_service
    start_remo
    show_completion
}

# Run main function
main "$@"
