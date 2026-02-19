# SSH Key Setup Guide

This guide explains how to set up SSH key authentication for Remo clients. SSH keys provide secure, passwordless authentication for tunnel connections.

## Overview

Remo uses SSH public key authentication to:
- Verify client identity without passwords
- Control which subdomains each client can access
- Enable multiple clients with different access levels

Each client generates an SSH key pair and provides the public key to the server administrator, who adds it to the authorized keys file with subdomain restrictions.

## Table of Contents

1. [Client Key Generation](#1-client-key-generation)
2. [Server Key Authorization](#2-server-key-authorization)
3. [Multiple Clients Setup](#3-multiple-clients-setup)
4. [Security Best Practices](#4-security-best-practices)
5. [Troubleshooting](#5-troubleshooting)

---

## 1. Client Key Generation

### Generate Ed25519 Key Pair

We recommend using the Ed25519 algorithm for better security and performance:

```bash
# Generate Ed25519 key pair
ssh-keygen -t ed25519 -C "remo-client-$(whoami)@$(hostname)" -f ~/.ssh/remo_ed25519
```

When prompted:
- **Enter passphrase** (optional but recommended): Adds extra security
- **Confirm passphrase**: Re-enter if you chose one

### Default Key Location

The command creates two files:

| File | Permissions | Description |
|------|-------------|-------------|
| `~/.ssh/remo_ed25519` | 600 (rw-------) | Private key - keep secret! |
| `~/.ssh/remo_ed25519.pub` | 644 (rw-r--r--) | Public key - share with server admin |

### View Your Public Key

```bash
# Display public key
cat ~/.ssh/remo_ed25519.pub
```

Output format:
```
ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIDIhz2GK/XCUj4i6Q5yQJNL1MXMY0RxzPV2QrBqfHrDq remo-client-alice@laptop
```

Copy this entire line to provide to your server administrator.

### Alternative: RSA Key (Legacy Systems)

If your system doesn't support Ed25519, use RSA with 4096 bits:

```bash
ssh-keygen -t rsa -b 4096 -C "remo-client-$(whoami)@$(hostname)" -f ~/.ssh/remo_rsa
```

---

## 2. Server Key Authorization

### Authorized Keys Location

On the Remo server, authorized keys are stored at:

```
/home/remo/.ssh/authorized_keys
```

If the file doesn't exist, create it with proper permissions.

### Key Format with Subdomain Rules

Each line in `authorized_keys` follows this format:

```
<public_key> <subdomain_rule>
```

### Subdomain Rules

Subdomain rules control which subdomains a client can claim:

| Rule | Description | Example Subdomains |
|------|-------------|-------------------|
| `*` | Any subdomain | `app`, `api`, `anything` |
| `prefix-*` | Subdomains starting with prefix | `dev-api`, `dev-app` |
| `exact-name` | Specific subdomain only | Only `staging` |

### Examples

```
# Alice can claim any subdomain
ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIDIhz2GK/XCUj4i6Q5yQJNL1MXMY0RxzPV2QrBqfHrDq alice@laptop *

# Bob can only use subdomains starting with "dev-"
ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGTzvz9z3qQfhZ3qZqZqZqZqZqZqZqZqZqZqZqZqZq bob@desktop dev-*

# Carol can only claim the "staging" subdomain
ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIDKj7n3h4k5l6m7n8o9p0q1r2s3t4u5v6w7x8y9z0 carol@server staging
```

### Adding a New Key

As the server administrator:

```bash
# Switch to remo user
sudo su - remo

# Create .ssh directory if needed
mkdir -p ~/.ssh
chmod 700 ~/.ssh

# Add the public key to authorized_keys
echo "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIDIhz2GK/XCUj4i6Q5yQJNL1MXMY0RxzPV2QrBqfHrDq alice@laptop *" >> ~/.ssh/authorized_keys

# Set proper permissions
chmod 600 ~/.ssh/authorized_keys

# Verify the file
cat ~/.ssh/authorized_keys
```

### Setting Permissions

Proper permissions are critical for SSH to work:

```bash
# As remo user
chmod 700 ~/.ssh
chmod 600 ~/.ssh/authorized_keys

# Verify
ls -la ~/.ssh/
```

Expected output:
```
drwx------ 2 remo remo 4096 Feb 19 10:00 .
drwxr-xr-x 4 remo remo 4096 Feb 19 09:00 ..
-rw------- 1 remo remo  234 Feb 19 10:00 authorized_keys
```

---

## 3. Multiple Clients Setup

### Scenario: Development Team

You have multiple developers who need different access levels:

```bash
# /home/remo/.ssh/authorized_keys

# Lead developer - full access
ssh-ed25519 AAAAC3NzaC1... alice@company-laptop *

# Backend developer - dev subdomains only
ssh-ed25519 AAAAC3NzaC1... bob@home-desktop dev-*

# Frontend developer - specific project subdomains
ssh-ed25519 AAAAC3NzaC1... carol@macbook project1-*
ssh-ed25519 AAAAC3NzaC1... carol@macbook project2-*

# CI/CD server - specific deployment subdomain
ssh-ed25519 AAAAC3NzaC1... cicd@build-server staging
```

### Scenario: Client Projects

Different clients get isolated subdomains:

```bash
# Client A - any subdomain under client-a
ssh-ed25519 AAAAC3NzaC1... client-a@agency client-a-*

# Client B - any subdomain under client-b
ssh-ed25519 AAAAC3NzaC1... client-b@agency client-b-*

# Internal tools - specific subdomains
ssh-ed25519 AAAAC3NzaC1... admin@company admin
ssh-ed25519 AAAAC3NzaC1... admin@company dashboard
```

### Multiple Keys per User

A single user can have multiple keys for different devices:

```bash
# Alice's laptop
ssh-ed25519 AAAAC3NzaC1... alice@laptop *

# Alice's desktop
ssh-ed25519 AAAAC3NzaC1... alice@desktop *

# Alice's phone (restricted to specific subdomains)
ssh-ed25519 AAAAC3NzaC1... alice@phone alice-mobile
```

---

## 4. Security Best Practices

### Key Permissions

Always use strict permissions:

```bash
# Client machine
chmod 600 ~/.ssh/remo_ed25519          # Private key
chmod 644 ~/.ssh/remo_ed25519.pub      # Public key
chmod 700 ~/.ssh/                       # SSH directory

# Server (as remo user)
chmod 700 ~/.ssh/
chmod 600 ~/.ssh/authorized_keys
```

### Passphrase Protection

Always use a passphrase for client keys:

```bash
# Generate with passphrase (recommended)
ssh-keygen -t ed25519 -f ~/.ssh/remo_ed25519

# Add passphrase to existing key
ssh-keygen -p -f ~/.ssh/remo_ed25519
```

Use `ssh-agent` to avoid typing the passphrase repeatedly:

```bash
# Start ssh-agent
eval "$(ssh-agent -s)"

# Add key to agent
ssh-add ~/.ssh/remo_ed25519

# Enter passphrase once, then it stays cached
```

### Regular Key Rotation

Rotate keys periodically (every 6-12 months):

1. Generate new key pair on client
2. Add new public key to server
3. Test connection with new key
4. Remove old key from server
5. Delete old key files from client

### Restrict Subdomain Access

Follow the principle of least privilege:

- Give users only the subdomains they need
- Use specific rules (`project1-*`) over wildcards (`*`)
- Separate production and development access
- Use different keys for different environments

### Audit Authorized Keys

Regularly review authorized keys:

```bash
# List all authorized keys
sudo su - remo -c "cat ~/.ssh/authorized_keys"

# Check for unused keys (compare with active users)
# Remove keys for users who no longer need access
```

### Backup Considerations

- **Client**: Backup private keys securely (password manager, encrypted storage)
- **Server**: Backup `authorized_keys` file as part of server backups
- Never share private keys between users

---

## 5. Troubleshooting

### Permission Denied Errors

#### Client Side

```bash
# Check key file permissions
ls -la ~/.ssh/remo_ed25519
# Should show: -rw------- (600)

# Fix permissions
chmod 600 ~/.ssh/remo_ed25519

# Verify key is loaded in agent
ssh-add -l

# Add key if not present
ssh-add ~/.ssh/remo_ed25519
```

#### Server Side

```bash
# Check authorized_keys permissions (as remo user)
ls -la ~/.ssh/
# Should show: drwx------ (700) for .ssh
# Should show: -rw------- (600) for authorized_keys

# Fix permissions
chmod 700 ~/.ssh
chmod 600 ~/.ssh/authorized_keys

# Check SSH logs for errors
sudo tail -f /var/log/auth.log
```

### Key Format Issues

#### Invalid Key Format

```bash
# Verify key format
cat ~/.ssh/remo_ed25519.pub

# Should start with:
# ssh-ed25519 AAAAC3NzaC1...

# If the key doesn't start with ssh-ed25519 or ssh-rsa, regenerate it
ssh-keygen -t ed25519 -f ~/.ssh/remo_ed25519 -C "comment"
```

#### Missing Subdomain Rule

If the key works but subdomain access is denied, check the authorized_keys format:

```bash
# Correct format (note the space before subdomain rule)
ssh-ed25519 AAAAC3NzaC1... comment subdomain-rule

# Incorrect format (missing subdomain)
ssh-ed25519 AAAAC3NzaC1... comment
```

### Verifying Key is Loaded

On the client:

```bash
# List loaded keys
ssh-add -l

# Expected output:
# 256 SHA256:abcdef123456... alice@laptop (ED25519)

# If empty, add the key
ssh-add ~/.ssh/remo_ed25519
```

### Testing SSH Connection

Test the SSH connection directly:

```bash
# Test SSH to remo server
ssh -i ~/.ssh/remo_ed25519 -p 2222 remo@your-server.com

# Should connect and show Remo banner
# If successful but Remo client fails, check client configuration
```

### Common Error Messages

#### "No supported authentication methods available"

- Key not in authorized_keys
- Wrong key file specified
- Key permissions too open

#### "Permission denied (publickey)"

- Key not added to authorized_keys
- Wrong key being used
- Subdomain rule missing from authorized_keys line

#### "Could not open a connection to your authentication agent"

```bash
# Start ssh-agent
eval "$(ssh-agent -s)"

# Or use the full path
exec ssh-agent bash
ssh-add ~/.ssh/remo_ed25519
```

### Debug Mode

Enable verbose output for troubleshooting:

```bash
# Verbose SSH connection test
ssh -vvv -i ~/.ssh/remo_ed25519 -p 2222 remo@your-server.com

# This shows detailed authentication steps
```

---

## Quick Reference

### Client Commands

```bash
# Generate key
ssh-keygen -t ed25519 -f ~/.ssh/remo_ed25519 -C "comment"

# View public key
cat ~/.ssh/remo_ed25519.pub

# Add to agent
ssh-add ~/.ssh/remo_ed25519

# List loaded keys
ssh-add -l
```

### Server Commands

```bash
# Add key to authorized_keys
echo "<public-key> <subdomain-rule>" >> /home/remo/.ssh/authorized_keys

# Set permissions
chmod 700 /home/remo/.ssh
chmod 600 /home/remo/.ssh/authorized_keys
```

### File Permissions Summary

| File | Client | Server |
|------|--------|--------|
| Private key | 600 | - |
| Public key | 644 | - |
| .ssh directory | 700 | 700 |
| authorized_keys | - | 600 |

---

## Next Steps

After setting up SSH keys:

1. [Configure nginx with SSL](./nginx.md)
2. Start Remo server
3. Connect using Remo client with your SSH key

## Related Documentation

- [Nginx Setup Guide](./nginx.md) - Web server and SSL configuration
- [nginx-example.conf](./nginx-example.conf) - Production nginx configuration
