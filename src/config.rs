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

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct ServerConfig {
    pub domain: String,
    pub data_dir: String,
    pub nano_socket: String,
    pub proxy: ProxyBackend,
    pub control_port: u16,
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
