# API Reference

Remo server exposes a REST API for tunnel management, health checks, and monitoring.

---

## Base URL

```
http://127.0.0.1:18080          # Local access (via SSH tunnel)
https://admin.yourdomain.com     # With nginx + SSL (requires admin_secret)
```

---

## Authentication

### Client Authentication

Most endpoints require the `X-Remo-Publickey` header:
```bash
X-Remo-Publickey: BASE64_PUBLIC_KEY
```

Get your public key:
```bash
remo auth inspect -f ~/.remo/identity.json
# Output: Public key: Nc8rhlSGkS1qATcyIOkytjuMhpdZoswbrLKwoCWKQEU=
```

### Admin Authentication

Admin endpoints require the `X-Admin-Secret` header:
```bash
X-Admin-Secret: your-secure-secret
```

Set in `/etc/remo/server.yaml`:
```yaml
admin_secret: your-secure-secret-here
```

**Rate Limiting:** Admin endpoints are rate-limited to 5 requests per minute per IP.

---

## Endpoints

### Register Tunnel

Register a new subdomain tunnel.

**Endpoint:** `POST /register`

**Headers:**
- `X-Remo-Publickey` (required)
- `Content-Type: application/json`

**Body:**
```json
{
  "subdomain": "myapp",
  "remote_port": 34291
}
```

**Response (200 OK):**
```json
{
  "subdomain": "myapp",
  "url": "https://myapp.yourdomain.com",
  "status": "ok"
}
```

**Error Responses:**
- `400` — Missing subdomain or invalid format
- `403` — Authorization denied (subdomain not allowed for this key)
- `409` — Subdomain already in use
- `429` — Rate limited (too many registration attempts)

**Example:**
```bash
curl -X POST http://127.0.0.1:18080/register \
  -H "X-Remo-Publickey: Nc8rhlSGkS1qATcyIOkytjuMhpdZoswbrLKwoCWKQEU=" \
  -H "Content-Type: application/json" \
  -d '{"subdomain": "myapp", "remote_port": 34291}'
```

---

### Unregister Tunnel

Gracefully unregister a tunnel.

**Endpoint:** `POST /unregister`

**Headers:**
- `X-Remo-Publickey` (required)
- `Content-Type: application/json`

**Body:**
```json
{
  "subdomain": "myapp"
}
```

**Response (200 OK):**
```json
{
  "status": "unregistered",
  "subdomain": "myapp"
}
```

**Error Responses:**
- `403` — Not authorized to unregister this subdomain
- `404` — Tunnel not found

---

### Health Check Ping

Send a health check ping to keep tunnel alive.

**Endpoint:** `POST /ping?subdomain=<subdomain>`

**Headers:**
- `X-Remo-Publickey` (required)

**Response (200 OK):**
```json
{
  "status": "ok",
  "subdomain": "myapp"
}
```

**Error Responses:**
- `403` — Not authorized
- `404` — Tunnel not found

**Note:** The client automatically sends pings every 30 seconds when TUI is enabled.

---

### List Connections

Get list of your active connections.

**Endpoint:** `GET /connections`

**Headers:**
- `X-Remo-Publickey` (required)

**Response (200 OK):**
```json
{
  "connections": [
    {
      "subdomain": "myapp",
      "port": 34291,
      "status": "active",
      "created_at": "2026-02-22T14:30:00Z",
      "last_ping": "2026-02-22T14:32:15Z"
    }
  ]
}
```

**Status Values:**
- `active` — Tunnel registered and receiving pings
- `stale` — Tunnel registered but no ping > timeout (5 min default)

---

### System Health

Check if server is healthy.

**Endpoint:** `GET /healthz`

**Response (200 OK):**
```json
{
  "status": "healthy"
}
```

**No authentication required.**

---

### Server Status

Get server statistics (admin only).

**Endpoint:** `GET /status`

**Headers:**
- `X-Admin-Secret` (required)

**Response (200 OK):**
```json
{
  "active_tunnels": 5,
  "tunnels": ["myapp", "api", "blog", "shop", "dev"],
  "authorized_keys": 12,
  "reservations": 8,
  "uptime_seconds": 86400,
  "total_requests": 15234,
  "total_errors": 23,
  "total_bytes_in": 10485760,
  "total_bytes_out": 52428800
}
```

---

### Cleanup Stale Tunnels (Admin)

Manually trigger cleanup of stale tunnels.

**Endpoint:** `POST /admin/cleanup`

**Headers:**
- `X-Admin-Secret` (required)

**Response (200 OK):**
```json
{
  "status": "ok",
  "removed": 3,
  "total": 5,
  "stale": 0
}
```

**Error Responses:**
- `401` — Invalid admin secret
- `429` — Rate limit exceeded (max 5/min/IP)

**Note:** Cleanup also runs automatically every 30 seconds.

---

### Metrics (Prometheus)

Prometheus-compatible metrics endpoint.

**Endpoint:** `GET /metrics`

**Headers:**
- `X-Admin-Secret` (required)

**Response:**
```
# HELP remo_requests_total Total requests processed
# TYPE remo_requests_total counter
remo_requests_total 15234

# HELP remo_errors_total Total errors
# TYPE remo_errors_total counter
remo_errors_total 23

# HELP remo_bytes_total Total bytes transferred
# TYPE remo_bytes_total counter
remo_bytes_in_total 10485760
remo_bytes_out_total 52428800

# HELP remo_active_tunnels Number of active tunnels
# TYPE remo_active_tunnels gauge
remo_active_tunnels 5

# HELP remo_tunnel_port Assigned tunnel port
# TYPE remo_tunnel_port gauge
remo_tunnel_port{subdomain="myapp"} 34291
```

---

### Events Stream

Server-sent events for real-time request logging.

**Endpoint:** `GET /events`

**Headers:**
- `X-Remo-Publickey` (required)

**Response:** Server-sent event stream

```
event: request
data: {"time":"2026-02-22T14:32:15Z","method":"GET","path":"/api/users","status":200,"latency":45,"remote":"192.168.1.100","bytes_in":0,"bytes_out":2048}

event: request
data: {"time":"2026-02-22T14:32:18Z","method":"POST","path":"/api/users","status":201,"latency":120,"remote":"192.168.1.100","bytes_in":512,"bytes_out":1024}
```

**Note:** Used by the TUI dashboard for real-time updates.

---

### Proxy (Default)

Proxy requests to registered tunnels.

**Endpoint:** `GET|POST|PUT|DELETE|PATCH /` (any path)

**Host Header:** `<subdomain>.yourdomain.com`

**Response:** Proxied response from upstream service

**Error Responses:**
- `404` — Tunnel not found OR No upstream available (both return 404 for security)
- Headers: `X-Remo-Error: no-tunnel` or `X-Remo-Error: no-upstream`

---

## Error Handling

### Status Codes

| Code | Meaning |
|------|---------|
| 200 | Success |
| 400 | Bad request (missing/invalid parameters) |
| 401 | Unauthorized (invalid admin secret) |
| 403 | Forbidden (not authorized for this action) |
| 404 | Not found (tunnel or upstream unavailable) |
| 409 | Conflict (subdomain already in use) |
| 429 | Too many requests (rate limited) |
| 500 | Internal server error |

### Security Note

All tunnel/upstream errors return **404** (not 502) to prevent subdomain enumeration attacks. Check `X-Remo-Error` header for debugging.

---

## Rate Limiting

### Limits

| Endpoint | Limit |
|----------|-------|
| `/register` | 10/min per IP |
| `/admin/cleanup` | 5/min per IP |
| Other endpoints | 100/min per IP |

### Rate Limit Response

```http
HTTP/1.1 429 Too Many Requests
Retry-After: 60

{
  "error": "rate limit exceeded"
}
```

---

## Client IP Detection

When behind nginx, Remo extracts the real client IP from `X-Forwarded-For` header:

```nginx
# In nginx config
proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
```

The server:
1. Checks if request comes from `trusted_proxies` (configured in server.yaml)
2. Extracts first IP from `X-Forwarded-For` chain
3. Falls back to `X-Real-Ip` if X-Forwarded-For is empty
4. Returns direct `RemoteAddr` if not from trusted proxy

This ensures the TUI shows real visitor IPs, not just `127.0.0.1`.

---

## Example: curl Commands

### Full Registration Flow

```bash
# 1. Get your public key
PUBKEY=$(remo auth inspect -f ~/.remo/identity.json | grep "Public key:" | awk '{print $3}')
echo "Public key: $PUBKEY"

# 2. Register tunnel (through SSH tunnel)
curl -X POST http://127.0.0.1:18080/register \
  -H "X-Remo-Publickey: $PUBKEY" \
  -H "Content-Type: application/json" \
  -d '{"subdomain": "myapp", "remote_port": 34291}'

# 3. List your connections
curl http://127.0.0.1:18080/connections \
  -H "X-Remo-Publickey: $PUBKEY"

# 4. Send health ping
curl -X POST "http://127.0.0.1:18080/ping?subdomain=myapp" \
  -H "X-Remo-Publickey: $PUBKEY"

# 5. Unregister
curl -X POST http://127.0.0.1:18080/unregister \
  -H "X-Remo-Publickey: $PUBKEY" \
  -H "Content-Type: application/json" \
  -d '{"subdomain": "myapp"}'
```

### Admin Operations

```bash
ADMIN_SECRET="your-secret-here"

# Check status
curl http://127.0.0.1:18080/status \
  -H "X-Admin-Secret: $ADMIN_SECRET"

# Cleanup stale tunnels
curl -X POST http://127.0.0.1:18080/admin/cleanup \
  -H "X-Admin-Secret: $ADMIN_SECRET"

# Get Prometheus metrics
curl http://127.0.0.1:18080/metrics \
  -H "X-Admin-Secret: $ADMIN_SECRET"
```

---

## WebSocket Support

WebSocket connections are proxied transparently:

```
Client ──WebSocket──▶ Nginx ──▶ Remo Server ──▶ SSH Tunnel ──▶ Your Service
```

**Requirements:**
- Nginx must support WebSocket proxying
- Add to nginx config:
```nginx
proxy_http_version 1.1;
proxy_set_header Upgrade $http_upgrade;
proxy_set_header Connection "upgrade";
```

No special handling needed in Remo.
