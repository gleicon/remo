use anyhow::{bail, Context, Result};
use std::os::unix::process::CommandExt;
use std::process::Command;

#[derive(clap::Args)]
pub struct GitHookArgs {
    /// Username as set in the SSH authorized_keys forced command
    #[arg(long)]
    pub user: String,

    /// Deploy mode: called by post-receive hook with app + sha
    #[arg(long)]
    pub deploy: Option<String>,

    #[arg(long)]
    pub sha: Option<String>,
}

pub async fn run(args: GitHookArgs) -> Result<()> {
    if let Some(app_name) = args.deploy {
        // Post-receive deploy path.
        let sha = args.sha.ok_or_else(|| anyhow::anyhow!("--sha is required for --deploy"))?;
        return run_deploy(app_name, sha, args.user).await;
    }

    // SSH forced-command path: auth and exec git-receive-pack.
    let ssh_cmd = std::env::var("SSH_ORIGINAL_COMMAND")
        .context("SSH_ORIGINAL_COMMAND not set — not called via SSH")?;

    let app_name = parse_app_name(&ssh_cmd)?;
    let data_dir = server_data_dir()?;

    // Verify user owns this app.
    let db_path = format!("{data_dir}/state.db");
    let pool = crate::db::open(&db_path).await?;
    let app = crate::db::app_get(&pool, &app_name).await?;

    match app {
        None => bail!("app '{app_name}' not found"),
        Some(a) if a.owner != args.user => bail!("access denied: {app_name}"),
        Some(_) => {}
    }

    let git_dir = format!("{data_dir}/git/{app_name}.git");
    if !std::path::Path::new(&git_dir).exists() {
        init_bare_repo(&git_dir, &app_name, &data_dir)?;
    }

    // Exec git-receive-pack — replace this process so git's exit code flows through.
    // REMO_USER must be explicitly passed to the child process because SSH forced-command
    // replaces the parent env; post-receive hook reads it to identify the deploying user.
    Err(Command::new("git-receive-pack")
        .arg(&git_dir)
        .env("REMO_USER", &args.user)
        .env("REMO_APP", &app_name)
        .exec()
        .into())
}

async fn run_deploy(app_name: String, sha: String, deployer: String) -> Result<()> {
    // Validate sha is a 40-char hex string before passing to git as an argument.
    if sha.len() != 40 || !sha.chars().all(|c| c.is_ascii_hexdigit()) {
        anyhow::bail!("invalid sha '{}': expected 40 hex chars", sha);
    }

    let data_dir = server_data_dir()?;
    let db_path = format!("{data_dir}/state.db");
    let pool = crate::db::open(&db_path).await?;

    let app = crate::db::app_get(&pool, &app_name)
        .await?
        .ok_or_else(|| anyhow::anyhow!("app '{app_name}' not found"))?;

    let server_cfg = crate::config::ServerConfig::load()?;
    let nano_client = crate::nano_client::NanoClient::from_config(&server_cfg);

    // Export the committed tree as a tar archive.
    let git_dir = format!("{data_dir}/git/{app_name}.git");
    let tar = Command::new("git")
        .args(["--git-dir", &git_dir, "archive", "--format=tar", &sha])
        .output()
        .context("git archive failed")?;

    if !tar.status.success() {
        bail!(
            "git archive failed: {}",
            String::from_utf8_lossy(&tar.stderr)
        );
    }

    let env_vars = crate::db::env_list_decoded(&pool, &app_name).await?;
    let ctx = crate::deploy::DeployContext {
        app_name: app_name.clone(),
        app_type: app.app_type.clone(),
        entrypoint: app.entrypoint.clone(),
        data_dir: std::path::PathBuf::from(&data_dir),
        hostname: app.hostname.clone(),
        deployer: deployer.clone(),
        env_vars,
        nano_client,
    };

    let deploy_id = uuid::Uuid::new_v4().to_string();
    crate::db::deployment_create(&pool, &crate::db::Deployment {
        id: deploy_id.clone(),
        app_id: app_name.clone(),
        deployer: deployer.clone(),
        sha: None,
        status: "pending".into(),
        log_output: None,
        created_at: chrono::Utc::now(),
    }).await?;

    match crate::deploy::run(&ctx, tar.stdout).await {
        Ok(deployed_sha) => {
            crate::db::app_set_sha(&pool, &app_name, &deployed_sha).await?;
            crate::db::deployment_update_status(
                &pool, &deploy_id, "success", Some(&deployed_sha), None,
            ).await?;
            println!("Deployed {app_name} @ {deployed_sha}");
        }
        Err(e) => {
            crate::db::deployment_update_status(
                &pool, &deploy_id, "failed", None, Some(&e.to_string()),
            ).await?;
            return Err(e);
        }
    }

    Ok(())
}

fn parse_app_name(ssh_cmd: &str) -> Result<String> {
    // SSH_ORIGINAL_COMMAND = "git-receive-pack 'myapp'"
    let parts: Vec<&str> = ssh_cmd.split_whitespace().collect();
    let raw = parts
        .get(1)
        .ok_or_else(|| anyhow::anyhow!("unexpected SSH_ORIGINAL_COMMAND: {ssh_cmd}"))?;
    let name = raw.trim_matches('\'').trim_matches('"').trim_matches('/');

    if !crate::validation::is_valid_app_name(name) {
        bail!("invalid app name in SSH command");
    }
    Ok(name.to_string())
}

fn server_data_dir() -> Result<String> {
    Ok(crate::config::ServerConfig::load()?.data_dir)
}

fn init_bare_repo(git_dir: &str, app_name: &str, _data_dir: &str) -> Result<()> {
    let status = Command::new("git")
        .args(["init", "--bare", git_dir])
        .stdout(std::process::Stdio::null())
        .stderr(std::process::Stdio::null())
        .status()
        .context("git init --bare")?;
    if !status.success() {
        bail!("git init --bare failed for {app_name}");
    }

    // Hook is fully static — no user-controlled data interpolated.
    // App name is derived server-side from $GIT_DIR basename, never from the push.
    // Use the absolute path of the current executable so the git user's restricted
    // PATH doesn't have to include /usr/local/bin.
    let remo_bin = std::env::current_exe()
        .unwrap_or_else(|_| std::path::PathBuf::from("/usr/local/bin/remo"))
        .to_string_lossy()
        .into_owned();
    let hook_path = format!("{git_dir}/hooks/post-receive");
    let hook = format!(
        "#!/bin/sh\n\
         # remo post-receive — DO NOT EDIT (managed by remo)\n\
         # REMO_APP is set by remo git-hook before exec'ing git-receive-pack.\n\
         # GIT_DIR is '.' inside hooks so we cannot derive the app name from it.\n\
         while read oldrev newrev ref; do\n\
             {remo_bin} git-hook --user \"${{REMO_USER}}\" --deploy \"${{REMO_APP}}\" --sha \"${{newrev}}\"\n\
         done\n"
    );
    std::fs::write(&hook_path, hook.as_bytes())?;
    use std::os::unix::fs::PermissionsExt;
    std::fs::set_permissions(&hook_path, std::fs::Permissions::from_mode(0o755))?;

    Ok(())
}
