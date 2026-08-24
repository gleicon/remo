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

    /// Template to scaffold: js (default), html, wasm, kv, spa, gas,
    /// or a name from ~/.remo/templates/<name>/ for your own starters
    #[arg(long, short = 't', value_name = "NAME")]
    pub template: Option<String>,

    // Legacy per-template flags — kept for backwards compatibility, not shown in --help.
    #[arg(long, conflicts_with = "template", hide = true)]
    pub html: bool,
    #[arg(long, conflicts_with = "template", hide = true)]
    pub wasm: bool,
    #[arg(long, conflicts_with = "template", hide = true)]
    pub kv: bool,
    #[arg(long, conflicts_with = "template", hide = true)]
    pub spa: bool,
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

// ── Template resolution ───────────────────────────────────────────────────────

/// Files to write into the new app directory: (relative_path, content).
type TemplateFiles = Vec<(String, String)>;

/// Resolve template name → list of files to write, app_type, entrypoint.
fn resolve_template(tpl: &str, app_name: &str) -> Result<(TemplateFiles, String, String)> {
    // User templates override built-ins: ~/.remo/templates/<name>/  or  ~/.remo/templates/<name>.js
    if let Some(files) = load_user_template(tpl, app_name)? {
        let entry = files.iter().find(|(p, _)| p == "index.js")
            .map(|(p, _)| p.clone())
            .unwrap_or_else(|| files.first().map(|(p, _)| p.clone()).unwrap_or_else(|| "index.js".into()));
        return Ok((files, "js".into(), entry));
    }

    let files: TemplateFiles = match tpl {
        "html" => vec![("index.js".into(), starter_html(app_name))],
        "wasm" => vec![("index.js".into(), starter_wasm())],
        "kv"   => vec![("index.js".into(), starter_kv(app_name))],
        "spa"  => vec![("index.js".into(), starter_spa(app_name))],
        "gas"  => vec![("index.gs".into(), starter_gas(app_name))],
        _      => vec![("index.js".into(), starter_js(app_name))],
    };
    let entry = if tpl == "gas" { "index.gs" } else { "index.js" };
    Ok((files, "js".into(), entry.into()))
}

/// Load a user-defined template from ~/.remo/templates/<name>/ or ~/.remo/templates/<name>.js.
/// Returns None if no such template exists, Err on I/O failure.
fn load_user_template(name: &str, app_name: &str) -> Result<Option<TemplateFiles>> {
    let Some(home) = dirs::home_dir() else { return Ok(None) };
    let base = home.join(".remo/templates");

    // Directory template: ~/.remo/templates/<name>/
    let dir = base.join(name);
    if dir.is_dir() {
        let mut files = Vec::new();
        for entry in std::fs::read_dir(&dir)? {
            let entry = entry?;
            let path = entry.path();
            if path.is_file() {
                let rel = path.file_name()
                    .and_then(|n| n.to_str())
                    .unwrap_or("")
                    .to_string();
                if rel.is_empty() { continue; }
                let content = std::fs::read_to_string(&path)?;
                // Replace {{name}} placeholder with the actual app name.
                files.push((rel, content.replace("{{name}}", app_name)));
            }
        }
        if !files.is_empty() {
            return Ok(Some(files));
        }
    }

    // Single-file template: ~/.remo/templates/<name>.js
    let single = base.join(format!("{name}.js"));
    if single.is_file() {
        let content = std::fs::read_to_string(&single)?;
        return Ok(Some(vec![("index.js".into(), content.replace("{{name}}", app_name))]));
    }

    Ok(None)
}

// ── Create command ────────────────────────────────────────────────────────────

async fn create(client: reqwest::Client, args: CreateArgs) -> Result<()> {
    let cfg = ClientConfig::load()?;
    let base = &cfg.server_url;

    // Resolve template name from --template or legacy flags.
    let tpl_name = if let Some(ref t) = args.template {
        t.as_str().to_owned()
    } else if args.html { "html".into() }
      else if args.wasm { "wasm".into() }
      else if args.kv   { "kv".into() }
      else if args.spa  { "spa".into() }
      else              { "js".into() };

    let (files, app_type, entrypoint) = resolve_template(&tpl_name, &args.name)?;

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

// ── Built-in template starters ────────────────────────────────────────────────

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

fn starter_kv(name: &str) -> String {
    format!(
        r#"// Persistent request counter — survives restarts via nano:kv.
// Docs: https://github.com/gleicon/nano-rs
import {{ kv }} from 'nano:kv';

export default {{
  async fetch(request) {{
    const url = new URL(request.url);

    if (url.pathname === '/reset' && request.method === 'POST') {{
      await kv.set('hits', new TextEncoder().encode('0'));
      return new Response(JSON.stringify({{ hits: 0 }}), {{
        headers: {{ 'content-type': 'application/json' }},
      }});
    }}

    if (url.pathname !== '/') {{
      return new Response('Not Found', {{ status: 404 }});
    }}

    const raw = await kv.get('hits');
    const hits = raw ? parseInt(new TextDecoder().decode(raw), 10) : 0;
    const next = hits + 1;
    await kv.set('hits', new TextEncoder().encode(String(next)));

    return new Response(
      JSON.stringify({{ app: '{name}', hits: next }}),
      {{ status: 200, headers: {{ 'content-type': 'application/json' }} }},
    );
  }},
}};
"#
    )
}

fn starter_spa(name: &str) -> String {
    format!(
        r#"// SPA shell — uses localStorage (backed by nano:kv) for browser-compatible storage.
// localStorage.getItem/setItem/removeItem work just like in a browser.
import {{ openKV }} from 'nano:kv';

// ── localStorage shim over nano:kv ───────────────────────────────────────────
const store = openKV('localStorage');
const localStorage = {{
  async getItem(key) {{
    const b = await store.get(String(key));
    return b ? new TextDecoder().decode(b) : null;
  }},
  async setItem(key, value) {{
    await store.set(String(key), new TextEncoder().encode(String(value)));
  }},
  async removeItem(key) {{
    await store.delete(String(key));
  }},
}};
globalThis.localStorage = localStorage;

// ── HTML shell ───────────────────────────────────────────────────────────────
const shell = (count, theme) => `<!DOCTYPE html>
<html lang="en" data-theme="${{theme}}">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>{name}</title>
  <style>
    [data-theme=dark]  {{ background:#111; color:#eee; }}
    [data-theme=light] {{ background:#fff; color:#111; }}
    body {{ font-family: sans-serif; max-width: 480px; margin: 4rem auto; padding: 0 1rem; }}
    button {{ margin: .5rem .25rem; padding: .4rem .9rem; cursor: pointer; }}
  </style>
</head>
<body>
  <h1>{name}</h1>
  <p>Visits: <strong id="count">${{count}}</strong></p>
  <p>Theme: <strong id="theme">${{theme}}</strong></p>
  <button onclick="fetch('/theme?t=dark').then(()=>location.reload())">Dark</button>
  <button onclick="fetch('/theme?t=light').then(()=>location.reload())">Light</button>
  <button onclick="fetch('/reset',{{method:'POST'}}).then(()=>location.reload())">Reset</button>
</body>
</html>`;

// ── Request handler ───────────────────────────────────────────────────────────
export default {{
  async fetch(request) {{
    const url = new URL(request.url);

    if (url.pathname === '/theme') {{
      const t = url.searchParams.get('t') === 'dark' ? 'dark' : 'light';
      await localStorage.setItem('theme', t);
      return new Response(null, {{ status: 204 }});
    }}

    if (url.pathname === '/reset' && request.method === 'POST') {{
      await localStorage.removeItem('visits');
      return new Response(null, {{ status: 204 }});
    }}

    if (url.pathname === '/') {{
      const raw = await localStorage.getItem('visits');
      const visits = raw ? parseInt(raw, 10) : 0;
      await localStorage.setItem('visits', String(visits + 1));
      const theme = (await localStorage.getItem('theme')) ?? 'light';
      return new Response(shell(visits + 1, theme), {{
        status: 200,
        headers: {{ 'content-type': 'text/html; charset=utf-8' }},
      }});
    }}

    return new Response('Not Found', {{ status: 404 }});
  }},
}};
"#,
        name = name
    )
}

fn starter_wasm() -> String {
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

fn starter_gas(name: &str) -> String {
    format!(
        r#"// Google Apps Script-compatible handler — runs on nano-rs via nano:gas shim.
// Deploy with: remo apps create myapp --template gas && remo push

function doGet(e) {{
  return ContentService
    .createTextOutput(JSON.stringify({{ app: '{name}', path: e.pathInfo || '/', params: e.parameters }}))
    .setMimeType(ContentService.MimeType.JSON);
}}

function doPost(e) {{
  const body = e.postData ? JSON.parse(e.postData.contents) : {{}};
  return ContentService
    .createTextOutput(JSON.stringify({{ app: '{name}', received: body }}))
    .setMimeType(ContentService.MimeType.JSON);
}}
"#
    )
}

// ── Other subcommands ─────────────────────────────────────────────────────────

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
