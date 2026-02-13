# remo

Milestone zero spike for the remo reverse tunnel. Build and test with:

```
make build
make test
```

Create an identity for the client:

```
./remo auth init -out ~/.remo/identity.json
```

Seed an authorized-keys file for the server (copy the public value produced by the previous command). When the server starts with `-state`, entries from this file are imported into the SQLite database and reused on subsequent launches:

```
echo BASE64_PUBLIC_KEY > /tmp/authorized.keys
```

## TLS certificates

Remo requires you to supply the TLS keypair. The recommended workflow:

1. Use an ACME client (certbot, acme.sh, lego, etc.) to issue a wildcard and apex cert via DNS-01. Example with certbot:
   ```
   sudo certbot certonly --manual --preferred-challenges dns \
     --email you@example.com -d rempapps.site -d '*.rempapps.site'
   ```
2. Copy the resulting `fullchain.pem` and `privkey.pem` to a secure path (e.g., `/etc/remo/fullchain.pem` and `/etc/remo/privkey.pem`) readable by the remo process (mode 600).
3. Configure a wildcard DNS record (e.g., `*.rempapps.site`) that points at your remo server’s public IP.
4. Start the server referencing the cert paths and providing an admin secret (used for `/status`/`/metrics` auth):
   ```
   ./remo server -listen :443 -domain rempapps.site -mode standalone \
     -tls-cert /etc/remo/fullchain.pem -tls-key /etc/remo/privkey.pem \
     -authorized /tmp/authorized.keys -state ~/.config/remo/state.db \
     -reserve -admin-secret changeme
   ```
5. Let your ACME client renew the cert and restart remo when files change. For certbot:
   ```
   sudo certbot renew --deploy-hook "cp /etc/letsencrypt/live/rempapps.site/fullchain.pem /etc/remo/fullchain.pem && cp /etc/letsencrypt/live/rempapps.site/privkey.pem /etc/remo/privkey.pem && systemctl restart remo"
   ```

Configuration files are supported via `--config path/to/server.yaml`. Flags always override file values. Example YAML:

```
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

Behind an existing reverse proxy, keep remo internal and trust only explicit proxy CIDRs:

```
./remo server -listen 127.0.0.1:18080 -domain rempapps.site -mode behind-proxy \
  -trusted-proxy 127.0.0.1/32 -trusted-hops 1 -authorized /tmp/authorized.keys \
  -state ~/.config/remo/state.db -admin-secret changeme
```

Expose a local service listening on port 3000 through subdomain demo:

```
./remo connect -server http://127.0.0.1:8080 -subdomain demo -upstream http://127.0.0.1:3000 -identity ~/.remo/identity.json -tui
```

With both processes running, requests sent to the server with the Host header `demo.rempapps.site` are forwarded to the local upstream. The tunnel handshake is authenticated via ed25519 signatures derived from the identity file, the server enforces subdomain ownership with the SQLite-backed allowlist and optional reservations, and request metadata is propagated through `X-Forwarded-*` plus `X-Remo-Subdomain` headers. Routing always keys off the Host header: as long as your wildcard DNS points to the remo server, each claimed subdomain will locate the correct tunnel.

### TUI controls (`remo connect -tui`)

- `/` start filter input; Enter saves the filter, Esc cancels
- `e` toggle errors-only view
- `p` pause/resume live updates
- `c` clear the request history

Server administration helpers:

```
# List keys stored in the SQLite state file
./remo keys list -state ~/.config/remo/state.db

# Add or update an authorized key; --prefix foo enforces foo-* subdomains
./remo keys add -state ~/.config/remo/state.db -pubkey BASE64 --prefix foo

# Remove an authorized key
./remo keys remove -state ~/.config/remo/state.db -pubkey BASE64

# Review or set reservations
./remo reservations list -state ~/.config/remo/state.db
./remo reservations set -state ~/.config/remo/state.db -subdomain demo -pubkey BASE64

# Rotate the local identity (backs up the previous file)
./remo auth rotate --path ~/.remo/identity.json

# Query /status (JSON)
./remo status --server https://remo.example.com --secret changeme

# Query /metrics
./remo status --server https://remo.example.com --secret changeme --metrics
```

Observability endpoints exposed by `remo server`:

- `GET /healthz` – readiness check used by process supervisors.
- `GET /status` – JSON snapshot including uptime, active tunnels, and database counts (requires `Authorization: Bearer <admin-secret>` over HTTPS).
- `GET /metrics` – Prometheus-style counters for tunnels, authorized keys, and reservations (same auth requirements).

Provide the admin secret via `--admin-secret`, the YAML config file, or allow remo to generate one when `--state` is configured (the secret is stored in SQLite and logged once). All CLI status calls and direct HTTP requests must use HTTPS and include the header `Authorization: Bearer <secret>`.
