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
        Self {
            base: base_url.into().trim_end_matches('/').to_string(),
            client: Client::new(),
        }
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
            .put(format!("{}/admin/apps/{hostname}", self.base))
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

    // ── Env ───────────────────────────────────────────────────────────────

    pub async fn set_env(&self, hostname: &str, vars: Vec<(String, String)>) -> Result<()> {
        let env: std::collections::HashMap<_, _> = vars.into_iter().collect();
        let res = self
            .client
            .put(format!("{}/admin/apps/{hostname}", self.base))
            .json(&UpdateAppRequest {
                env_vars: Some(env),
                script: None,
                workers: None,
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

#[derive(Debug, Serialize, Deserialize)]
pub struct CreateAppRequest {
    pub hostname: String,
    pub script: String,
    pub workers: u32,
    pub cpu_time_ms: Option<u64>,
    pub memory_mb: Option<u64>,
    pub env_vars: Option<std::collections::HashMap<String, String>>,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct UpdateAppRequest {
    pub script: Option<String>,
    pub workers: Option<u32>,
    pub env_vars: Option<std::collections::HashMap<String, String>>,
}
