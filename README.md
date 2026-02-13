# remo

Single-binary reverse tunnel that exposes local services through public
`*.rempapps.site` subdomains. Runs standalone with its own TLS or behind an
existing nginx/reverse-proxy on your VPS.

## Quick start (step by step)

### 1. Build

```
git clone https://github.com/gleicon/remo.git
cd remo
make build        # produces ./remo
```

Other useful make targets: `make test`, `make cover`, `make dist` (cross-compile),
`make help` for the full list.

### 2. Generate a client identity

```
./remo auth init -out ~/.remo/identity.json
```

This creates an ed25519 keypair. Note the **public key** printed to stdout —
you will need it for step 3.

### 3. Authorize the client on the server

Create an authorized-keys file containing the public key from step 2:

```
echo '<BASE64_PUBLIC_KEY>' > /tmp/authorized.keys
```

You can restrict which subdomains a key may claim by appending a rule:

```
echo '<BASE64_PUBLIC_KEY> demo-*' > /tmp/authorized.keys
```

When the server starts with `--state`, entries from this file are imported into
the SQLite database and reused on subsequent launches.

### 4. Configure DNS

Point a wildcard DNS record at your VPS public IP:

```
*.rempapps.site.  A  <VPS_IP>
```

### 5. Obtain TLS certificates

Remo requires you to supply the TLS keypair. Use any ACME client to issue a
wildcard + apex cert via DNS-01. Example with certbot:

```
sudo certbot certonly --manual --preferred-challenges dns \
  --email you@example.com -d rempapps.site -d '*.rempapps.site'
```

Copy the resulting files to a secure path readable by remo (mode 600):

```
sudo mkdir -p /etc/remo
sudo cp /etc/letsencrypt/live/rempapps.site/fullchain.pem /etc/remo/
sudo cp /etc/letsencrypt/live/rempapps.site/privkey.pem   /etc/remo/
sudo chmod 600 /etc/remo/*.pem
```

Set up automatic renewal:

```
sudo certbot renew --deploy-hook \
  "cp /etc/letsencrypt/live/rempapps.site/fullchain.pem /etc/remo/fullchain.pem && \
   cp /etc/letsencrypt/live/rempapps.site/privkey.pem /etc/remo/privkey.pem && \
   systemctl restart remo"
```

### 6. Start the server

**Option A — Standalone (remo terminates TLS directly)**

```
./remo server \
  -listen :443 \
  -domain rempapps.site \
  -mode standalone \
  -tls-cert /etc/remo/fullchain.pem \
  -tls-key /etc/remo/privkey.pem \
  -authorized /tmp/authorized.keys \
  -state ~/.config/remo/state.db \
  -reserve \
  -admin-secret changeme
```

**Option B — Behind nginx (reuse existing TLS setup)**

```
./remo server \
  -listen 127.0.0.1:18080 \
  -domain rempapps.site \
  -mode behind-proxy \
  -trusted-proxy 127.0.0.1/32 \
  -trusted-hops 1 \
  -authorized /tmp/authorized.keys \
  -state ~/.config/remo/state.db \
  -admin-secret changeme
```

Add the following nginx server block:

```nginx
server {
    listen 443 ssl;
    server_name *.rempapps.site;

    ssl_certificate     /etc/remo/fullchain.pem;
    ssl_certificate_key /etc/remo/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:18080;
        proxy_set_header Host              $host;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_http_version 1.1;
        proxy_set_header Upgrade    $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}
```

### 7. Connect from your laptop

```
./remo connect \
  -server https://rempapps.site \
  -subdomain demo \
  -upstream http://127.0.0.1:3000 \
  -identity ~/.remo/identity.json
```

Your local port 3000 is now reachable at `https://demo.rempapps.site`.

Add `-tui` for a live top-like request log in your terminal.

### 8. Verify

```
curl https://demo.rempapps.site/
```

The request is forwarded through the tunnel to `http://127.0.0.1:3000` on your
laptop. Headers `X-Forwarded-For`, `X-Forwarded-Proto`, and `X-Remo-Subdomain`
are added automatically.

## Local development (no TLS)

For quick testing without certificates:

```
# Terminal 1 — start a local upstream
python3 -m http.server 3000

# Terminal 2 — start the server in behind-proxy mode (plain HTTP)
./remo server -listen 127.0.0.1:8080 -domain rempapps.site \
  -mode behind-proxy -authorized /tmp/authorized.keys \
  -state /tmp/remo-state.db -admin-secret dev

# Terminal 3 — connect
./remo connect -server http://127.0.0.1:8080 -subdomain demo \
  -upstream http://127.0.0.1:3000 -identity ~/.remo/identity.json -tui

# Terminal 4 — test it
curl -H "Host: demo.rempapps.site" http://127.0.0.1:8080/
```

## Configuration file

Flags can be provided via YAML with `--config path/to/server.yaml`.
Flags always override file values.

```yaml
listen: ":443"
domain: rempapps.site
mode: standalone
tls_cert: /etc/remo/fullchain.pem
tls_key: /etc/remo/privkey.pem
trusted_proxies:
  - 127.0.0.1/32
trusted_hops: 1
authorized: /tmp/authorized.keys
state: /home/you/.config/remo/state.db
reserve: true
admin_secret: changeme
```

## TUI controls (`remo connect --tui`)

| Key | Action |
|-----|--------|
| `/` | Start filter input (Enter saves, Esc cancels) |
| `e` | Toggle errors-only view (4xx/5xx) |
| `p` | Pause / resume live updates |
| `c` | Clear request history |

## Server administration

```bash
# List authorized keys
./remo keys list -state ~/.config/remo/state.db

# Add/update a key (--prefix foo restricts to foo-* subdomains)
./remo keys add -state ~/.config/remo/state.db -pubkey BASE64 --prefix foo

# Remove a key
./remo keys remove -state ~/.config/remo/state.db -pubkey BASE64

# List reservations
./remo reservations list -state ~/.config/remo/state.db

# Reserve a subdomain for a key
./remo reservations set -state ~/.config/remo/state.db -subdomain demo -pubkey BASE64

# Rotate the local client identity (backs up the previous file)
./remo auth rotate --path ~/.remo/identity.json
```

## Observability

Endpoints exposed by `remo server` (require `Authorization: Bearer <admin-secret>`):

| Endpoint | Description |
|----------|-------------|
| `GET /healthz` | Readiness check (no auth required) |
| `GET /status` | JSON snapshot: uptime, active tunnels, database counts |
| `GET /metrics` | Prometheus-style counters for tunnels, keys, reservations |

Query from the CLI:

```bash
./remo status --server https://rempapps.site --secret changeme
./remo status --server https://rempapps.site --secret changeme --metrics
```

The admin secret can be provided via `--admin-secret`, the YAML config, or
auto-generated when `--state` is configured (stored in SQLite and logged once
at startup).

## How it works

1. The **client** opens a WebSocket to the server's `/tunnel` endpoint and
   authenticates with an ed25519 signature over the claimed subdomain and
   current timestamp.
2. The **server** verifies the signature against the authorized-keys allowlist,
   checks subdomain reservation rules, and registers the tunnel.
3. Incoming HTTP requests with `Host: <sub>.rempapps.site` are matched to the
   active tunnel and forwarded to the client over the WebSocket.
4. The **client** proxies the request to the local upstream and returns the
   response back through the tunnel.
5. Reconnect with exponential backoff keeps the tunnel alive across transient
   network failures.

## Cross-compiling

```
make dist
```

Produces binaries under `dist/` for linux/amd64, linux/arm64, darwin/amd64,
darwin/arm64, and windows/amd64.

## Make targets

Run `make help` for the full list:

```
make all       # fmt, vet, test, then build (default)
make build     # compile the remo binary
make dist      # cross-compile for all platforms
make clean     # remove binary, dist/, and Go caches
make deps      # download module dependencies
make tidy      # run go mod tidy
make fmt       # format all Go source files
make vet       # run go vet
make lint      # vet + staticcheck
make test      # run tests
make test-v    # run tests verbose, no cache
make cover     # run tests with coverage summary
make check     # fmt + vet + test
```
