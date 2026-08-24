pub mod apps;
pub mod deploy;
pub mod env;
pub mod git_hook;
pub mod login;
pub mod logs;
pub mod push;
pub mod server;
pub mod setup;
pub mod users;

/// Detect the current app name from the git remote named "remo".
/// Parses both `ssh://host/appname` and `git@host:appname`.
pub fn detect_app_name() -> Option<String> {
    let out = std::process::Command::new("git")
        .args(["remote", "get-url", "remo"])
        .output()
        .ok()?;
    if !out.status.success() { return None; }
    let url = String::from_utf8_lossy(&out.stdout).trim().to_string();
    // ssh://host/appname  →  last path segment
    // git@host:appname    →  after ':'
    let name = if url.contains("://") {
        url.split('/').last()?.to_string()
    } else {
        url.split(':').last()?.to_string()
    };
    let name = name.trim().to_string();
    if name.is_empty() { None } else { Some(name) }
}
