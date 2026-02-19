# Nginx and Let's Encrypt Setup Guide

This guide walks you through setting up nginx with Let's Encrypt SSL certificates for Remo, including wildcard subdomain support.

## Prerequisites

Before starting, ensure you have:

- A server with a public IP address (VPS, cloud instance, etc.)
- A registered domain name (e.g., `example.com`)
- DNS access to configure records for your domain
- Root or sudo access to the server
- Remo server installed and running on port 18080

## Table of Contents

1. [Install Nginx](#1-install-nginx)
2. [Configure DNS](#2-configure-dns)
3. [Obtain SSL Certificate](#3-obtain-ssl-certificate)
4. [Configure Nginx for Remo](#4-configure-nginx-for-remo)
5. [Troubleshooting](#5-troubleshooting)

---

## 1. Install Nginx

### Ubuntu/Debian

```bash
# Update package lists
sudo apt update

# Install nginx
sudo apt install nginx -y

# Start nginx and enable on boot
sudo systemctl start nginx
sudo systemctl enable nginx

# Verify nginx is running
sudo systemctl status nginx
```

### CentOS/RHEL/Rocky Linux

```bash
# Install nginx
sudo dnf install nginx -y

# Start nginx and enable on boot
sudo systemctl start nginx
sudo systemctl enable nginx

# Verify nginx is running
sudo systemctl status nginx
```

### Verify Installation

Open your browser and navigate to your server's IP address. You should see the nginx welcome page.

---

## 2. Configure DNS

To use wildcard SSL certificates and route all subdomains to Remo, configure the following DNS records:

### Required DNS Records

| Type | Name | Value | TTL |
|------|------|-------|-----|
| A | `@` | `<your-server-ip>` | 300 |
| A | `*` | `<your-server-ip>` | 300 |

### Example for domain `example.com`

```
A     example.com     → 203.0.113.10
A     *.example.com   → 203.0.113.10
```

The wildcard record (`*.example.com`) ensures all subdomains (e.g., `app.example.com`, `api.example.com`, `anything.example.com`) resolve to your server.

### DNS Propagation

DNS changes can take time to propagate. Verify with:

```bash
# Check root domain
dig +short example.com

# Check wildcard resolution
dig +short test.example.com
```

---

## 3. Obtain SSL Certificate

We'll use Let's Encrypt with certbot to obtain a wildcard certificate.

### Install Certbot

#### Ubuntu/Debian

```bash
sudo apt install certbot python3-certbot-nginx -y
```

#### CentOS/RHEL/Rocky Linux

```bash
sudo dnf install certbot python3-certbot-nginx -y
```

### Obtain Wildcard Certificate

Wildcard certificates require DNS validation. You'll need to add a TXT record to prove domain ownership.

```bash
# Request wildcard certificate
sudo certbot certonly --manual --preferred-challenges dns \
  -d "*.yourdomain.tld" -d "yourdomain.tld"
```

Replace `yourdomain.tld` with your actual domain.

### DNS Challenge Process

1. Certbot will display a DNS TXT record to add:
   ```
   Please deploy a DNS TXT record under the name:
   _acme-challenge.yourdomain.tld
   
   with the following value:
   xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
   ```

2. Add the TXT record in your DNS provider's dashboard:
   - Type: TXT
   - Name: `_acme-challenge`
   - Value: (the string certbot provided)
   - TTL: 300

3. Wait 30-60 seconds for DNS propagation, then press Enter in certbot

4. Certbot will verify and issue your certificate

### Certificate Location

Your certificates will be stored at:

```
/etc/letsencrypt/live/yourdomain.tld/
├── fullchain.pem    # Certificate + intermediate CA
├── privkey.pem      # Private key
├── cert.pem         # Certificate only
└── chain.pem        # Intermediate CA only
```

### Automatic Renewal

Let's Encrypt certificates expire every 90 days. Enable automatic renewal:

```bash
# Test renewal process
sudo certbot renew --dry-run

# Enable automatic renewal (usually already enabled via systemd timer)
sudo systemctl enable certbot-renew.timer
sudo systemctl start certbot-renew.timer

# Check renewal timer status
sudo systemctl status certbot-renew.timer
```

---

## 4. Configure Nginx for Remo

### Copy the Example Configuration

```bash
# Copy the example config from Remo docs
sudo cp /path/to/remo/docs/nginx-example.conf /etc/nginx/sites-available/remo

# Edit the configuration to use your domain
sudo nano /etc/nginx/sites-available/remo
```

### Customize for Your Domain

Replace all occurrences of `yourdomain.tld` with your actual domain:

```bash
# Replace domain in the config file
sudo sed -i 's/yourdomain\.tld/example.com/g' /etc/nginx/sites-available/remo
```

### Enable the Site

```bash
# Create symlink to enable the site
sudo ln -s /etc/nginx/sites-available/remo /etc/nginx/sites-enabled/

# Remove default site (optional)
sudo rm /etc/nginx/sites-enabled/default
```

### Test Configuration

Always test nginx configuration before reloading:

```bash
# Test configuration syntax
sudo nginx -t
```

Expected output:
```
nginx: the configuration file /etc/nginx/nginx.conf syntax is ok
nginx: configuration file /etc/nginx/nginx.conf test is successful
```

### Reload Nginx

```bash
# Reload nginx to apply changes
sudo systemctl reload nginx
```

### Verify SSL

Open your browser and navigate to:
- `https://yourdomain.tld` - Should show Remo landing page
- `https://test.yourdomain.tld` - Should route to Remo (may show "no tunnel" if no client connected)

Check SSL certificate:
```bash
curl -vI https://yourdomain.tld 2>&1 | grep -E "(subject|issuer|SSL)"
```

---

## 5. Troubleshooting

### Check Nginx Error Logs

```bash
# View error logs in real-time
sudo tail -f /var/log/nginx/remo-error.log

# View last 50 lines
sudo tail -n 50 /var/log/nginx/remo-error.log
```

### Verify SSL Certificate

```bash
# Check certificate details
echo | openssl s_client -servername yourdomain.tld -connect yourdomain.tld:443 2>/dev/null | openssl x509 -noout -dates -subject -issuer

# Check certificate expiration
certbot certificates
```

### Test Subdomain Routing

```bash
# Test DNS resolution
dig +short test.yourdomain.tld

# Test HTTP response
curl -I https://test.yourdomain.tld
```

### Common Issues

#### 502 Bad Gateway

This means nginx can't connect to Remo:

```bash
# Check if Remo is running on port 18080
sudo ss -tlnp | grep 18080

# Check Remo logs
sudo journalctl -u remo -f

# Verify proxy_pass in nginx config
grep proxy_pass /etc/nginx/sites-available/remo
```

#### SSL Certificate Warnings

If browsers show certificate warnings:

```bash
# Check certificate validity
certbot certificates

# Force renewal if needed
sudo certbot renew --force-renewal
```

#### Permission Denied Errors

```bash
# Check nginx can read certificate files
sudo ls -la /etc/letsencrypt/live/yourdomain.tld/

# Check nginx user
ps aux | grep nginx

# Verify file permissions
sudo chmod 644 /etc/letsencrypt/live/yourdomain.tld/fullchain.pem
sudo chmod 600 /etc/letsencrypt/live/yourdomain.tld/privkey.pem
```

#### Subdomain Not Routing

```bash
# Verify wildcard DNS
dig +short anything.yourdomain.tld

# Check nginx server_name directive
grep server_name /etc/nginx/sites-available/remo

# Test nginx configuration
sudo nginx -t
```

### Getting Help

If issues persist:

1. Check Remo logs: `sudo journalctl -u remo -n 100`
2. Check nginx logs: `sudo tail -f /var/log/nginx/error.log`
3. Verify Remo is listening: `curl http://127.0.0.1:18080`
4. Test without nginx: `curl -H "Host: test.yourdomain.tld" http://127.0.0.1:18080`

---

## Next Steps

After nginx is configured:

1. [Set up SSH keys for client authentication](./ssh-setup.md)
2. Start Remo server if not already running
3. Connect clients using the Remo CLI

## Reference

- [nginx-example.conf](./nginx-example.conf) - Complete nginx configuration template
- [SSH Setup Guide](./ssh-setup.md) - Client authentication setup
