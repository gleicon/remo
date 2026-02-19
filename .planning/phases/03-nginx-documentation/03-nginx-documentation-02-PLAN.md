---
phase: 03-nginx-documentation
plan: 02
type: execute
wave: 2
depends_on:
  - 03-nginx-documentation-01
files_modified:
  - README.md
autonomous: true
requirements:
  - DOC-01
must_haves:
  truths:
    - User can understand Remo architecture from README
    - User can follow quick start guide to get running
    - User knows where to find nginx setup docs
    - User understands admin endpoint authentication
    - User can troubleshoot common issues
  artifacts:
    - path: README.md
      provides: "Complete project documentation with architecture, quick start, and references"
      min_lines: 200
      contains:
        - "Architecture"
        - "Quick Start"
        - "nginx"
        - "admin_secret"
        - "TUI"
  key_links:
    - from: README.md
      to: docs/nginx.md
      via: "documentation link"
      pattern: "docs/nginx.md"
    - from: README.md
      to: docs/ssh-setup.md
      via: "documentation link"
      pattern: "docs/ssh-setup.md"
---

<objective>
Rewrite README.md with comprehensive documentation including architecture diagram, quick start guide, and references to new nginx/SSH documentation.

Purpose: Provide users with a complete entry point to understand, install, and use Remo, with clear links to detailed setup documentation.
Output: Fully rewritten README.md with architecture explanation, improved quick start, and proper cross-references.
</objective>

<execution_context>
@/Users/gleicon/.config/opencode/get-shit-done/workflows/execute-plan.md
@/Users/gleicon/.config/opencode/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/ROADMAP.md
@.planning/STATE.md
@README.md (current version)
@docs/nginx.md (from Plan 01)
@docs/ssh-setup.md (from Plan 01)
@docs/nginx-example.conf (from Plan 01)
</context>

<tasks>

<task type="auto">
  <name>Task 1: Rewrite README with architecture and quick start</name>
  <files>README.md</files>
  <action>
Rewrite README.md with the following structure:

1. **Header** (keep existing title/tagline)
   - Title: Remo
   - Tagline: Self-hosted reverse tunnel with SSH and TUI dashboard
   - Badges: Go version, License

2. **What is Remo?** (NEW - architecture overview)
   - One paragraph explaining the tool
   - ASCII architecture diagram showing:
     - Client (laptop) -> SSH tunnel -> Server (VPS) -> Nginx -> Internet
     - Show data flow: HTTP request -> nginx -> Remo server -> SSH tunnel -> local service
   - Key features list:
     - Wildcard subdomain routing (*.yourdomain.tld)
     - SSH-based secure tunnels (no custom protocol)
     - Real-time TUI dashboard with request logging
     - Simple binary, no dependencies

3. **Quick Start** (ENHANCE existing)
   - Prerequisites: Go 1.21+, SSH client, domain with DNS
   - Step 1: Install client (keep existing curl command)
   - Step 2: Set up server (reference docs/nginx.md for production)
   - Step 3: Connect and expose service
   - Include example commands with expected output

4. **How It Works** (ENHANCE existing)
   - Keep the 5-step explanation
   - Add note about TUI showing real-time requests
   - Mention session statistics and log export

5. **Production Setup with Nginx** (NEW section)
   - Brief explanation why nginx is recommended
   - Link to docs/nginx.md for full setup
   - Link to docs/nginx-example.conf for configuration
   - Quick certbot command example

6. **SSH Key Authentication** (NEW section)
   - Brief explanation of Ed25519 key pairs
   - Link to docs/ssh-setup.md for detailed guide
   - Quick example of adding a key

7. **TUI Dashboard** (NEW section)
   - Screenshot description (ASCII or placeholder)
   - Key bindings: 'q' quit, 'c' clear, 'e' error filter, 'p' pause
   - Statistics display explanation
   - JSON log export on quit

8. **Configuration** (ENHANCE existing)
   - Server config with all options explained
   - Client flags reference table
   - Admin endpoints with authentication note

9. **Admin Endpoints** (ENHANCE existing)
   - Keep existing endpoint table
   - Add authentication section explaining admin_secret
   - Show example curl commands with Bearer token
   - Document that /events is localhost-only (security feature)

10. **File Locations** (keep existing table)

11. **Troubleshooting** (NEW section)
    - Connection refused: Check if server is running
    - 502 Bad Gateway: Check nginx upstream
    - Permission denied: Check SSH key authorization
    - Subdomain not found: Check registration
    - TUI not showing logs: Check --tui flag

12. **Development** (keep existing)

13. **Documentation** (NEW - links section)
    - docs/nginx.md - Production nginx setup
    - docs/ssh-setup.md - SSH key management
    - docs/nginx-example.conf - Example nginx configuration

Maintain clear, concise language. Use code blocks for all commands and configurations. Keep the existing working examples but enhance context.
  </action>
  <verify>
    - README.md rewritten with all sections
    - Contains ASCII architecture diagram
    - Contains TUI section with key bindings
    - Links to docs/nginx.md and docs/ssh-setup.md
    - Contains admin authentication documentation
    - All existing accurate information preserved
  </verify>
  <done>
    README.md is a comprehensive, well-structured document that serves as the primary entry point for Remo users, with clear architecture explanation, quick start, and links to detailed setup guides
  </done>
</task>

</tasks>

<verification>
After task completion:
1. Verify README.md has all required sections
2. Check that all links to docs/ files are correct
3. Ensure architecture diagram renders properly in markdown
4. Verify admin authentication is clearly documented
5. Confirm TUI features are accurately described
</verification>

<success_criteria>
- README.md has comprehensive architecture overview with ASCII diagram
- Quick start guide is clear and actionable
- TUI dashboard features are documented (key bindings, statistics, export)
- Admin endpoint authentication is explained with examples
- Cross-references to docs/nginx.md and docs/ssh-setup.md exist
- Troubleshooting section covers common issues
</success_criteria>

<output>
After completion, create `.planning/phases/03-nginx-documentation/03-nginx-documentation-02-SUMMARY.md`
</output>
