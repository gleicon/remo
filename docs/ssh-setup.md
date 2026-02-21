# SSH Key Setup

Remo uses SSH public keys for authentication. The installer handles key generation automatically.

## Automatic Setup (Recommended)

The installer creates keys automatically:

**Server:** Asks for client public keys during setup
**Client:** Generates identity at `~/.remo/identity.json`

## Manual Key Management

### View Your Public Key

```bash
remo auth inspect -f ~/.remo/identity.json
```

### Add Keys to Server

```bash
# On server
echo 'CLIENT_PUBLIC_KEY *' | sudo tee -a /etc/remo/authorized.keys
```

### Subdomain Rules

Format: `public-key subdomain-rule`

- `*` - Any subdomain
- `prefix-*` - Subdomains starting with prefix
- `exact-name` - Specific subdomain only

## Files

- Server: `/etc/remo/authorized.keys`
- Client: `~/.remo/identity.json`
- Config: `~/.remo/config`
