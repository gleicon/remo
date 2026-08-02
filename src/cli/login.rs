use anyhow::Result;
use crate::config::ClientConfig;

#[derive(clap::Args)]
pub struct LoginArgs {
    /// remo server URL
    #[arg(long)]
    pub server: String,

    /// Auth token (admin or user token)
    #[arg(long)]
    pub token: String,
}

pub async fn run(args: LoginArgs) -> Result<()> {
    // Validate the token by hitting the server health endpoint.
    let client = reqwest::Client::new();
    let res = client
        .get(format!("{}/health", args.server.trim_end_matches('/')))
        .bearer_auth(&args.token)
        .send()
        .await;

    match res {
        Ok(r) if r.status().is_success() => {}
        Ok(r) => anyhow::bail!("server returned {}: check token", r.status()),
        Err(e) => anyhow::bail!("cannot reach {}: {e}", args.server),
    }

    let cfg = ClientConfig {
        server_url: args.server.trim_end_matches('/').to_string(),
        token: args.token,
    };
    cfg.save()?;

    let path = ClientConfig::path()?;
    println!("Logged in. Config saved to {}", path.display());
    Ok(())
}
