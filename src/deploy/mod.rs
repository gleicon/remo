use anyhow::{Context, Result};
use sha2::{Digest, Sha256};
use std::path::{Path, PathBuf};

use crate::nano_client::{CreateAppRequest, NanoClient, UpdateAppRequest};

pub const KEEP_DEPLOYS: usize = 5;
/// Maximum deploy archive size. Recompile to change for your VPS class.
pub const MAX_DEPLOY_BYTES: usize = 10 * 1024 * 1024; // 10 MB

#[derive(Debug, Clone)]
pub struct DeployContext {
    pub app_name: String,
    pub app_type: String,
    pub entrypoint: String,
    pub data_dir: PathBuf,
    pub hostname: String,
    pub deployer: String,
    pub env_vars: std::collections::HashMap<String, String>,
    pub nano_client: NanoClient,
}

/// Extract a tar archive from the git archive pipe into a content-addressed
/// directory, swap the `current/` symlink, and hot-reload nano-rs.
pub async fn run(ctx: &DeployContext, tar_bytes: Vec<u8>) -> Result<String> {
    anyhow::ensure!(
        tar_bytes.len() <= MAX_DEPLOY_BYTES,
        "deploy archive too large: {} bytes (limit {} bytes / {} MB)",
        tar_bytes.len(),
        MAX_DEPLOY_BYTES,
        MAX_DEPLOY_BYTES / 1024 / 1024,
    );

    let sha = content_sha(&tar_bytes);
    let deploy_dir = ctx.data_dir.join("apps").join(&ctx.app_name).join("deploys").join(&sha);

    if !deploy_dir.exists() {
        std::fs::create_dir_all(&deploy_dir)?;
        extract_tar(&tar_bytes, &deploy_dir).context("extract deploy archive")?;
    }

    swap_symlink(&deploy_dir, &ctx.app_name, &ctx.data_dir)?;

    let current_dir = ctx.data_dir.join("apps").join(&ctx.app_name).join("current");
    let entrypoint_path = current_dir.join(&ctx.entrypoint);

    // Reload before pruning: prune may delete the active deploy dir if the same
    // content-addressed SHA was deployed previously (its mtime is older).
    // Registration failure is non-fatal: the file deploy (extract + symlink) is durable;
    // nano-rs is ephemeral. The sync loop re-registers on next nano-rs recovery.
    if let Err(e) = reload_nano(ctx, &entrypoint_path).await {
        tracing::warn!("nano-rs registration failed (sync loop will recover): {e}");
    }
    prune_old_deploys(&ctx.app_name, &ctx.data_dir).await?;

    Ok(sha)
}

fn content_sha(bytes: &[u8]) -> String {
    let mut hasher = Sha256::new();
    hasher.update(bytes);
    hex::encode(hasher.finalize())[..16].to_string()
}

fn extract_tar(bytes: &[u8], dest: &Path) -> Result<()> {
    let mut archive = tar::Archive::new(bytes);
    archive.unpack(dest)?;
    Ok(())
}

fn swap_symlink(deploy_dir: &Path, app_name: &str, data_dir: &Path) -> Result<()> {
    let link_path = data_dir.join("apps").join(app_name).join("current");
    let tmp = data_dir.join("apps").join(app_name).join(".current.tmp");

    // Atomic symlink swap: write tmp, then rename.
    if tmp.exists() {
        std::fs::remove_file(&tmp)?;
    }
    std::os::unix::fs::symlink(deploy_dir, &tmp)?;
    std::fs::rename(&tmp, &link_path)?;
    Ok(())
}

async fn prune_old_deploys(app_name: &str, data_dir: &Path) -> Result<()> {
    let deploys_dir = data_dir.join("apps").join(app_name).join("deploys");
    let mut entries: Vec<_> = std::fs::read_dir(&deploys_dir)?
        .filter_map(|e| e.ok())
        .collect();
    // sort_by_cached_key: calls metadata() once per entry (not O(N log N) syscalls).
    // Entries with unreadable mtime use UNIX_EPOCH so they sort as oldest (correct behavior).
    entries.sort_by_cached_key(|e| {
        e.metadata()
            .and_then(|m| m.modified())
            .unwrap_or(std::time::SystemTime::UNIX_EPOCH)
    });

    if entries.len() > KEEP_DEPLOYS {
        for old in entries.iter().take(entries.len() - KEEP_DEPLOYS) {
            let _ = std::fs::remove_dir_all(old.path());
        }
    }
    Ok(())
}

async fn reload_nano(ctx: &DeployContext, entrypoint: &Path) -> Result<()> {
    let entrypoint_str = entrypoint.to_string_lossy().into_owned();
    let env: std::collections::HashMap<_, _> = ctx.env_vars.clone();

    let compat = if ctx.app_type == "gas" { Some("gas".to_string()) } else { None };

    // Try reload (app already registered in nano-rs); fall back to create on 404 (first deploy).
    let update = ctx
        .nano_client
        .update_app(
            &ctx.hostname,
            &UpdateAppRequest {
                entrypoint: Some(entrypoint_str.clone()),
                env_vars: if env.is_empty() { None } else { Some(env.clone()) },
                limits: None,
                compat: compat.clone(),
            },
        )
        .await;

    match update {
        Ok(()) => {}
        Err(e) if crate::nano_client::is_not_found(&e) => {
            ctx.nano_client
                .create_app(&CreateAppRequest {
                    hostname: ctx.hostname.clone(),
                    entrypoint: entrypoint_str,
                    env_vars: env,
                    limits: crate::nano_client::AppLimits { workers: 2, ..Default::default() },
                    activate: true,
                    compat,
                })
                .await?;
        }
        Err(e) => return Err(e),
    }

    ctx.nano_client.reload_app(&ctx.hostname).await?;
    Ok(())
}
