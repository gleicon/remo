use anyhow::{bail, Context, Result};
use std::io::{self, BufRead as _, Write as _};
use std::path::PathBuf;

#[derive(clap::Args)]
pub struct SetupArgs {
    /// Claim a single-use invite token (sent by your admin)
    #[arg(long)]
    pub invite: Option<String>,
}

pub async fn run(args: SetupArgs) -> Result<()> {
    if let Some(raw) = args.invite {
        // Accept either a bare token or a full URL (.../invite/<token>)
        let token = if raw.starts_with("http://") || raw.starts_with("https://") {
            raw.trim_end_matches('/').split('/').last().unwrap_or(&raw).to_string()
        } else {
            raw
        };
        run_invite(token).await
    } else {
        run_interactive().await
    }
}

// ── Invite claim (user-facing) ────────────────────────────────────────────────

async fn run_invite(token: String) -> Result<()> {
    let server_url = prompt("Server URL (e.g. https://remo.yourdomain.tld)")?;

    // Validate invite before generating SSH key
    let client = reqwest::Client::new();
    let check = client
        .get(format!("{server_url}/health"))
        .send()
        .await
        .with_context(|| format!("cannot reach {server_url}"))?;
    if !check.status().is_success() {
        bail!("server returned {}", check.status());
    }
    println!("Connected to {server_url}");

    let ssh_pub = pick_or_generate_key()?;

    println!("Claiming invite...");
    let res = client
        .post(format!("{server_url}/api/invites/{token}/claim"))
        .json(&serde_json::json!({ "ssh_pubkey": ssh_pub }))
        .send()
        .await
        .context("claim invite")?;

    if !res.status().is_success() {
        bail!("claim failed: {}", res.text().await?);
    }

    let body: serde_json::Value = res.json().await?;
    let username = body["username"].as_str().context("no username in response")?;
    let user_token = body["token"].as_str().context("no token in response")?;

    save_config(&server_url, user_token)?;
    write_ssh_config_entry(&server_url)?;

    println!();
    println!("Setup complete.");
    println!("  Username : {username}");
    println!("  Server   : {server_url}");
    println!("  Config   : {}", crate::config::ClientConfig::path()?.display());
    println!();
    let host = host_from_url(&server_url);
    println!("To deploy:");
    println!("  remo apps create <appname>");
    println!("  git remote add remo ssh://{host}/<appname>");
    println!("  git push remo main");

    Ok(())
}

// ── Interactive setup (admin bootstrap or user-with-token) ────────────────────

async fn run_interactive() -> Result<()> {
    println!("remo setup\n");

    let server_url = prompt("Server URL (e.g. https://remo.yourdomain.tld)")?;

    println!("How are you setting up?");
    println!("  1  Admin — I have a master token (first-time server setup)");
    println!("  2  User  — I have a user token from my admin");
    let choice = prompt("Choice [1/2]")?;

    match choice.trim() {
        "1" => run_admin_bootstrap(server_url).await,
        "2" => run_user_token(server_url).await,
        _ => bail!("enter 1 or 2"),
    }
}

async fn run_admin_bootstrap(server_url: String) -> Result<()> {
    let master_token = prompt_password("Master token")?;

    validate_connection(&server_url, &master_token).await?;
    println!("Connected.");

    let username = prompt("Your username")?;

    let ssh_pub = pick_or_generate_key()?;

    let client = authed_client(&master_token);

    // If the user already exists (e.g. re-running setup after a master token rotation),
    // skip creation and use the existing token already in the local config.
    let create_res = client
        .post(format!("{server_url}/api/users"))
        .json(&serde_json::json!({
            "name": username,
            "ssh_pubkey": ssh_pub,
        }))
        .send()
        .await
        .context("create user")?;

    let user_token: String = if create_res.status().as_u16() == 409 {
        // User already exists — load existing local token.
        println!("User '{username}' already exists. Re-using existing token.");
        crate::config::ClientConfig::load()
            .map(|c| c.token)
            .unwrap_or_default()
    } else if create_res.status().is_success() {
        let body: serde_json::Value = create_res.json().await?;
        body["token"].as_str().context("no token in response")?.to_string()
    } else {
        bail!("create user failed: {}", create_res.text().await?);
    };

    if user_token.is_empty() {
        bail!("user already exists but no local token found — use option 2 (User token) instead");
    }

    save_config(&server_url, &user_token)?;
    write_ssh_config_entry(&server_url)?;

    println!();
    println!("Setup complete.");
    println!("  Username : {username}");
    println!("  Server   : {server_url}");
    println!("  Config   : {}", crate::config::ClientConfig::path()?.display());

    Ok(())
}

async fn run_user_token(server_url: String) -> Result<()> {
    let user_token = prompt_password("User token (from your admin)")?;

    validate_connection(&server_url, &user_token).await?;
    println!("Connected.");

    let ssh_pub = pick_or_generate_key()?;

    save_config(&server_url, &user_token)?;
    write_ssh_config_entry(&server_url)?;

    println!();
    println!("Client configured.");
    println!("  Config : {}", crate::config::ClientConfig::path()?.display());
    println!();
    println!("Your SSH public key (send to your admin to register):");
    println!("  {ssh_pub}");
    println!();
    println!("Admin runs on the server:");
    println!("  remo server add-key --user <your-username> --key \"{ssh_pub}\"");

    Ok(())
}

// ── Helpers ───────────────────────────────────────────────────────────────────

fn pick_or_generate_key() -> Result<String> {
    let home = dirs::home_dir().context("no home dir")?;
    let ssh = home.join(".ssh");

    let candidates: Vec<(&str, PathBuf)> = [
        "id_rsa_remo.pub",
        "id_ed25519.pub",
        "id_ecdsa.pub",
        "id_rsa.pub",
    ]
    .iter()
    .filter_map(|name| {
        let p = ssh.join(name);
        if p.exists() { Some((*name, p)) } else { None }
    })
    .collect();

    if !candidates.is_empty() {
        println!("Found SSH public keys:");
        for (i, (name, _)) in candidates.iter().enumerate() {
            println!("  {} ~/.ssh/{}", i + 1, name);
        }
        println!("  {} Generate new key at ~/.ssh/id_rsa_remo", candidates.len() + 1);
        println!("  {} Paste a key manually", candidates.len() + 2);

        let choice = prompt("Choose")?;
        let n: usize = choice.trim().parse().unwrap_or(0);

        if n >= 1 && n <= candidates.len() {
            let key = std::fs::read_to_string(&candidates[n - 1].1)?.trim().to_string();
            return Ok(key);
        }
        if n == candidates.len() + 1 {
            return generate_key(&ssh);
        }
        // fall through to paste
        let key = prompt("Paste public key")?;
        return Ok(key.trim().to_string());
    }

    println!("No SSH keys found.");
    println!("  1  Generate new key at ~/.ssh/id_rsa_remo");
    println!("  2  Paste a key manually");
    let choice = prompt("Choose [1/2]")?;
    match choice.trim() {
        "1" => generate_key(&ssh),
        _ => {
            let key = prompt("Paste public key")?;
            Ok(key.trim().to_string())
        }
    }
}

fn generate_key(ssh_dir: &std::path::Path) -> Result<String> {
    let key_path = ssh_dir.join("id_rsa_remo");
    let status = std::process::Command::new("ssh-keygen")
        .args([
            "-t", "ed25519",
            "-f", key_path.to_str().unwrap(),
            "-C", "remo-deploy",
            "-N", "",
        ])
        .status()
        .context("ssh-keygen not found")?;
    if !status.success() {
        bail!("ssh-keygen failed");
    }
    let pub_path = ssh_dir.join("id_rsa_remo.pub");
    let key = std::fs::read_to_string(&pub_path)?.trim().to_string();
    println!("Generated {}", pub_path.display());
    Ok(key)
}

fn save_config(server_url: &str, token: &str) -> Result<()> {
    let cfg = crate::config::ClientConfig {
        server_url: server_url.trim_end_matches('/').to_string(),
        token: token.to_string(),
    };
    cfg.save()?;
    println!("Config saved to {}", crate::config::ClientConfig::path()?.display());
    Ok(())
}

fn write_ssh_config_entry(server_url: &str) -> Result<()> {
    let host = host_from_url(server_url);
    let home = dirs::home_dir().context("no home dir")?;
    let config_path = home.join(".ssh").join("config");

    let existing = if config_path.exists() {
        std::fs::read_to_string(&config_path)?
    } else {
        String::new()
    };

    if existing.contains(&format!("Host {host}")) {
        println!("~/.ssh/config already has entry for {host}");
        return Ok(());
    }

    let stanza = format!(
        "\nHost {host}\n    User git\n    IdentityFile ~/.ssh/id_rsa_remo\n    IdentitiesOnly yes\n"
    );
    std::fs::write(&config_path, format!("{existing}{stanza}"))?;
    println!("Added ~/.ssh/config entry for {host}");
    Ok(())
}

fn host_from_url(url: &str) -> String {
    url.trim_start_matches("https://")
        .trim_start_matches("http://")
        .split('/')
        .next()
        .unwrap_or(url)
        .to_string()
}

async fn validate_connection(server_url: &str, token: &str) -> Result<()> {
    let res = authed_client(token)
        .get(format!("{server_url}/health"))
        .send()
        .await
        .with_context(|| format!("cannot reach {server_url}"))?;
    if !res.status().is_success() {
        bail!("server returned {}: check token", res.status());
    }
    Ok(())
}

fn authed_client(token: &str) -> reqwest::Client {
    use reqwest::header::{HeaderMap, HeaderValue, AUTHORIZATION};
    let mut headers = HeaderMap::new();
    headers.insert(
        AUTHORIZATION,
        HeaderValue::from_str(&format!("Bearer {token}")).unwrap(),
    );
    reqwest::Client::builder()
        .default_headers(headers)
        .build()
        .unwrap()
}

fn prompt(label: &str) -> Result<String> {
    print!("{label}: ");
    io::stdout().flush()?;
    let mut buf = String::new();
    io::stdin().lock().read_line(&mut buf)?;
    Ok(buf.trim().to_string())
}

fn prompt_default(label: &str, default: &str) -> Result<String> {
    print!("{label} [{default}]: ");
    io::stdout().flush()?;
    let mut buf = String::new();
    io::stdin().lock().read_line(&mut buf)?;
    let s = buf.trim().to_string();
    Ok(if s.is_empty() { default.to_string() } else { s })
}

fn prompt_password(label: &str) -> Result<String> {
    rpassword::prompt_password(format!("{label}: "))
        .context("failed to read password")
}
