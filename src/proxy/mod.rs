pub mod caddy;
pub mod nginx;

use anyhow::Result;

pub use caddy::CaddyBackend;
pub use nginx::NginxBackend;

/// Proxy backend: manages TLS and vhost config for a reverse proxy.
/// nano-rs does all app routing by Host header — the proxy's only job is TLS termination.
pub trait ProxyBackend: Send + Sync {
    /// Ensure the proxy config will accept HTTPS for this hostname.
    /// nginx: certbot --nginx -d <hostname>; Caddy: noop (on-demand TLS).
    fn provision_cert(&self, hostname: &str) -> Result<()>;

    /// Remove config/cert for hostname on app delete.
    fn remove_cert(&self, hostname: &str) -> Result<()>;

    /// Reload the proxy daemon after config changes.
    fn reload(&self) -> Result<()>;
}

pub fn from_config(backend: &crate::config::ProxyBackend, domain: &str) -> Box<dyn ProxyBackend> {
    match backend {
        crate::config::ProxyBackend::Nginx => Box::new(NginxBackend::new(domain.to_string())),
        crate::config::ProxyBackend::Caddy => Box::new(CaddyBackend::new(domain.to_string())),
    }
}
