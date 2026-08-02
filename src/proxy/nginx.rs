use anyhow::{Context, Result};
use std::path::PathBuf;
use std::process::Command;

use super::ProxyBackend;

pub struct NginxBackend {
    domain: String,
}

impl NginxBackend {
    pub fn new(domain: String) -> Self {
        Self { domain }
    }

    fn vhost_path(&self, hostname: &str) -> PathBuf {
        PathBuf::from("/etc/nginx/sites-enabled").join(format!("{hostname}.conf"))
    }

    fn write_vhost(&self, hostname: &str) -> Result<()> {
        let path = self.vhost_path(hostname);
        // Wildcard A record already covers *.domain, so no DNS entry needed.
        // nginx does SNI pass-through until certbot upgrades this to HTTPS.
        let cfg = format!(
            r#"server {{
    listen 80;
    server_name {hostname};

    location / {{
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }}
}}
"#,
        );
        std::fs::write(&path, cfg)
            .with_context(|| format!("write nginx vhost {}", path.display()))
    }
}

impl ProxyBackend for NginxBackend {
    fn provision_cert(&self, hostname: &str) -> Result<()> {
        self.write_vhost(hostname)?;
        self.reload()?;

        let status = Command::new("certbot")
            .args(["--nginx", "-d", hostname, "--non-interactive", "--agree-tos", "-m", &format!("admin@{}", self.domain)])
            .status()
            .context("certbot not found — install with: apt install certbot python3-certbot-nginx")?;

        if !status.success() {
            anyhow::bail!("certbot failed for {hostname}");
        }
        Ok(())
    }

    fn remove_cert(&self, hostname: &str) -> Result<()> {
        let path = self.vhost_path(hostname);
        if path.exists() {
            std::fs::remove_file(&path)?;
        }
        // Best-effort cert revocation; not fatal if certbot is not installed.
        let _ = Command::new("certbot")
            .args(["delete", "--cert-name", hostname, "--non-interactive"])
            .status();
        self.reload()
    }

    fn reload(&self) -> Result<()> {
        let status = Command::new("nginx")
            .arg("-t")
            .status()
            .context("nginx not found")?;
        if !status.success() {
            anyhow::bail!("nginx config test failed — fix /etc/nginx before reloading");
        }
        Command::new("systemctl")
            .args(["reload", "nginx"])
            .status()
            .context("systemctl reload nginx failed")?;
        Ok(())
    }
}
