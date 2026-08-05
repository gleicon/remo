use anyhow::{bail, Result};

pub async fn run() -> Result<()> {
    let app = crate::cli::detect_app_name()
        .ok_or_else(|| anyhow::anyhow!(
            "not inside a remo app directory (no git remote named 'remo' found)"
        ))?;

    let status = std::process::Command::new("git")
        .args(["push", "remo", "main"])
        .status()?;

    if !status.success() {
        bail!("git push failed");
    }

    // Print the current HEAD sha so the user sees what was deployed.
    if let Ok(out) = std::process::Command::new("git")
        .args(["rev-parse", "--short", "HEAD"])
        .output()
    {
        let sha = String::from_utf8_lossy(&out.stdout).trim().to_string();
        println!("Deployed {app} @ {sha}");
    }

    Ok(())
}
