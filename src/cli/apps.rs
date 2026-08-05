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
    let cfg = ClientConfig::load()?;
    let base = &cfg.server_url;
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
    let hostname = app["hostname"].as_str().unwrap_or(&args.name).to_string();

    // SSH remote: ssh://host/appname (works with any standard SSH config)
    let host = base
        .trim_start_matches("https://")
        .trim_start_matches("http://")
        .split('/')
        .next()
        .unwrap_or(base.as_str());
    let remote_url = format!("ssh://{host}/{}", args.name);

    println!("App:     {hostname}");
    println!("URL:     https://{hostname}");

    // Are we already inside a git repo?
    let in_git = std::process::Command::new("git")
        .args(["rev-parse", "--git-dir"])
        .stdout(std::process::Stdio::null())
        .stderr(std::process::Stdio::null())
        .status()
        .map(|s| s.success())
        .unwrap_or(false);

    if in_git {
        // Already a repo — just add the remote.
        let added = std::process::Command::new("git")
            .args(["remote", "add", "remo", &remote_url])
            .status()
            .map(|s| s.success())
            .unwrap_or(false);
        if added {
            println!("Remote:  remo → {remote_url}");
        } else {
            println!("Remote:  remo already exists (skipped)");
        }
        println!("Deploy:  remo push");
    } else {
        // Create directory, init repo, write starter file, initial commit.
        std::fs::create_dir_all(&args.name)?;

        // Write a minimal starter matching the entrypoint filename.
        let starter = format!("{}/{}", args.name, args.entrypoint);
        if !std::path::Path::new(&starter).exists() {
            std::fs::write(&starter, starter_js(&args.name))?;
            println!("Created: {starter}");
        }

        let run = |cmd: &str, a: &[&str]| {
            std::process::Command::new(cmd)
                .args(a)
                .current_dir(&args.name)
                .status()
                .map(|s| s.success())
                .unwrap_or(false)
        };

        run("git", &["init", "-q"]);
        run("git", &["add", "."]);
        run("git", &["commit", "-q", "-m", "init"]);
        run("git", &["remote", "add", "remo", &remote_url]);

        println!("Remote:  remo → {remote_url}");
        println!();
        println!("  cd {} && remo push", args.name);
    }

    Ok(())
}

fn starter_js(name: &str) -> String {
    format!(
        r#"export default {{
  fetch(request) {{
    const url = new URL(request.url);
    if (url.pathname === "/") {{
      return new Response("hello from {name}", {{ status: 200 }});
    }}
    return new Response("Not Found", {{ status: 404 }});
  }},
}};
"#
    )
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
