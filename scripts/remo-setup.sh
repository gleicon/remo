#!/usr/bin/env bash
#
# remo-setup.sh — install and configure remo (client, server, or both)
#
# Usage:
#   ./scripts/remo-setup.sh client
#   ./scripts/remo-setup.sh server  --domain yourdomain.tls --email you@example.com
#   ./scripts/remo-setup.sh server  --domain yourdomain.tls --email you@example.com --behind-proxy
#   ./scripts/remo-setup.sh all     --domain yourdomain.tls --email you@example.com
#
set -euo pipefail

REMO_HOME="${REMO_HOME:-$HOME/.remo}"
REMO_CONFIG_DIR="${REMO_CONFIG_DIR:-$HOME/.config/remo}"
REMO_CERT_DIR="/etc/remo"
REMO_BIN="/usr/local/bin/remo"
REMO_IDENTITY="$REMO_HOME/identity.json"
REMO_AUTHORIZED="$REMO_HOME/authorized.keys"
REMO_STATE="$REMO_CONFIG_DIR/state.db"
REMO_SERVER_CONFIG="$REMO_CONFIG_DIR/server.yaml"

DOMAIN=""
EMAIL=""
MODE="standalone"
ADMIN_SECRET=""
SKIP_CERTS=false
SKIP_BUILD=false

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
Usage: remo-setup.sh <command> [options]

Commands:
  client                Set up the remo client (identity + directories)
  server                Set up the remo server (certs, config, authorized keys)
  all                   Set up both client and server on this machine

Options:
  --domain <domain>     Base domain (required for server/all)
  --email <email>       Email for certbot (required for server/all unless --skip-certs)
  --behind-proxy        Use behind-proxy mode instead of standalone
  --admin-secret <s>    Admin secret (auto-generated if omitted)
  --skip-certs          Skip certbot certificate provisioning
  --skip-build          Skip building from source (assume remo is already installed)
  -h, --help            Show this help

Environment:
  REMO_HOME             Base directory for remo files (default: ~/.remo)
  REMO_CONFIG_DIR       Config directory (default: ~/.config/remo)

Examples:
  # Client only (laptop)
  ./scripts/remo-setup.sh client

  # Standalone server (VPS)
  ./scripts/remo-setup.sh server --domain yourdomain.tls --email you@example.com

  # Server behind nginx
  ./scripts/remo-setup.sh server --domain yourdomain.tld --email you@example.com --behind-proxy

  # Both on same machine (dev/testing)
  ./scripts/remo-setup.sh all --domain yourdomain.tld --email you@example.com --skip-certs
EOF
    exit 0
}

check_command() {
    command -v "$1" >/dev/null 2>&1 || return 1
}

ensure_directories() {
    info "Creating directories"
    mkdir -p "$REMO_HOME"
    chmod 700 "$REMO_HOME"
    mkdir -p "$REMO_CONFIG_DIR"
    chmod 700 "$REMO_CONFIG_DIR"
}

build_remo() {
    if [ "$SKIP_BUILD" = true ]; then
        if ! check_command remo && [ ! -f ./remo ]; then
            die "remo binary not found. Remove --skip-build or install remo first."
        fi
        info "Skipping build (--skip-build)"
        return
    fi

    if [ -f ./Makefile ] && [ -f ./go.mod ]; then
        info "Building remo from source"
        check_command go || die "Go is required to build remo. Install from https://go.dev/dl/"
        make build
        info "Build complete: ./remo"
    elif check_command remo; then
        info "Using existing remo binary: $(command -v remo)"
    elif [ -f ./remo ]; then
        info "Using existing remo binary: ./remo"
    else
        die "No remo source tree or binary found. Run this script from the remo repo root."
    fi
}

remo_cmd() {
    if [ -f ./remo ]; then
        ./remo "$@"
    elif check_command remo; then
        remo "$@"
    else
        die "remo binary not found"
    fi
}

install_binary() {
    if [ -f ./remo ] && [ "$(id -u)" = "0" ]; then
        info "Installing remo to $REMO_BIN"
        cp ./remo "$REMO_BIN"
        chmod 755 "$REMO_BIN"
    elif [ -f ./remo ] && check_command sudo; then
        info "Installing remo to $REMO_BIN (requires sudo)"
        sudo cp ./remo "$REMO_BIN"
        sudo chmod 755 "$REMO_BIN"
    else
        warn "Skipping install to $REMO_BIN (run as root or copy manually)"
    fi
}

setup_identity() {
    if [ -f "$REMO_IDENTITY" ]; then
        info "Identity already exists: $REMO_IDENTITY"
    else
        info "Generating client identity"
        remo_cmd auth init -out "$REMO_IDENTITY"
        info "Identity created: $REMO_IDENTITY"
    fi

    PUBKEY=$(python3 -c "import json,sys; print(json.load(open('$REMO_IDENTITY'))['public'])" 2>/dev/null || true)
    if [ -z "$PUBKEY" ]; then
        PUBKEY=$(jq -r .public "$REMO_IDENTITY" 2>/dev/null || true)
    fi
    if [ -n "$PUBKEY" ]; then
        printf "\n${BOLD}Public key:${NC} %s\n" "$PUBKEY"
        printf "Use this key to authorize the client on the server.\n\n"
    fi
}

setup_authorized_keys() {
    if [ -f "$REMO_AUTHORIZED" ]; then
        info "Authorized keys file exists: $REMO_AUTHORIZED"
        return
    fi

    PUBKEY=""
    if [ -f "$REMO_IDENTITY" ]; then
        PUBKEY=$(python3 -c "import json,sys; print(json.load(open('$REMO_IDENTITY'))['public'])" 2>/dev/null || true)
        if [ -z "$PUBKEY" ]; then
            PUBKEY=$(jq -r .public "$REMO_IDENTITY" 2>/dev/null || true)
        fi
    fi

    if [ -n "$PUBKEY" ]; then
        info "Seeding authorized keys from local identity"
        echo "$PUBKEY *" > "$REMO_AUTHORIZED"
        chmod 600 "$REMO_AUTHORIZED"
        info "Created $REMO_AUTHORIZED (wildcard rule)"
    else
        warn "No identity found to seed authorized keys."
        warn "Create $REMO_AUTHORIZED manually:"
        warn "  echo '<BASE64_PUBLIC_KEY> *' > $REMO_AUTHORIZED"
    fi
}

generate_admin_secret() {
    if [ -n "$ADMIN_SECRET" ]; then
        return
    fi
    ADMIN_SECRET=$(openssl rand -base64 32 | tr -d '/+=' | head -c 32)
    info "Generated admin secret (save this): $ADMIN_SECRET"
}

setup_certs() {
    if [ "$SKIP_CERTS" = true ]; then
        info "Skipping certificate provisioning (--skip-certs)"
        return
    fi

    [ -n "$DOMAIN" ] || die "--domain is required for certificate setup"
    [ -n "$EMAIL" ]  || die "--email is required for certbot"

    check_command certbot || die "certbot is required. Install: sudo apt install certbot (or brew install certbot)"

    if [ -f "$REMO_CERT_DIR/fullchain.pem" ] && [ -f "$REMO_CERT_DIR/privkey.pem" ]; then
        info "Certificates already exist in $REMO_CERT_DIR"
        return
    fi

    info "Requesting wildcard certificate for $DOMAIN via DNS-01"
    warn "You will need to create a DNS TXT record when prompted."
    echo ""

    sudo certbot certonly \
        --manual \
        --preferred-challenges dns \
        --email "$EMAIL" \
        --agree-tos \
        --no-eff-email \
        -d "$DOMAIN" \
        -d "*.$DOMAIN"

    info "Copying certificates to $REMO_CERT_DIR"
    sudo mkdir -p "$REMO_CERT_DIR"
    sudo cp "/etc/letsencrypt/live/$DOMAIN/fullchain.pem" "$REMO_CERT_DIR/fullchain.pem"
    sudo cp "/etc/letsencrypt/live/$DOMAIN/privkey.pem"   "$REMO_CERT_DIR/privkey.pem"
    sudo chmod 600 "$REMO_CERT_DIR"/*.pem

    info "Setting up automatic renewal"
    local hook="cp /etc/letsencrypt/live/$DOMAIN/fullchain.pem $REMO_CERT_DIR/fullchain.pem && "
    hook+="cp /etc/letsencrypt/live/$DOMAIN/privkey.pem $REMO_CERT_DIR/privkey.pem"
    if check_command systemctl; then
        hook+=" && systemctl restart remo"
    fi
    sudo certbot renew --deploy-hook "$hook" --dry-run && \
        info "Renewal dry-run passed" || \
        warn "Renewal dry-run failed — check certbot configuration"
}

write_server_config() {
    [ -n "$DOMAIN" ] || die "--domain is required"

    generate_admin_secret

    info "Writing server config: $REMO_SERVER_CONFIG"

    if [ "$MODE" = "behind-proxy" ]; then
        cat > "$REMO_SERVER_CONFIG" <<YAML
listen: "127.0.0.1:18080"
domain: "$DOMAIN"
mode: behind-proxy
trusted_proxies:
  - "127.0.0.1/32"
trusted_hops: 1
authorized: "$REMO_AUTHORIZED"
state: "$REMO_STATE"
reserve: true
admin_secret: "$ADMIN_SECRET"
YAML
    else
        cat > "$REMO_SERVER_CONFIG" <<YAML
listen: ":443"
domain: "$DOMAIN"
mode: standalone
tls_cert: "$REMO_CERT_DIR/fullchain.pem"
tls_key: "$REMO_CERT_DIR/privkey.pem"
authorized: "$REMO_AUTHORIZED"
state: "$REMO_STATE"
reserve: true
admin_secret: "$ADMIN_SECRET"
YAML
    fi

    chmod 600 "$REMO_SERVER_CONFIG"
}

write_nginx_config() {
    [ "$MODE" = "behind-proxy" ] || return 0

    local nginx_conf="/etc/nginx/sites-available/remo-$DOMAIN.conf"
    if [ -f "$nginx_conf" ]; then
        info "Nginx config already exists: $nginx_conf"
        return
    fi

    info "Writing nginx config: $nginx_conf"
    sudo tee "$nginx_conf" > /dev/null <<NGINX
server {
    listen 443 ssl;
    server_name *.$DOMAIN;

    ssl_certificate     $REMO_CERT_DIR/fullchain.pem;
    ssl_certificate_key $REMO_CERT_DIR/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:18080;
        proxy_set_header Host              \$host;
        proxy_set_header X-Forwarded-For   \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_http_version 1.1;
        proxy_set_header Upgrade    \$http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}
NGINX

    if [ -d /etc/nginx/sites-enabled ]; then
        sudo ln -sf "$nginx_conf" "/etc/nginx/sites-enabled/remo-$DOMAIN.conf"
        info "Symlinked to sites-enabled"
    fi

    if check_command nginx; then
        sudo nginx -t && info "Nginx config test passed" || warn "Nginx config test failed"
    fi
}

write_systemd_unit() {
    check_command systemctl || return 0

    local unit="/etc/systemd/system/remo.service"
    if [ -f "$unit" ]; then
        info "Systemd unit already exists: $unit"
        return
    fi

    local bin="$REMO_BIN"
    [ -f "$bin" ] || bin="$(pwd)/remo"

    info "Writing systemd unit: $unit"
    sudo tee "$unit" > /dev/null <<UNIT
[Unit]
Description=Remo reverse tunnel server
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=$bin server --config $REMO_SERVER_CONFIG
Restart=on-failure
RestartSec=5
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
UNIT

    sudo systemctl daemon-reload
    info "Systemd unit installed. Start with: sudo systemctl enable --now remo"
}

print_client_summary() {
    echo ""
    printf "${GREEN}${BOLD}=== Client setup complete ===${NC}\n"
    echo ""
    echo "  Identity:  $REMO_IDENTITY"
    echo ""
    echo "  Connect to a server:"
    echo "    remo connect \\"
    echo "      -server https://<DOMAIN> \\"
    echo "      -subdomain <NAME> \\"
    echo "      -upstream http://127.0.0.1:<PORT> \\"
    echo "      -identity $REMO_IDENTITY \\"
    echo "      -tui"
    echo ""
}

print_server_summary() {
    echo ""
    printf "${GREEN}${BOLD}=== Server setup complete ===${NC}\n"
    echo ""
    echo "  Config:          $REMO_SERVER_CONFIG"
    echo "  Authorized keys: $REMO_AUTHORIZED"
    echo "  State DB:        $REMO_STATE"
    echo "  Admin secret:    $ADMIN_SECRET"
    echo ""
    if [ "$MODE" = "behind-proxy" ]; then
        echo "  Mode: behind-proxy (listening on 127.0.0.1:18080)"
        echo ""
        echo "  Start manually:"
        echo "    remo server --config $REMO_SERVER_CONFIG"
        echo ""
        echo "  Don't forget to reload nginx:"
        echo "    sudo systemctl reload nginx"
    else
        echo "  Mode: standalone (listening on :443)"
        echo ""
        echo "  Start manually:"
        echo "    sudo remo server --config $REMO_SERVER_CONFIG"
    fi
    echo ""
    if check_command systemctl; then
        echo "  Or with systemd:"
        echo "    sudo systemctl enable --now remo"
    fi
    echo ""
}

do_client() {
    ensure_directories
    build_remo
    setup_identity
    print_client_summary
}

do_server() {
    [ -n "$DOMAIN" ] || die "--domain is required for server setup"

    ensure_directories
    build_remo
    install_binary
    setup_identity
    setup_authorized_keys
    setup_certs
    write_server_config
    write_nginx_config
    write_systemd_unit
    print_server_summary
}

do_all() {
    [ -n "$DOMAIN" ] || die "--domain is required"

    ensure_directories
    build_remo
    install_binary
    setup_identity
    setup_authorized_keys
    setup_certs
    write_server_config
    write_nginx_config
    write_systemd_unit
    print_client_summary
    print_server_summary
}

COMMAND="${1:-}"
[ -n "$COMMAND" ] || usage
shift

while [ $# -gt 0 ]; do
    case "$1" in
        --domain)       DOMAIN="$2";       shift 2 ;;
        --email)        EMAIL="$2";        shift 2 ;;
        --behind-proxy) MODE="behind-proxy"; shift ;;
        --admin-secret) ADMIN_SECRET="$2"; shift 2 ;;
        --skip-certs)   SKIP_CERTS=true;   shift ;;
        --skip-build)   SKIP_BUILD=true;   shift ;;
        -h|--help)      usage ;;
        *)              die "Unknown option: $1" ;;
    esac
done

case "$COMMAND" in
    client) do_client ;;
    server) do_server ;;
    all)    do_all ;;
    -h|--help|help) usage ;;
    *)      die "Unknown command: $COMMAND. Use: client, server, or all" ;;
esac
