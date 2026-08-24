use anyhow::{Context, Result};
use std::process::Command;

use super::ProxyBackend;

/// Caddy backend. On-demand TLS means no per-app action is needed — Caddy
/// fetches a cert on first HTTPS hit for any subdomain covered by the wildcard
/// A record. The only config needed is written once by `remo server install`.
pub struct CaddyBackend;

impl CaddyBackend {
    pub fn new(_domain: String) -> Self {
        Self
    }

    /// Returns the static Caddyfile snippet written by `remo server install`.
    /// With on-demand TLS this never changes — new apps need no config update.
    pub fn static_caddyfile(domain: &str, nano_port: u16) -> String {
        format!(
            r#"*.{domain} {{
    tls {{
        on_demand
    }}
    reverse_proxy localhost:{nano_port}
}}
"#,
        )
    }
}

impl ProxyBackend for CaddyBackend {
    fn provision_cert(&self, _hostname: &str) -> Result<()> {
        // On-demand TLS: Caddy fetches the cert itself on first HTTPS request.
        // Nothing to do per-app.
        Ok(())
    }

    fn remove_cert(&self, _hostname: &str) -> Result<()> {
        // Cert expires naturally; no per-app revocation needed for dev/staging.
        Ok(())
    }

    fn reload(&self) -> Result<()> {
        Command::new("systemctl")
            .args(["reload", "caddy"])
            .status()
            .context("systemctl reload caddy failed")?;
        Ok(())
    }
}
