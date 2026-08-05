use anyhow::Result;
use clap::Subcommand;

use crate::config::ClientConfig;

#[derive(Subcommand)]
pub enum EnvCmd {
    /// Set env vars: KEY=VALUE ...
    Set {
        /// App name (optional — inferred from git remote when omitted)
        app: Option<String>,
        /// KEY=VALUE pairs
        #[arg(required = true)]
        pairs: Vec<String>,
    },
    /// List env vars
    List {
        /// App name (optional — inferred from git remote when omitted)
        app: Option<String>,
    },
    /// Remove an env var
    Unset {
        /// App name (optional — inferred from git remote when omitted)
        app: Option<String>,
        key: String,
    },
}

pub async fn run(cmd: EnvCmd) -> Result<()> {
    let cfg = ClientConfig::load()?;
    let client = api_client(&cfg);
    let base = &cfg.server_url;

    match cmd {
        EnvCmd::Set { app, pairs } => {
            let app = resolve_app(app)?;
            let vars: Result<Vec<_>> = pairs.iter().map(|p| {
                let (k, v) = p.split_once('=').ok_or_else(|| {
                    anyhow::anyhow!("invalid pair '{}': expected KEY=VALUE", p)
                })?;
                Ok((k.to_string(), v.to_string()))
            }).collect();
            let map: serde_json::Value = serde_json::Value::Object(
                vars?.into_iter()
                    .map(|(k, v)| (k, serde_json::Value::String(v)))
                    .collect(),
            );
            let res = client
                .put(format!("{base}/api/apps/{app}/env"))
                .json(&serde_json::json!({ "vars": map }))
                .send().await?;
            if !res.status().is_success() {
                anyhow::bail!("{}", res.text().await?);
            }
            println!("Updated env for {app}");
        }
        EnvCmd::List { app } => {
            let app = resolve_app(app)?;
            let res = client.get(format!("{base}/api/apps/{app}/env")).send().await?;
            let vars: serde_json::Value = res.json().await?;
            if let Some(obj) = vars.as_object() {
                if obj.is_empty() {
                    println!("No env vars for {app}");
                } else {
                    for (k, _) in obj {
                        println!("{k}=***");
                    }
                }
            }
        }
        EnvCmd::Unset { app, key } => {
            let app = resolve_app(app)?;
            let res = client
                .delete(format!("{base}/api/apps/{app}/env/{key}"))
                .send().await?;
            if !res.status().is_success() {
                anyhow::bail!("{}", res.text().await?);
            }
            println!("Unset {key} for {app}");
        }
    }
    Ok(())
}

fn resolve_app(explicit: Option<String>) -> Result<String> {
    explicit
        .or_else(crate::cli::detect_app_name)
        .ok_or_else(|| anyhow::anyhow!(
            "app name required (or run from inside a remo app directory)"
        ))
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
