use anyhow::Result;

use crate::config::ClientConfig;

// ── Scale ─────────────────────────────────────────────────────────────────────

#[derive(clap::Args)]
pub struct ScaleArgs {
    pub app: String,
    pub workers: u32,
}

pub async fn scale(args: ScaleArgs) -> Result<()> {
    let cfg = ClientConfig::load()?;
    let client = api_client(&cfg);
    let res = client
        .post(format!("{}/api/apps/{}/scale", cfg.server_url, args.app))
        .json(&serde_json::json!({ "workers": args.workers }))
        .send()
        .await?;
    if !res.status().is_success() {
        anyhow::bail!("{}", res.text().await?);
    }
    println!("{} scaled to {} workers", args.app, args.workers);
    Ok(())
}

// ── Deployments ───────────────────────────────────────────────────────────────

#[derive(clap::Args)]
pub struct DeploymentsArgs {
    pub app: String,
}

pub async fn deployments(args: DeploymentsArgs) -> Result<()> {
    let cfg = ClientConfig::load()?;
    let client = api_client(&cfg);
    let res = client
        .get(format!("{}/api/apps/{}/deployments", cfg.server_url, args.app))
        .send()
        .await?;
    let deploys: Vec<serde_json::Value> = res.json().await?;
    for d in &deploys {
        println!(
            "{} {} {}",
            d["id"].as_str().unwrap_or("?"),
            d["status"].as_str().unwrap_or("?"),
            d["created_at"].as_str().unwrap_or(""),
        );
    }
    Ok(())
}

// ── Rollback ──────────────────────────────────────────────────────────────────

#[derive(clap::Args)]
pub struct RollbackArgs {
    pub app: String,
    /// SHA or deploy ID to roll back to (default: previous)
    pub sha: Option<String>,
}

pub async fn rollback(args: RollbackArgs) -> Result<()> {
    let cfg = ClientConfig::load()?;
    let client = api_client(&cfg);
    let res = client
        .post(format!("{}/api/apps/{}/rollback", cfg.server_url, args.app))
        .json(&serde_json::json!({ "sha": args.sha }))
        .send()
        .await?;
    if !res.status().is_success() {
        anyhow::bail!("{}", res.text().await?);
    }
    println!("Rolled back {}", args.app);
    Ok(())
}

fn api_client(cfg: &ClientConfig) -> reqwest::Client {
    use reqwest::header::{HeaderMap, HeaderValue, AUTHORIZATION};
    let mut headers = HeaderMap::new();
    headers.insert(
        AUTHORIZATION,
        HeaderValue::from_str(&format!("Bearer {}", cfg.token)).unwrap(),
    );
    reqwest::Client::builder().default_headers(headers).build().unwrap()
}
