use anyhow::Result;
use clap::Subcommand;

use crate::config::ClientConfig;

#[derive(Subcommand)]
pub enum UsersCmd {
    /// Add a user and print their token (admin only)
    Add(AddArgs),
    /// List users
    List,
    /// Remove a user
    Remove { name: String },
}

#[derive(clap::Args)]
pub struct AddArgs {
    pub name: String,

    /// SSH public key for git push auth
    #[arg(long)]
    pub pubkey: Option<String>,
}

pub async fn run(cmd: UsersCmd) -> Result<()> {
    let cfg = ClientConfig::load()?;
    let client = api_client(&cfg);
    let base = &cfg.server_url;

    match cmd {
        UsersCmd::Add(args) => {
            let res = client
                .post(format!("{base}/api/users"))
                .json(&serde_json::json!({
                    "name": args.name,
                    "ssh_pubkey": args.pubkey,
                }))
                .send()
                .await?;
            if !res.status().is_success() {
                anyhow::bail!("{}", res.text().await?);
            }
            let body: serde_json::Value = res.json().await?;
            let token = body["token"].as_str().unwrap_or("(error)");
            println!("User {} created.", args.name);
            println!("Token (shown once): {token}");
            if let Some(ref key) = args.pubkey {
                println!();
                println!("Add to /etc/remo/authorized_keys on the server:");
                println!("  command=\"remo git-hook --user {}\" {}", args.name, key);
            }
        }
        UsersCmd::List => {
            let res = client.get(format!("{base}/api/users")).send().await?;
            let users: Vec<serde_json::Value> = res.json().await?;
            for u in &users {
                println!(
                    "{:<20} admin={}",
                    u["name"].as_str().unwrap_or("?"),
                    u["is_admin"].as_bool().unwrap_or(false)
                );
            }
        }
        UsersCmd::Remove { name } => {
            let res = client.delete(format!("{base}/api/users/{name}")).send().await?;
            if !res.status().is_success() {
                anyhow::bail!("{}", res.text().await?);
            }
            println!("Removed {name}");
        }
    }
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
