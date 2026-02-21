# Remo Server Deployment & Debugging Report

**Date:** 2026-02-21  
**Server:** mgc-saas-apps-1 (169.150.1.130)  
**Domain:** cloud.remoapps.site

---

## Problem Identified

The remo systemd service was failing to start with exit code 1. The service showed as "failed" and continuously attempted to restart.

### Root Cause

Permission denied on `/etc/remo/server.yaml`. The remo user couldn't read the configuration file because:
- `/etc/remo/` directory had permissions `drwx------` (700) - only root could access
- `server.yaml` and `authorized.keys` were not readable by the remo user

---

## Files Modified/Fixed

### 1. `/etc/remo/` Directory Permissions
- **Before:** `drwx------ root root`
- **After:** `drwxr-x--- root remo` (750)
- **Reason:** Allows remo group to access config files

### 2. `/etc/remo/server.yaml` Permissions
- **Before:** `-rw------- root root` (600)
- **After:** `-rw-r----- root remo` (640)
- **Reason:** Readable by remo group for config parsing

### 3. `/etc/remo/authorized.keys` Permissions
- **Before:** `-rw------- root root` (600)
- **After:** `-rw-r--r-- remo remo` (644)
- **Reason:** Readable by all, writable by remo user

### 4. `/home/remo/.ssh/` Directory
- **Created:** `authorized_keys` file
- **Permissions:** Directory `700`, file `600`
- **Ownership:** `remo:remo`
- **Reason:** Standard SSH authentication

### 5. `/var/lib/remo/` Directory
- **Permissions:** `755`
- **Ownership:** `remo:remo`
- **Reason:** SQLite state database storage

---

## Current Status

### Service Status
```
● remo.service - Remo Server
     Loaded: loaded (/etc/systemd/system/remo.service; enabled; preset: enabled)
     Active: active (running) since Sat 2026-02-21 17:50:25 UTC
   Main PID: 1735182 (remo)
      Tasks: 6 (limit: 1109)
     Memory: 1.7M (peak: 2.0M)
        CPU: 18ms
```

### Listening Ports
```
LISTEN 0 4096 127.0.0.1:18080 0.0.0.0:* users:(("remo",pid=1735182,fd=3))
```

### Configuration
- **Domain:** cloud.remoapps.site
- **Mode:** behind-proxy (waits for nginx on port 18080)
- **Listen:** 127.0.0.1:18080
- **Authorized keys:** 3 keys configured
- **SSH authorized_keys:** 2 keys configured

---

## Key Insights for Future Deployments

### Permission Model
1. `/etc/remo/` must be readable by remo group (750)
2. Config files need group read permission (640)
3. Data directory `/var/lib/remo/` must be owned by remo user

### Authentication Flow
1. **SSH Connection** uses `/home/remo/.ssh/authorized_keys` (standard SSH format)
2. **Remo Authorization** uses `/etc/remo/authorized.keys` (base64 keys with subdomain rules)

### Two-Layer Security
- **Layer 1 (SSH):** Standard SSH key authentication for tunnel establishment
- **Layer 2 (Remo):** Base64 identity keys for subdomain routing and access control

---

## Debugging Commands

```bash
# Check service status
sudo systemctl status remo

# View logs
sudo journalctl -u remo -f

# Test as remo user
sudo -u remo /usr/local/bin/remo server --config /etc/remo/server.yaml --log debug

# Check permissions
ls -la /etc/remo/
ls -la /home/remo/.ssh/

# Verify listening
ss -tlnp | grep 18080
curl http://127.0.0.1:18080
```

---

## Client Connection Instructions

### Prerequisites
1. SSH key added to `/home/remo/.ssh/authorized_keys` on server
2. Remo identity key added to `/etc/remo/authorized.keys` on server

### Connection Command
```bash
./remo connect --server remo@169.150.1.130:22 --subdomain myapp --upstream http://127.0.0.1:3000 --tui
```

### Adding New Client Keys

**Add SSH key for tunnel access:**
```bash
# On server
echo 'ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI... user@laptop' | sudo tee -a /home/remo/.ssh/authorized_keys
```

**Add Remo identity for subdomain authorization:**
```bash
# On server
echo 'BASE64_PUBLIC_KEY *' | sudo tee -a /etc/remo/authorized.keys
```

---

## Client Connection Debugging

### Issue: Remo Client Exits with "context canceled"

**Status:** SSH layer works, remo client needs investigation

**Symptoms:**
- Manual SSH tunnel works: `ssh -i key -R 0:localhost:3000 remo@server` ✓
- Port allocation works: "Allocated port 33507 for remote forward" ✓
- Remo client exits immediately with "Error: context canceled"

**What Works:**
```bash
# SSH authentication and tunnel work perfectly
ssh -i /tmp/remo-test-key -N -R 0:localhost:3000 remo@169.150.1.130
# Output: "Allocated port 33507 for remote forward to localhost:3000"
```

**What Doesn't Work:**
```bash
./remo connect --server remo@169.150.1.130:22 --subdomain test --upstream http://127.0.0.1:3000
# Output: "Error: context canceled" (immediately)
```

**Investigation Notes:**
1. Server is running and listening on 127.0.0.1:18080 ✓
2. SSH keys are properly configured ✓
3. Manual SSH reverse tunnel works ✓
4. The issue appears to be in remo client's connection handling

**Potential Causes:**
- Context cancellation happening before SSH connection attempt
- TUI initialization issues (even without --tui flag)
- Signal handling conflicts
- Terminal/detection issues when running in background

**Next Steps for Debugging:**
1. Add print statements at entry points in client code
2. Check if context is canceled in main() before Execute()
3. Verify TUI isn't interfering with non-TUI mode
4. Test with simpler signal handling

## Architecture Summary

```
Internet → Nginx (443) → Remo Server (18080) → SSH Tunnel → Local Service
```

**Data Flow:**
1. Client runs `remo connect` with SSH key
2. SSH establishes reverse tunnel `ssh -R 0:localhost:18080`
3. Server assigns port and registers subdomain
4. Nginx routes `*.cloud.remoapps.site` to Remo
5. Remo routes by subdomain through SSH tunnel
6. Traffic reaches local service on client's machine

---

**Status:** ✅ Server is running and ready for client connections

**Last Updated:** 2026-02-21
