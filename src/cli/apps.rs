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

    /// Scaffold an HTML-serving JS worker instead of the default API starter
    #[arg(long, conflicts_with = "wasm")]
    pub html: bool,

    /// Scaffold a WebAssembly starter (JS wrapper + inline wasm module)
    #[arg(long, conflicts_with = "html")]
    pub wasm: bool,

    /// App type: js | wasm | static (overridden by --html/--wasm)
    #[arg(long = "type", default_value = "js")]
    pub app_type: String,

    /// Entry point (overridden by --html/--wasm)
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

enum Template { Js, Html, Wasm }

async fn create(client: reqwest::Client, args: CreateArgs) -> Result<()> {
    let cfg = ClientConfig::load()?;
    let base = &cfg.server_url;

    let template = if args.wasm { Template::Wasm } else if args.html { Template::Html } else { Template::Js };
    let (app_type, entrypoint) = match &template {
        Template::Wasm => ("js".to_string(), "index.js".to_string()),
        Template::Html => ("js".to_string(), "index.js".to_string()),
        Template::Js   => (args.app_type.clone(), args.entrypoint.clone()),
    };

    let res = client
        .post(format!("{base}/api/apps"))
        .json(&serde_json::json!({
            "name": args.name,
            "type": app_type,
            "entrypoint": entrypoint,
        }))
        .send()
        .await?;

    if !res.status().is_success() {
        let body = res.text().await?;
        anyhow::bail!("{body}");
    }

    let app: serde_json::Value = res.json().await?;
    let hostname = app["hostname"].as_str().unwrap_or(&args.name).to_string();

    let host = base
        .trim_start_matches("https://")
        .trim_start_matches("http://")
        .split('/')
        .next()
        .unwrap_or(base.as_str());
    let remote_url = format!("ssh://{host}/{}", args.name);

    println!("App:     {hostname}");
    println!("URL:     https://{hostname}");

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
            println!("Remote:  remo → {remote_url}");
        } else {
            println!("Remote:  remo already exists (skipped)");
        }
        println!("Deploy:  remo push");
    } else {
        std::fs::create_dir_all(&args.name)?;

        let files: Vec<(&str, String)> = match &template {
            Template::Html => vec![
                ("index.js", starter_html(&args.name)),
            ],
            Template::Wasm => vec![
                ("index.js", starter_wasm(&args.name)),
            ],
            Template::Js => vec![
                (&entrypoint, starter_js(&args.name)),
            ],
        };

        for (filename, content) in &files {
            let path = format!("{}/{filename}", args.name);
            if !std::path::Path::new(&path).exists() {
                std::fs::write(&path, content)?;
                println!("Created: {path}");
            }
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

fn starter_html(name: &str) -> String {
    format!(
        r#"const HTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>{name}</title>
  <style>
    body {{ font-family: sans-serif; max-width: 640px; margin: 4rem auto; padding: 0 1rem; }}
    h1 {{ color: #111; }}
  </style>
</head>
<body>
  <h1>{name}</h1>
  <p>Edit this HTML in <code>index.js</code> and run <code>remo push</code>.</p>
</body>
</html>`;

export default {{
  fetch(request) {{
    const url = new URL(request.url);
    if (url.pathname === "/") {{
      return new Response(HTML, {{
        status: 200,
        headers: {{ "Content-Type": "text/html; charset=utf-8" }},
      }});
    }}
    return new Response("Not Found", {{ status: 404 }});
  }},
}};
"#
    )
}

fn starter_wasm(_name: &str) -> String {
    // Minimal wasm module: exports add(i32, i32) -> i32
    // Compile your own with wasm-pack or replace WASM_B64 with your module.
    r#"// Inline wasm module: exports add(a, b) -> a + b
// Replace WASM_B64 with your own compiled .wasm (base64-encoded).
const WASM_B64 = "AGFzbQEAAAABBwFgAn9/AX8DAgEABwcBA2FkZAAACgkBBwAgACABags=";

function b64ToBytes(b64) {
  const bin = atob(b64);
  return Uint8Array.from(bin, (c) => c.charCodeAt(0));
}

let _wasm;
async function getWasm() {
  if (!_wasm) {
    const { instance } = await WebAssembly.instantiate(b64ToBytes(WASM_B64));
    _wasm = instance.exports;
  }
  return _wasm;
}

export default {
  async fetch(request) {
    const url = new URL(request.url);
    if (url.pathname === "/add") {
      const w = await getWasm();
      const a = Number(url.searchParams.get("a") ?? 0);
      const b = Number(url.searchParams.get("b") ?? 0);
      return new Response(String(w.add(a, b)), {
        status: 200,
        headers: { "Content-Type": "text/plain" },
      });
    }
    return new Response("try /add?a=2&b=3", { status: 200 });
  },
};
"#.to_string()
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
