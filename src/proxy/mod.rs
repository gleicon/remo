pub mod caddy;
pub mod nginx;

use anyhow::Result;
use std::sync::Arc;

pub use caddy::CaddyBackend;
pub use nginx::NginxBackend;

/// Proxy backend: manages TLS and vhost config for a reverse proxy.
/// nano-rs does all app routing by Host header — the proxy's only job is TLS termination.
/// Canonical subdomains (`{owner}-{name}.{domain}`) are covered by a wildcard cert written
/// at install time. This trait is only called for **custom domains** set via PUT /domain.
pub trait ProxyBackend: Send + Sync {
    /// Provision TLS and a vhost for a custom domain.
    fn provision_cert(&self, hostname: &str) -> Result<()>;

    /// Remove TLS config/cert for a custom domain on app delete or domain clear.
    fn remove_cert(&self, hostname: &str) -> Result<()>;

    /// Reload the proxy daemon after config changes.
    fn reload(&self) -> Result<()>;
}

/// No-op proxy for Docker mode: wildcard cert is provisioned once at install time;
/// custom domain TLS requires manual nginx config on the host.
pub struct NoopProxy;

impl ProxyBackend for NoopProxy {
    fn provision_cert(&self, hostname: &str) -> Result<()> {
        tracing::warn!(
            "Docker mode: custom domain TLS for '{hostname}' must be provisioned manually on the host (certbot --nginx -d {hostname})"
        );
        Ok(())
    }
    fn remove_cert(&self, hostname: &str) -> Result<()> {
        tracing::warn!("Docker mode: custom domain cert removal for '{hostname}' must be done manually on the host");
        Ok(())
    }
    fn reload(&self) -> Result<()> { Ok(()) }
}

/// Build the proxy backend from server config.
/// Uses NoopProxy when running inside Docker (bind_addr = 0.0.0.0) because certbot
/// and systemctl are on the host, not in the container.
pub fn from_config(cfg: &crate::config::ServerConfig) -> Arc<dyn ProxyBackend> {
    if cfg.bind_addr == "0.0.0.0" {
        return Arc::new(NoopProxy);
    }
    match &cfg.proxy {
        crate::config::ProxyBackend::Nginx => Arc::new(NginxBackend::new(cfg.domain.clone())),
        crate::config::ProxyBackend::Caddy => Arc::new(CaddyBackend::new(cfg.domain.clone())),
    }
}
