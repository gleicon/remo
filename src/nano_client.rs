use anyhow::Result;
use reqwest::Client;
use serde::{Deserialize, Serialize};

#[derive(Debug, Clone)]
pub struct NanoClient {
    base: String,
    client: Client,
}

impl NanoClient {
    pub fn new(base_url: impl Into<String>) -> Self {
        Self::new_with_key(base_url, None::<String>)
    }

    pub fn new_with_key(base_url: impl Into<String>, api_key: Option<impl Into<String>>) -> Self {
        let mut headers = reqwest::header::HeaderMap::new();
        if let Some(key) = api_key {
            let key_str = key.into();
            if !key_str.is_empty() {
                if let Ok(val) = reqwest::header::HeaderValue::from_str(&key_str) {
                    headers.insert("X-Admin-Key", val);
                }
            }
        }
        Self {
            base: base_url.into().trim_end_matches('/').to_string(),
            client: Client::builder().default_headers(headers).build().unwrap_or_default(),
        }
    }

    pub fn from_config(cfg: &crate::config::ServerConfig) -> Self {
        // REMO_NANO_SOCKET overrides server.toml — lets the host git-hook use
        // 127.0.0.1:8889 while the Docker container uses the nano-rs DNS name.
        let socket = std::env::var("REMO_NANO_SOCKET")
            .unwrap_or_else(|_| cfg.nano_socket.clone());
        Self::new_with_key(&socket, cfg.nano_admin_key.as_deref())
    }

    // ── App CRUD ──────────────────────────────────────────────────────────

    pub async fn create_app(&self, req: &CreateAppRequest) -> Result<()> {
        let res = self
            .client
            .post(format!("{}/admin/apps", self.base))
            .json(req)
            .send()
            .await?;
        check_status(res).await
    }

    pub async fn update_app(&self, hostname: &str, req: &UpdateAppRequest) -> Result<()> {
        let res = self
            .client
            .patch(format!("{}/admin/apps/{hostname}", self.base))
            .json(req)
            .send()
            .await?;
        check_status(res).await
    }

    pub async fn delete_app(&self, hostname: &str) -> Result<()> {
        let res = self
            .client
            .delete(format!("{}/admin/apps/{hostname}", self.base))
            .send()
            .await?;
        check_status(res).await
    }

    pub async fn reload_app(&self, hostname: &str) -> Result<()> {
        let res = self
            .client
            .post(format!("{}/admin/apps/{hostname}/reload", self.base))
            .send()
            .await?;
        check_status(res).await
    }

    pub async fn scale_app(&self, hostname: &str, workers: u32) -> Result<()> {
        let res = self
            .client
            .post(format!("{}/admin/apps/{hostname}/scale", self.base))
            .json(&serde_json::json!({ "workers": workers }))
            .send()
            .await?;
        check_status(res).await
    }

    pub async fn drain_app(&self, hostname: &str) -> Result<()> {
        let res = self
            .client
            .post(format!("{}/admin/apps/{hostname}/drain", self.base))
            .send()
            .await?;
        check_status(res).await
    }

    // ── Observability ─────────────────────────────────────────────────────

    /// Fetch live isolate stats from nano-rs (V8 heap + request count per worker).
    /// Returns the raw JSON array from /admin/isolates.
    pub async fn get_isolates(&self) -> Result<serde_json::Value> {
        let res = self
            .client
            .get(format!("{}/admin/isolates", self.base))
            .send()
            .await?;
        if !res.status().is_success() {
            anyhow::bail!(
                "nano-rs /admin/isolates {}: {}",
                res.status(),
                res.text().await.unwrap_or_default()
            );
        }
        Ok(res.json().await?)
    }

    // ── Env ───────────────────────────────────────────────────────────────

    pub async fn set_env(&self, hostname: &str, vars: Vec<(String, String)>) -> Result<()> {
        let env: std::collections::HashMap<_, _> = vars.into_iter().collect();
        let res = self
            .client
            .patch(format!("{}/admin/apps/{hostname}", self.base))
            .json(&UpdateAppRequest {
                env_vars: Some(env),
                entrypoint: None,
                limits: None,
            })
            .send()
            .await?;
        check_status(res).await
    }
}

async fn check_status(res: reqwest::Response) -> Result<()> {
    let status = res.status();
    if status.is_success() {
        return Ok(());
    }
    let body = res.text().await.unwrap_or_default();
    anyhow::bail!("nano-rs admin API {status}: {body}")
}

/// Returns true if an error from this client indicates HTTP 404 Not Found.
/// Co-located with check_status so the format stays in sync.
pub fn is_not_found(e: &anyhow::Error) -> bool {
    e.to_string().contains(" 404 ")
}

// ── Request shapes ────────────────────────────────────────────────────────────

#[derive(Debug, Serialize, Deserialize, Default)]
pub struct AppLimits {
    pub workers: u32,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub memory_mb: Option<u64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub timeout_secs: Option<u64>,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct CreateAppRequest {
    pub hostname: String,
    /// Absolute path to the JS entrypoint file, accessible to nano-rs.
    pub entrypoint: String,
    #[serde(default, skip_serializing_if = "std::collections::HashMap::is_empty")]
    pub env_vars: std::collections::HashMap<String, String>,
    #[serde(default)]
    pub limits: AppLimits,
    /// Immediately activate the app (skip pending phase).
    #[serde(default = "bool_true")]
    pub activate: bool,
}

fn bool_true() -> bool { true }

#[derive(Debug, Serialize, Deserialize)]
pub struct UpdateAppRequest {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub entrypoint: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub env_vars: Option<std::collections::HashMap<String, String>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub limits: Option<AppLimits>,
}
