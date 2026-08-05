use anyhow::{bail, Result};

pub async fn run() -> Result<()> {
    let app = crate::cli::detect_app_name()
        .ok_or_else(|| anyhow::anyhow!(
            "not inside a remo app directory (no git remote named 'remo' found)"
        ))?;

    let status = std::process::Command::new("git")
        .args(["push", "remo", "main"])
        .status()?;

    if !status.success() {
        bail!("git push failed");
    }

    let sha = std::process::Command::new("git")
        .args(["rev-parse", "--short", "HEAD"])
        .output()
        .ok()
        .and_then(|o| String::from_utf8(o.stdout).ok())
        .map(|s| s.trim().to_string())
        .unwrap_or_default();

    println!("Deployed {app} @ {sha}");

    // Fetch hostname from server to print the live URL.
    if let Ok(cfg) = crate::config::ClientConfig::load() {
        use reqwest::header::{HeaderMap, HeaderValue, AUTHORIZATION};
        let mut headers = HeaderMap::new();
        if let Ok(v) = HeaderValue::from_str(&format!("Bearer {}", cfg.token)) {
            headers.insert(AUTHORIZATION, v);
        }
        if let Ok(client) = reqwest::Client::builder().default_headers(headers).build() {
            if let Ok(res) = client
                .get(format!("{}/api/apps/{app}", cfg.server_url))
                .send()
                .await
            {
                if let Ok(info) = res.json::<serde_json::Value>().await {
                    if let Some(h) = info["hostname"].as_str() {
                        println!("URL:     https://{h}");
                    }
                }
            }
        }
    }

    Ok(())
}
