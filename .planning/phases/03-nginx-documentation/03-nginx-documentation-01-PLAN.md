---
phase: 03-nginx-documentation
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - docs/nginx.md
  - docs/ssh-setup.md
  - docs/nginx-example.conf
autonomous: true
requirements:
  - NGX-02
  - NGX-03
  - DOC-02
must_haves:
  truths:
    - User can find nginx config example for wildcard subdomains
    - User can follow Let's Encrypt setup instructions
    - User can set up SSH keys for client authentication
    - Nginx config handles WebSocket upgrades properly
  artifacts:
    - path: docs/nginx-example.conf
      provides: "Production nginx config with wildcard subdomain routing"
      min_lines: 80
      contains:
        - "server_name *."
        - "proxy_pass http://127.0.0.1:18080"
        - "proxy_set_header Upgrade"
        - "ssl_certificate"
    - path: docs/nginx.md
      provides: "Nginx + Let's Encrypt setup documentation"
      min_lines: 100
      contains:
        - "Let's Encrypt"
        - "certbot"
        - "wildcard certificate"
    - path: docs/ssh-setup.md
      provides: "SSH key generation and authorization guide"
      min_lines: 60
      contains:
        - "ssh-keygen"
        - "authorized_keys"
        - "Ed25519"
  key_links:
    - from: docs/nginx.md
      to: docs/nginx-example.conf
      via: "reference/example"
      pattern: "nginx-example.conf"
---

<objective>
Create comprehensive nginx configuration and SSH key setup documentation for production deployment.

Purpose: Provide users with complete, copy-paste-ready configuration files and step-by-step guides for setting up nginx with Let's Encrypt SSL and SSH key-based authentication.
Output: Three documentation files with production-ready nginx config, setup instructions, and SSH key management guide.
</objective>

<execution_context>
@/Users/gleicon/.config/opencode/get-shit-done/workflows/execute-plan.md
@/Users/gleicon/.config/opencode/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/ROADMAP.md
@.planning/STATE.md
@README.md
@internal/server/server.go
</context>

<tasks>

<task type="auto">
  <name>Task 1: Create nginx configuration example</name>
  <files>docs/nginx-example.conf</files>
  <action>
Create `docs/nginx-example.conf` with a complete production nginx configuration for Remo:

1. HTTP to HTTPS redirect server block (port 80)
2. Main HTTPS server block with:
   - Wildcard subdomain matching: `*.yourdomain.tld`
   - SSL configuration (cert paths as placeholders)
   - Proxy headers for proper forwarding:
     - Host, X-Real-IP, X-Forwarded-For, X-Forwarded-Proto
   - WebSocket support (Upgrade and Connection headers)
   - Reasonable timeouts (60s proxy_read_timeout)
   - Client max body size (100MB for uploads)

Include comments explaining:
- Replace `yourdomain.tld` with actual domain
- SSL certificate paths from Let's Encrypt
- Why each proxy header is needed
- WebSocket support for real-time features

Use 127.0.0.1:18080 as the upstream (Remo's default behind-proxy mode).
  </action>
  <verify>
    - File exists at docs/nginx-example.conf
    - Contains "server_name *." for wildcard matching
    - Contains "proxy_pass http://127.0.0.1:18080"
    - Contains WebSocket upgrade headers
    - Contains SSL certificate configuration
  </verify>
  <done>
    nginx-example.conf exists with complete wildcard subdomain configuration, SSL support, and WebSocket handling
  </done>
</task>

<task type="auto">
  <name>Task 2: Create nginx setup documentation</name>
  <files>docs/nginx.md</files>
  <action>
Create `docs/nginx.md` with comprehensive nginx and Let's Encrypt setup guide:

Structure:
1. Prerequisites (nginx installed, domain pointed to server)
2. Initial nginx installation (Ubuntu/Debian commands)
3. DNS configuration for wildcard certificate (*.yourdomain.tld)
4. Let's Encrypt setup with certbot:
   - Installing certbot
   - Obtaining wildcard certificate with DNS challenge
   - Automatic renewal setup
5. Remo-specific nginx configuration:
   - Copying nginx-example.conf
   - Customizing for your domain
   - Testing configuration
   - Reloading nginx
6. Troubleshooting section:
   - Checking nginx error logs
   - Verifying SSL certificate
   - Testing subdomain routing
   - Common issues (502 errors, SSL warnings)

Include actual command examples with placeholders like `<your-domain.tld>`.
Reference docs/nginx-example.conf as the configuration template.
  </action>
  <verify>
    - File exists at docs/nginx.md
    - Contains Let's Encrypt / certbot instructions
    - Contains DNS wildcard setup
    - Contains nginx configuration test commands
    - References nginx-example.conf
  </verify>
  <done>
    nginx.md exists with complete step-by-step guide for nginx + Let's Encrypt setup including DNS wildcard configuration
  </done>
</task>

<task type="auto">
  <name>Task 3: Create SSH key setup documentation</name>
  <files>docs/ssh-setup.md</files>
  <action>
Create `docs/ssh-setup.md` with SSH key generation and authorization guide:

Structure:
1. Overview (how Remo uses SSH keys for authentication)
2. Client key generation:
   - Using ssh-keygen to create Ed25519 key pair
   - Default location and file permissions
   - Viewing your public key
3. Server key authorization:
   - Location: /home/remo/.ssh/authorized_keys
   - Format: `<base64_public_key> <subdomain_rule>`
   - Subdomain rules explained:
     - `*` - any subdomain
     - `prefix-*` - subdomains starting with prefix
     - `exact-name` - specific subdomain only
4. Multiple clients setup:
   - Adding multiple keys
   - Different subdomain restrictions per key
5. Security best practices:
   - Key permissions (600 for private, 644 for authorized_keys)
   - Regular key rotation
   - Restricting subdomain access
6. Troubleshooting:
   - Permission denied errors
   - Key format issues
   - Verifying key is loaded

Include command examples for each step.
  </action>
  <verify>
    - File exists at docs/ssh-setup.md
    - Contains ssh-keygen instructions
    - Contains authorized_keys format explanation
    - Contains subdomain rule examples
    - Contains permission settings (chmod 600/644)
  </verify>
  <done>
    ssh-setup.md exists with complete SSH key generation, authorization, and subdomain restriction guide
  </done>
</task>

</tasks>

<verification>
After all tasks complete:
1. Verify docs/ directory exists with three files
2. Check nginx-example.conf has valid nginx syntax structure
3. Verify all markdown files have proper formatting
4. Confirm cross-references between docs are consistent
</verification>

<success_criteria>
- docs/nginx-example.conf: Production-ready nginx config with wildcard subdomain support, SSL, WebSocket handling
- docs/nginx.md: Complete Let's Encrypt setup guide with DNS wildcard instructions
- docs/ssh-setup.md: SSH key generation and authorization documentation
- All files are ready for user to copy-paste and follow
</success_criteria>

<output>
After completion, create `.planning/phases/03-nginx-documentation/03-nginx-documentation-01-SUMMARY.md`
</output>
