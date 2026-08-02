use anyhow::Result;

use crate::config::ClientConfig;

#[derive(clap::Args)]
pub struct LogsArgs {
    pub app: String,

    /// Number of recent lines to fetch
    #[arg(long, default_value_t = 100)]
    pub lines: u64,

    /// Follow (not yet implemented)
    #[arg(short, long)]
    pub follow: bool,
}

pub async fn run(args: LogsArgs) -> Result<()> {
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
        .get(format!("{}/api/apps/{}/logs?lines={}", cfg.server_url, args.app, args.lines))
        .send()
        .await?;

    if !res.status().is_success() {
        anyhow::bail!("{}", res.text().await?);
    }

    let body = res.text().await?;
    print!("{body}");
    Ok(())
}
