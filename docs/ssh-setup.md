# SSH Key Setup and Authentication

Remo uses standard SSH public key authentication for secure, passwordless tunnel connections. This document covers both automatic and manual key management.

---

## Automatic Setup (Recommended)

The installer handles everything automatically:

### Server Setup
```bash
sudo curl -sL https://raw.githubusercontent.com/gleicon/remo/main/install.sh | bash
```

When prompted:
```
Add SSH public keys for client authentication
Format: ssh-ed25519 AAAAC3... user@host [subdomain-rule]
Press Enter without input when done

SSH public key (or Enter to finish): ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI... your@email.com *
[INFO]  Added SSH key with rule: *

SSH public key (or Enter to finish): 
[STEP] Client keys configured
```

### Client Setup
```bash
curl -sL https://raw.githubusercontent.com/gleicon/remo/main/install.sh | bash
```

The installer will:
1. Generate Ed25519 identity key in `~/.remo/identity.json`
2. Display your public key for server registration
3. Ask for server domain
4. Optionally test the connection

---

## Manual SSH Key Setup

### Generate SSH Key Pair

**Client Side:**
```bash
# Generate Ed25519 key (recommended)
ssh-keygen -t ed25519 -C "remo-$(whoami)" -f ~/.ssh/id_ed25519

# Or use existing SSH key
ls ~/.ssh/id_*
```

**View Public Key:**
```bash
# SSH format
cat ~/.ssh/id_ed25519.pub
# Output: ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI... your@email.com

# Or Remo format (base64)
remo auth inspect -f ~/.remo/identity.json
```

### Add to Server

**Option 1: SSH authorized_keys (for tunnel access)**
```bash
# On server as remo user
echo 'ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI... your@email.com' >> ~/.ssh/authorized_keys
chmod 600 ~/.ssh/authorized_keys
```

**Option 2: Remo authorized.keys (for subdomain authorization)**
```bash
# On server as root
echo 'BASE64_PUBLIC_KEY subdomain-rule' | sudo tee -a /etc/remo/authorized.keys

# Example: allow any subdomain
echo 'Nc8rhlSGkS1qATcyIOkytjuMhpdZoswbrLKwoCWKQEU= *' | sudo tee -a /etc/remo/authorized.keys

# Example: allow only specific subdomain
echo 'Nc8rhlSGkS1qATcyIOkytjuMhpdZoswbrLKwoCWKQEU= myapp' | sudo tee -a /etc/remo/authorized.keys

# Example: allow pattern
echo 'Nc8rhlSGkS1qATcyIOkytjuMhpdZoswbrLKwoCWKQEU= dev-*' | sudo tee -a /etc/remo/authorized.keys
```

---

## Subdomain Authorization Rules

Remo uses **two-layer authentication**:

### Layer 1: SSH Authentication
Standard SSH key in `~/.remo/.ssh/authorized_keys` — required to establish the tunnel.

### Layer 2: Remo Authorization
Base64 public keys in `/etc/remo/authorized.keys` — controls which subdomains a client can claim.

**Format:** `base64-public-key subdomain-rule`

**Rules:**
| Rule | Description | Example |
|------|-------------|---------|
| `*` | Any subdomain | Client can claim `foo`, `bar`, `anything` |
| `prefix-*` | Pattern match | Client can claim `dev-1`, `dev-test`, `dev-2024` |
| `exact-name` | Specific only | Client can only claim `staging` |

**Examples:**
```
# Developer team - can create any dev subdomain
ssh-ed25519 AAAAC3... dev@company.com dev-*

# Staging server - fixed subdomain
ssh-ed25519 AAAAC3... ops@company.com staging

# QA team - multiple patterns
ssh-ed25519 AAAAC3... qa1@company.com qa-*
ssh-ed25519 AAAAC3... qa2@company.com qa-*
```

---

## Key Conversion

**SSH format (ssh-ed25519 ...) to Remo format (base64):**

```bash
# Extract base64 public key from SSH key
ssh-keygen -f ~/.ssh/id_ed25519.pub -e -m PKCS8 | grep -v '^-' | base64 -d | base64

# Or use remo to generate and show
remo auth init -out ~/.remo/identity.json
remo auth inspect -f ~/.remo/identity.json
```

**Example output:**
```
Public key: Nc8rhlSGkS1qATcyIOkytjuMhpdZoswbrLKwoCWKQEU=
```

---

## Troubleshooting

### Permission Denied (SSH)
```bash
# Check permissions on server
ls -la /home/remo/.ssh/
# Should be: drwx------ .ssh
# Should be: -rw------- authorized_keys

# Fix permissions
chmod 700 /home/remo/.ssh
chmod 600 /home/remo/.ssh/authorized_keys
chown -R remo:remo /home/remo/.ssh
```

### Unauthorized (Remo)
```bash
# Check authorized.keys format
cat /etc/remo/authorized.keys
# Should be: BASE64_KEY subdomain-rule

# Not: ssh-ed25519 AAAAC3... (that's SSH format, not Remo format)

# Convert SSH to Remo format:
PUBKEY=$(cat ~/.ssh/id_ed25519.pub | awk '{print $2}' | base64 -d | base64)
echo "$PUBKEY *" | sudo tee -a /etc/remo/authorized.keys
```

### Test SSH Connection
```bash
# From client
ssh -v -i ~/.ssh/id_ed25519 remo@your-server "echo 'SSH OK'"

# Should show: Authenticated to your-server using "publickey"
```

---

## Security Best Practices

1. **Use Ed25519 keys** — Modern, secure, smaller than RSA
2. **Separate keys per client** — Don't share keys between laptops
3. **Restrict subdomain rules** — Use specific patterns, not `*` when possible
4. **Rotate keys regularly** — Generate new keys every 6-12 months
5. **Monitor authorized.keys** — Check for unexpected entries

---

## Files Reference

| File | Location | Purpose |
|------|----------|---------|
| SSH private key | `~/.ssh/id_ed25519` | Client SSH authentication |
| SSH public key | `~/.ssh/id_ed25519.pub` | For server authorized_keys |
| Remo identity | `~/.remo/identity.json` | Client identity (SSH key + metadata) |
| SSH authorized | `/home/remo/.ssh/authorized_keys` | Server SSH authentication |
| Remo authorized | `/etc/remo/authorized.keys` | Server subdomain authorization |

---

## Advanced: Multiple Keys

**Client with multiple identities:**
```bash
# Work laptop
remo connect --server work.example.com --identity ~/.remo/work.json --subdomain api

# Personal laptop  
remo connect --server personal.example.com --identity ~/.remo/personal.json --subdomain blog
```

**Server with multiple authorized keys:**
```bash
# /etc/remo/authorized.keys
AAAA...workkey...== api
AAAA...workkey...== staging
AAAA...personalkey...== blog
AAAA...personalkey...== portfolio
```
