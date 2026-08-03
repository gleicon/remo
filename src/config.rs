use anyhow::{Context, Result};
use serde::{Deserialize, Serialize};
use std::path::PathBuf;

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ClientConfig {
    pub server_url: String,
    pub token: String,
}

impl ClientConfig {
    pub fn path() -> Result<PathBuf> {
        let home = dirs::home_dir().context("no home directory")?;
        Ok(home.join(".remo").join("config.toml"))
    }

    pub fn load() -> Result<Self> {
        let path = Self::path()?;
        let raw = std::fs::read_to_string(&path)
            .with_context(|| format!("run `remo login` first (config not found: {})", path.display()))?;
        toml::from_str(&raw).context("invalid config file")
    }

    pub fn save(&self) -> Result<()> {
        let path = Self::path()?;
        if let Some(parent) = path.parent() {
            std::fs::create_dir_all(parent)?;
        }
        let raw = toml::to_string_pretty(self)?;
        std::fs::write(&path, raw)?;
        Ok(())
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ServerConfig {
    pub domain: String,
    pub data_dir: String,
    pub nano_socket: String,
    /// API key sent as X-Admin-Key to nano-rs admin API.
    /// Must match NANO_ADMIN_API_KEY in the nano-rs environment.
    pub nano_admin_key: Option<String>,
    pub proxy: ProxyBackend,
    pub control_port: u16,
    /// Bind address for the control plane. Use 0.0.0.0 in Docker (port
    /// security enforced by the host-side Docker port binding).
    #[serde(default = "default_bind_addr")]
    pub bind_addr: String,
}

fn default_bind_addr() -> String {
    "127.0.0.1".to_string()
}

impl Default for ServerConfig {
    fn default() -> Self {
        Self {
            domain: Default::default(),
            data_dir: Default::default(),
            nano_socket: Default::default(),
            nano_admin_key: None,
            proxy: Default::default(),
            control_port: 7070,
            bind_addr: default_bind_addr(),
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize, Default, clap::ValueEnum)]
pub enum ProxyBackend {
    #[default]
    Nginx,
    Caddy,
}

impl ServerConfig {
    pub fn system_path() -> PathBuf {
        PathBuf::from("/etc/remo/server.toml")
    }

    pub fn load() -> Result<Self> {
        let path = Self::system_path();
        let raw = std::fs::read_to_string(&path)
            .with_context(|| format!("server not initialized (missing {})", path.display()))?;
        toml::from_str(&raw).context("invalid server config")
    }

    pub fn save(&self) -> Result<()> {
        let path = Self::system_path();
        if let Some(parent) = path.parent() {
            std::fs::create_dir_all(parent)?;
        }
        let raw = toml::to_string_pretty(self)?;
        std::fs::write(&path, raw)?;
        Ok(())
    }
}
