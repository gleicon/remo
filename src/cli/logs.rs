use anyhow::{bail, Result};

use crate::config::ClientConfig;

#[derive(clap::Args)]
pub struct LogsArgs {
    /// App name (optional — inferred from git remote when omitted)
    pub app: Option<String>,

    /// Number of recent lines to fetch
    #[arg(long, default_value_t = 100)]
    pub lines: u64,
}

pub async fn run(args: LogsArgs) -> Result<()> {
    let app = args.app
        .or_else(crate::cli::detect_app_name)
        .ok_or_else(|| anyhow::anyhow!(
            "app name required (or run from inside a remo app directory)"
        ))?;

    let cfg = ClientConfig::load()?;
    let client = {
        use reqwest::header::{HeaderMap, HeaderValue, AUTHORIZATION};
        let mut headers = HeaderMap::new();
        headers.insert(
            AUTHORIZATION,
            HeaderValue::from_str(&format!("Bearer {}", cfg.token)).unwrap(),
        );
        reqwest::Client::builder().default_headers(headers).build().unwrap()
    };

    let res = client
        .get(format!("{}/api/apps/{}/logs?lines={}", cfg.server_url, app, args.lines))
        .send()
        .await?;

    if !res.status().is_success() {
        bail!("{}", res.text().await?);
    }

    print!("{}", res.text().await?);
    Ok(())
}
