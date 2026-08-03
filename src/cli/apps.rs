use anyhow::Result;
use clap::Subcommand;

use crate::config::ClientConfig;

#[derive(Subcommand)]
pub enum AppsCmd {
    /// Create a new app
    Create(CreateArgs),
    /// List apps
    List,
    /// Show app details
    Info { name: String },
    /// Delete an app
    Delete { name: String },
}

#[derive(clap::Args)]
pub struct CreateArgs {
    /// App name (becomes <name>.<domain>)
    pub name: String,

    /// App type: js | wasm | static
    #[arg(long = "type", default_value = "js")]
    pub app_type: String,

    /// Entry point (e.g. index.js or index.wasm)
    #[arg(long, default_value = "index.js")]
    pub entrypoint: String,
}

pub async fn run(cmd: AppsCmd) -> Result<()> {
    let cfg = ClientConfig::load()?;
    let client = api_client(&cfg);

    match cmd {
        AppsCmd::Create(args) => create(client, args).await,
        AppsCmd::List => list(client).await,
        AppsCmd::Info { name } => info(client, name).await,
        AppsCmd::Delete { name } => delete(client, name).await,
    }
}

async fn create(client: reqwest::Client, args: CreateArgs) -> Result<()> {
    let base = ClientConfig::load()?.server_url;
    let res = client
        .post(format!("{base}/api/apps"))
        .json(&serde_json::json!({
            "name": args.name,
            "type": args.app_type,
            "entrypoint": args.entrypoint,
        }))
        .send()
        .await?;

    if !res.status().is_success() {
        let body = res.text().await?;
        anyhow::bail!("{body}");
    }

    let app: serde_json::Value = res.json().await?;
    let host = base
        .trim_start_matches("https://")
        .trim_start_matches("http://")
        .split('/')
        .next()
        .unwrap_or(&base);
    let remote_url = format!("git@{host}:{}", args.name);

    println!("Created: {}", app["hostname"].as_str().unwrap_or(&args.name));

    // If we're inside a git repo, add the remote automatically.
    let in_git = std::process::Command::new("git")
        .args(["rev-parse", "--git-dir"])
        .stdout(std::process::Stdio::null())
        .stderr(std::process::Stdio::null())
        .status()
        .map(|s| s.success())
        .unwrap_or(false);

    if in_git {
        let added = std::process::Command::new("git")
            .args(["remote", "add", "remo", &remote_url])
            .status()
            .map(|s| s.success())
            .unwrap_or(false);
        if added {
            println!("Remote:  added 'remo' → {remote_url}");
            println!("Push:    git push remo main");
        } else {
            println!("Remote:  {remote_url}  (remote 'remo' already exists — skipped)");
        }
    } else {
        println!("Remote:  git remote add remo {remote_url}");
        println!("Push:    git push remo main");
    }

    Ok(())
}

async fn list(client: reqwest::Client) -> Result<()> {
    let base = ClientConfig::load()?.server_url;
    let res = client.get(format!("{base}/api/apps")).send().await?;
    let apps: Vec<serde_json::Value> = res.json().await?;
    if apps.is_empty() {
        println!("No apps.");
    } else {
        for app in &apps {
            println!(
                "{:<20} {}",
                app["name"].as_str().unwrap_or("?"),
                app["hostname"].as_str().unwrap_or("")
            );
        }
    }
    Ok(())
}

async fn info(client: reqwest::Client, name: String) -> Result<()> {
    let base = ClientConfig::load()?.server_url;
    let res = client.get(format!("{base}/api/apps/{name}")).send().await?;
    if res.status().as_u16() == 404 {
        anyhow::bail!("app '{name}' not found");
    }
    let app: serde_json::Value = res.json().await?;
    println!("{}", serde_json::to_string_pretty(&app)?);
    Ok(())
}

async fn delete(client: reqwest::Client, name: String) -> Result<()> {
    let base = ClientConfig::load()?.server_url;
    let res = client.delete(format!("{base}/api/apps/{name}")).send().await?;
    if !res.status().is_success() {
        let body = res.text().await?;
        anyhow::bail!("{body}");
    }
    println!("Deleted {name}");
    Ok(())
}

fn api_client(cfg: &ClientConfig) -> reqwest::Client {
    use reqwest::header::{HeaderMap, HeaderValue, AUTHORIZATION};
    let mut headers = HeaderMap::new();
    headers.insert(
        AUTHORIZATION,
        HeaderValue::from_str(&format!("Bearer {}", cfg.token)).unwrap(),
    );
    reqwest::Client::builder()
        .default_headers(headers)
        .build()
        .unwrap()
}
