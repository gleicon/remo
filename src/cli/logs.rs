use anyhow::{bail, Result};

use crate::config::ClientConfig;

#[derive(clap::Args)]
pub struct LogsArgs {
    /// App name (optional — inferred from git remote when omitted)
    pub app: Option<String>,

    /// Number of recent deployment entries to show
    #[arg(long, default_value_t = 20)]
    pub lines: u64,

    /// Poll live runtime stats (request count, heap) every 2 seconds
    #[arg(short, long)]
    pub follow: bool,
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

    // Always show deployment history first.
    let res = client
        .get(format!("{}/api/apps/{}/logs?lines={}", cfg.server_url, app, args.lines))
        .send()
        .await?;
    if !res.status().is_success() {
        bail!("{}", res.text().await?);
    }
    print!("{}", res.text().await?);

    if !args.follow {
        return Ok(());
    }

    // Follow mode: poll /api/apps/{app}/stats every 2 seconds.
    // Shows live V8 isolate stats (request count, heap) from nano-rs /admin/isolates.
    println!("\n--- following runtime stats (Ctrl+C to stop) ---");
    let mut last_total: Option<u64> = None;
    loop {
        tokio::time::sleep(std::time::Duration::from_secs(2)).await;
        let Ok(res) = client
            .get(format!("{}/api/apps/{}/stats", cfg.server_url, app))
            .send()
            .await
        else {
            continue;
        };
        if !res.status().is_success() {
            continue;
        }
        let Ok(stats) = res.json::<serde_json::Value>().await else {
            continue;
        };
        let isolates = stats["isolates"].as_array().map(|a| a.as_slice()).unwrap_or(&[]);
        let total_requests: u64 = isolates.iter()
            .filter_map(|i| i["requests"].as_u64())
            .sum();
        let heap_kb: u64 = isolates.iter()
            .filter_map(|i| i["heap_used"].as_u64())
            .sum::<u64>() / 1024;

        if last_total != Some(total_requests) {
            let ts = chrono::Utc::now().format("%Y-%m-%dT%H:%M:%SZ");
            println!(
                "{ts}  workers={}  requests={}  heap={heap_kb}KB",
                isolates.len(),
                total_requests,
            );
            last_total = Some(total_requests);
        }
    }
}
