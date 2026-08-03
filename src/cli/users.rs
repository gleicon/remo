use anyhow::Result;
use clap::Subcommand;

use crate::config::ClientConfig;

#[derive(Subcommand)]
pub enum UsersCmd {
    /// Add a user and print their token (admin only)
    Add(AddArgs),
    /// Create a single-use invite link for a new user
    Invite(InviteArgs),
    /// List users
    List,
    /// List pending/used invites
    Invites,
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

#[derive(clap::Args)]
pub struct InviteArgs {
    /// Username to pre-assign
    pub name: String,
    /// Email address (stored for reference, not verified)
    #[arg(long)]
    pub email: Option<String>,
    /// Expiry in seconds (default 3600 = 1 hour)
    #[arg(long, default_value_t = 3600)]
    pub expires: u64,
}

pub async fn run(cmd: UsersCmd) -> Result<()> {
    let cfg = ClientConfig::load()?;
    let client = api_client(&cfg);
    let base = &cfg.server_url;

    match cmd {
        UsersCmd::Invite(args) => {
            let res = client
                .post(format!("{base}/api/admin/invites"))
                .json(&serde_json::json!({
                    "username": args.name,
                    "email": args.email,
                    "expires_in_secs": args.expires,
                }))
                .send()
                .await?;
            if !res.status().is_success() {
                anyhow::bail!("{}", res.text().await?);
            }
            let body: serde_json::Value = res.json().await?;
            let cmd = body["claim_command"].as_str().unwrap_or("(error)");
            let exp = body["expires_at"].as_str().unwrap_or("?");
            println!("Invite created for '{}' (expires {})", args.name, exp);
            println!();
            println!("Send this command to the user (shown once):");
            println!("  {cmd}");
            if let Some(ref email) = args.email {
                println!();
                println!("Email: {email}");
            }
        }
        UsersCmd::Invites => {
            let res = client.get(format!("{base}/api/admin/invites")).send().await?;
            if !res.status().is_success() {
                anyhow::bail!("{}", res.text().await?);
            }
            let invites: Vec<serde_json::Value> = res.json().await?;
            if invites.is_empty() {
                println!("No invites.");
            }
            for i in &invites {
                println!(
                    "{:<20} email={:<30} expires={} used={}",
                    i["username"].as_str().unwrap_or("?"),
                    i["email"].as_str().unwrap_or("-"),
                    i["expires_at"].as_str().unwrap_or("?"),
                    i["used"].as_bool().unwrap_or(false),
                );
            }
        }
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
