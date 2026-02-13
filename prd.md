remo — Product Specification (v1)

Single-binary reverse tunnel + public ingress for *.rempapps.site, runnable on laptop and VPS. Supports direct TLS termination or running behind an existing nginx (“drop-in to my VPS config”).

⸻

1) Goals, non-goals, guiding principles

Goals
	•	One binary: remo with subcommands (server, connect, status, auth).
	•	Wildcard domain routing: https://<sub>.rempapps.site → localhost:<port> behind NAT.
	•	Two deployment modes on VPS:
	1.	Standalone: remo terminates TLS and routes directly.
	2.	Behind nginx: reuse existing TLS/certs and reverse-proxy to remo server.
	•	Cross-platform: Linux amd64/arm64, macOS, Windows.
	•	Start simple: single tenant, minimal config, boring defaults.

Non-goals (MVP)
	•	Full ngrok-like UI/inspection proxy.
	•	Multi-region edge network.
	•	OIDC/SSO gating (can be future “team mode”, like pgrok).  ￼

Design principles
	•	“Make the 80% path one command.”
	•	Default-secure: TLS everywhere, authenticated tunnels, explicit routing ownership.
	•	“No mandatory extra daemons.” (nginx optional, not required)

⸻

2) Reference patterns (what to borrow)
	•	pgrok: uses SSH remote port forwarding as the tunnel primitive and dynamically configures routing on connect/disconnect; it also ships a TUI client concept.  ￼
	•	holepunch-client: “semi-persistent” SSH reverse tunnel with keepalives + reconnect behavior; this is a proven minimalistic direction.  ￼
	•	awesome-tunneling: confirms the target user story (“self-hosted dev tunnels with auto HTTPS behind NAT”).  ￼
	•	tunnel.pyjam.as: demonstrates a WireGuard-based approach, but it’s not the simplest MVP path for single-binary UX.  ￼

⸻

3) Product overview

3.1 Components

remo server (VPS)
	•	Public ingress (HTTP/S), routing, tunnel registry, auth.
	•	Supports:
	•	Standalone TLS termination (recommended default when you want “no nginx”)
	•	Behind-nginx mode (reuse existing certs + port 443 setup)

remo connect (client)
	•	Creates/maintains a secure tunnel to the VPS.
	•	Binds a public subdomain and maps it to a local upstream (localhost:3000).
	•	Reconnect, backoff, status reporting.

⸻

4) Transport choice: SSH vs WireGuard (MVP decision)

MVP: SSH-like reverse tunnel semantics (userland)

Why this is the MVP bet
	•	No admin privileges / no TUN device required on clients.
	•	NAT traversal is simple: client dials VPS outbound.
	•	The model is widely used and aligns with pgrok/holepunch patterns.  ￼

Implementation note
	•	You can:
	•	implement a small embedded SSH server/client in Go to get multiplexed channels and familiar key auth semantics, or
	•	use a custom TLS multiplex protocol (still “SSH-like” behavior from the user POV).

Later: WireGuard mode (advanced)

WireGuard shines for “real VPN” / multi-service routing, but it expands scope (IPAM, firewalling, routing policy). Tools like tunnel.pyjam.as show the concept works.  ￼
Plan: add WireGuard as an optional “power mode” after HTTP tunnels are rock solid.

⸻

5) VPS deployment modes (MVP requirement)

5.1 Mode A — Standalone (no nginx)

remo server binds directly to :443 and does:
	•	TLS termination (ACME)
	•	Host-based routing
	•	Tunnel forwarding

TLS strategy
	•	Prefer wildcard cert for *.rempapps.site (DNS-01).
	•	Backup: bring-your-own cert (--tls-cert, --tls-key).

5.2 Mode B — Behind nginx (reuse existing VPS setup)

Goal: If the VPS already has nginx + Let’s Encrypt, remo becomes just the “tunnel router” behind it.

Requirements
	•	remo server listens on an internal port, e.g. 127.0.0.1:18080 (HTTP) or 127.0.0.1:18443 (HTTPS optional).
	•	nginx terminates TLS for *.rempapps.site and proxies to remo.

Example nginx (conceptual)
	•	server_name  *.rempapps.site;
	•	location / { proxy_pass http://127.0.0.1:18080; }
	•	Forward headers:
	•	Host
	•	X-Forwarded-For
	•	X-Forwarded-Proto

Behavior in behind-nginx mode
	•	remo trusts X-Forwarded-* headers only from loopback / configured proxy CIDRs.
	•	remo still routes by Host → tunnel.

⸻

6) Routing model (MVP)

HTTP tunnels (must-have)
	•	Public request arrives with Host: foo.rempapps.site
	•	remo server finds active tunnel owner for foo
	•	Forwards request over tunnel to client upstream 127.0.0.1:<port>
	•	Returns response to public client

Headers
	•	Preserve Host
	•	Add:
	•	X-Forwarded-For, X-Forwarded-Proto
	•	X-Remo-Subdomain: foo

TCP tunnels (optional v1.1)
	•	Map a public port range on VPS to client ports
	•	Useful for SSH/dev DBs, but can wait.

⸻

7) Authentication & authorization (MVP)

Client identity
	•	remo auth init generates an ed25519 keypair
	•	Server stores an authorized_keys-style allowlist:
	•	pubkey → account
	•	optional allowed subdomains (e.g. gleicon-*)

Subdomain claiming rules
	•	First connected session to claim foo owns it until disconnect (or TTL expiry).
	•	Persistent reservations in SQLite (MVP+):
	•	pubkey can be granted a stable name (pgrok “stable subdomain” idea).  ￼

⸻

8) CLI UX (commands)

Server

remo server \
  --domain rempapps.site \
  --mode standalone \
  --listen :443 \
  --acme dns01 \
  --state /var/lib/remo/state.db

Server behind nginx

remo server \
  --domain rempapps.site \
  --mode behind-proxy \
  --listen 127.0.0.1:18080 \
  --trusted-proxies 127.0.0.1/32 \
  --state /var/lib/remo/state.db

Client

remo connect foo 3000
# exposes https://foo.rempapps.site → http://127.0.0.1:3000

Status

remo status


⸻

9) TUI (“top-like request log”) — MVP option

Decision
	•	Yes, include a TUI in MVP as an opt-in flag:
	•	remo connect foo 3000 --tui
	•	Rationale:
	•	It’s a differentiator that improves daily usability without requiring a web UI.
	•	Bubble Tea is a mature Go TUI framework; can render a “top-like” view and live logs.  ￼

TUI features (MVP)
	•	“Top bar”: tunnel state (connected, reconnecting, uptime, bytes in/out)
	•	“Requests table” (rolling window):
	•	time, method, path, status, latency, remote IP, bytes
	•	Filters:
	•	search by substring (/webhook, /api)
	•	toggle only errors (4xx/5xx)
	•	Keyboard:
	•	q quit, f filter, e errors-only, c clear, p pause

Non-goals for TUI (MVP)
	•	Full request/response body capture (privacy + complexity)
	•	Replay feature (later)

⸻

10) Observability (MVP)

Server metrics
	•	active tunnels
	•	req/s per subdomain
	•	latency p50/p95
	•	bytes in/out
	•	auth failures
	•	TLS/ACME status

Expose on an admin port:
	•	GET /healthz
	•	GET /status (JSON)
	•	optional /metrics

Client
	•	connection state + reconnect count
	•	upstream health check (optional --health /healthz)

⸻

11) Data model and storage

MVP can be in-memory + flat file, but recommended:
	•	SQLite on VPS for:
	•	authorized keys
	•	reserved subdomains
	•	audit log (connect/disconnect, claim attempts)

⸻

12) Implementation language: Go vs Zig

Go (recommended for v1)
	•	Networking/TLS, cross-compiling, and TUI ecosystem are all “boring and mature” (a feature here).
	•	Matches your “single binary” requirement and speeds MVP.

Zig (possible later)
	•	If you want Zig, the pragmatic path is:
	•	MVP in Go
	•	evaluate Zig rewrite of the dataplane after v1 proves the UX and protocol

⸻

13) MVP milestones (updated)

Milestone 0 — dev spike
	•	HTTP tunnel end-to-end (no TLS), simple connect → server routing

Milestone 1 — MVP (your requested scope)
	•	Standalone TLS (wildcard cert via DNS-01 or bring-your-own cert)
	•	Behind-nginx mode (--mode behind-proxy)
	•	Client auth + subdomain claim locking
	•	Reconnect/keepalive (holepunch-style resilience inspiration)  ￼
	•	Optional TUI (--tui) with top-like request log

Milestone 2 — power & polish
	•	TCP tunnels (port range)
	•	rate limiting / basic abuse guard
	•	reservations + multi-user structure

