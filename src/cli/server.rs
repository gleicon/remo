use anyhow::{bail, Result};
use clap::Subcommand;

use crate::config::{ProxyBackend, ServerConfig};
use crate::server;

#[derive(Subcommand)]
pub enum ServerCmd {
    /// Install remo on this VPS (run as root)
    Install(InstallArgs),
    /// Start the remo control plane
    Start(StartArgs),
    /// Print server status
    Status,
    /// Assess VPS readiness and remo installation health
    Doctor,
}

#[derive(clap::Args)]
pub struct InstallArgs {
    /// Base domain (e.g. apps.yourdomain.tld)
    #[arg(long)]
    pub domain: String,

    /// Reverse proxy backend
    #[arg(long, default_value = "nginx")]
    pub proxy: ProxyBackend,

    /// nano-rs admin socket or URL
    #[arg(long, default_value = "http://127.0.0.1:8889")]
    pub nano_url: String,

    /// Docker-compose mode: writes bind_addr=0.0.0.0, nano_socket=http://nano-rs:8889,
    /// and a .env file with NANO_ADMIN_API_KEY. Skips useradd and nginx config.
    #[arg(long)]
    pub docker: bool,

    /// Force re-init even if already installed
    #[arg(long)]
    pub reinit: bool,
}

#[derive(clap::Args)]
pub struct StartArgs {
    /// Port to listen on
    #[arg(long, default_value_t = 7070)]
    pub port: u16,
}

pub async fn run(cmd: ServerCmd) -> Result<()> {
    match cmd {
        ServerCmd::Install(args) => install(args).await,
        ServerCmd::Start(args) => server::start(args.port).await,
        ServerCmd::Status => status().await,
        ServerCmd::Doctor => doctor().await,
    }
}

async fn install(args: InstallArgs) -> Result<()> {
    if std::env::var("USER").unwrap_or_default() != "root" {
        bail!("`remo server install` must run as root");
    }

    let cfg_path = ServerConfig::system_path();
    if cfg_path.exists() && !args.reinit {
        bail!(
            "already installed ({}). Re-run with --reinit to overwrite.",
            cfg_path.display()
        );
    }

    let docker = args.docker;
    println!("Installing remo on {} ({})", args.domain, if docker { "docker mode" } else { "bare VPS mode" });

    use std::os::unix::fs::DirBuilderExt;
    use std::os::unix::fs::PermissionsExt;

    // /etc/remo is 0o700 so the master_token is never readable by other accounts.
    std::fs::DirBuilder::new().recursive(true).mode(0o700).create("/etc/remo")?;
    std::fs::set_permissions("/etc/remo", std::fs::Permissions::from_mode(0o700))?;
    println!("  created /etc/remo");

    if !docker {
        // Bare VPS: create data directories, git user, chown.
        let data_dir = "/var/lib/remo";
        for dir in &[
            format!("{data_dir}/apps"),
            format!("{data_dir}/git"),
            "/var/log/remo".to_string(),
        ] {
            std::fs::create_dir_all(dir)?;
            println!("  created {dir}");
        }

        // useradd exits 9 if git user already exists.
        let ua = std::process::Command::new("useradd")
            .args(["--system", "--shell", "/usr/sbin/nologin", "--no-create-home", "git"])
            .output();
        match ua {
            Ok(o) if o.status.success() => println!("  created git system user"),
            Ok(o) if o.status.code() == Some(9) => println!("  git system user already exists"),
            Ok(o) => bail!("useradd failed: {}", String::from_utf8_lossy(&o.stderr)),
            Err(e) => bail!("could not run useradd: {e}"),
        }

        for dir in &[format!("{data_dir}/git"), format!("{data_dir}/apps")] {
            let ok = std::process::Command::new("chown")
                .args(["git", dir.as_str()])
                .status()
                .map(|s| s.success())
                .unwrap_or(false);
            if !ok { bail!("chown git {dir} failed"); }
            println!("  chown git {dir}");
        }
    }

    // authorized_keys: Docker reads it via bind-mount; bare VPS needs it for sshd.
    let ak_path = "/etc/remo/authorized_keys";
    if !std::path::Path::new(ak_path).exists() {
        {
            use std::io::Write;
            use std::os::unix::fs::OpenOptionsExt;
            let mut f = std::fs::OpenOptions::new()
                .write(true).create(true).mode(0o600)
                .open(ak_path)?;
            f.write_all(b"")?;
        }
        if !docker {
            let _ = std::process::Command::new("chown").args(["git", ak_path]).status();
        }
        println!("  created {ak_path}");
    }

    // Write master_token atomically at 0o600.
    let master_token = generate_token();
    {
        use std::io::Write;
        use std::os::unix::fs::OpenOptionsExt;
        let mut f = std::fs::OpenOptions::new()
            .write(true).create(true).truncate(true).mode(0o600)
            .open("/etc/remo/master_token")?;
        f.write_all(master_token.as_bytes())?;
    }
    println!("  wrote /etc/remo/master_token");

    // Generate nano-rs admin key.
    let nano_admin_key = generate_token();

    let (nano_socket, bind_addr) = if docker {
        ("http://nano-rs:8889".to_string(), "0.0.0.0".to_string())
    } else {
        (args.nano_url.clone(), "127.0.0.1".to_string())
    };

    let cfg = ServerConfig {
        domain: args.domain.clone(),
        data_dir: "/var/lib/remo".to_string(),
        nano_socket,
        nano_admin_key: Some(nano_admin_key.clone()),
        proxy: args.proxy.clone(),
        control_port: 7070,
        bind_addr,
    };
    cfg.save()?;
    println!("  wrote {}", ServerConfig::system_path().display());

    // Initialize SQLite.
    let db_path = "/var/lib/remo/state.db";
    if std::path::Path::new(db_path).exists() {
        println!("  skipped DB init (already exists)");
    } else {
        // In Docker mode the data dir is a volume — skip DB init here,
        // remo-server creates it on first start.
        if !docker {
            let pool = crate::db::open(db_path).await?;
            drop(pool);
            println!("  initialized {db_path}");
        }
    }

    if docker {
        // Write .env file for docker compose in the current directory.
        let env_path = ".env";
        {
            use std::io::Write;
            use std::os::unix::fs::OpenOptionsExt;
            let mut f = std::fs::OpenOptions::new()
                .write(true).create(true).truncate(true).mode(0o600)
                .open(env_path)?;
            writeln!(f, "NANO_ADMIN_API_KEY={nano_admin_key}")?;
        }
        println!("  wrote {env_path} (NANO_ADMIN_API_KEY)");
    } else {
        write_proxy_config(&args.domain, &args.proxy)?;
    }

    println!();
    println!("Installation complete.");
    println!();
    println!("Master token (save this — shown once):");
    println!("  {master_token}");
    println!();
    if docker {
        println!("NANO_ADMIN_API_KEY written to .env — load with: docker compose up -d");
        println!();
        println!("Add your SSH public key to /etc/remo/authorized_keys:");
        println!("  echo 'command=\"remo git-hook --user <name>\" ssh-ed25519 AAA...' >> /etc/remo/authorized_keys");
        println!();
        println!("Configure nginx on the host:");
        println!("  *.{domain}  → 127.0.0.1:8080  (nano-rs apps)", domain = args.domain);
        println!("  {domain}    → 127.0.0.1:7070  (remo control plane)", domain = args.domain);
    } else {
        println!("nano-rs admin key (set NANO_ADMIN_API_KEY in nano-rs environment):");
        println!("  {nano_admin_key}");
        println!();
        println!("Login from your laptop:");
        println!("  remo login --server https://remo.{} --token <token>", args.domain);
        println!();
        println!("DNS: add these records to your domain:");
        println!("  *.{}   A  <this VPS IP>", args.domain);
        println!("  remo.{}  A  <this VPS IP>", args.domain);
    }

    Ok(())
}

fn write_proxy_config(domain: &str, backend: &ProxyBackend) -> Result<()> {
    match backend {
        ProxyBackend::Nginx => {
            // Wildcard vhost: all app traffic → nano-rs data plane.
            let app_path = "/etc/nginx/sites-enabled/remo-wildcard.conf";
            let app_cfg = format!(
                "# remo wildcard — DO NOT EDIT (managed by remo)\n\
                 server {{\n\
                     listen 80;\n\
                     server_name *.{domain};\n\
                     location / {{\n\
                         proxy_pass http://127.0.0.1:8080;\n\
                         proxy_set_header Host $host;\n\
                         proxy_set_header X-Real-IP $remote_addr;\n\
                         proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;\n\
                         proxy_set_header X-Forwarded-Proto $scheme;\n\
                     }}\n\
                 }}\n"
            );
            std::fs::write(app_path, app_cfg)?;
            println!("  wrote {app_path}");

            // Control-plane vhost: remo CLI traffic → remo control plane.
            let ctl_path = "/etc/nginx/sites-enabled/remo-control.conf";
            let ctl_cfg = format!(
                "# remo control plane — DO NOT EDIT (managed by remo)\n\
                 server {{\n\
                     listen 80;\n\
                     server_name remo.{domain};\n\
                     location / {{\n\
                         proxy_pass http://127.0.0.1:7070;\n\
                         proxy_set_header Host $host;\n\
                         proxy_set_header X-Real-IP $remote_addr;\n\
                     }}\n\
                 }}\n"
            );
            std::fs::write(ctl_path, ctl_cfg)?;
            println!("  wrote {ctl_path}");
            println!("  run: systemctl reload nginx");
        }
        ProxyBackend::Caddy => {
            let path = "/etc/caddy/remo.caddyfile";
            let cfg = crate::proxy::caddy::CaddyBackend::static_caddyfile(domain, 8080);
            std::fs::write(path, cfg)?;
            println!("  wrote {path}");
            println!("  add: import /etc/caddy/remo.caddyfile  to /etc/caddy/Caddyfile");
            println!("  run: systemctl reload caddy");
        }
    }
    Ok(())
}

async fn status() -> Result<()> {
    match ServerConfig::load() {
        Ok(cfg) => {
            println!("domain:  {}", cfg.domain);
            println!("data:    {}", cfg.data_dir);
            println!("nano:    {}", cfg.nano_socket);
            println!("proxy:   {:?}", cfg.proxy);
            println!("port:    {}", cfg.control_port);
        }
        Err(e) => println!("not installed: {e}"),
    }
    Ok(())
}

fn generate_token() -> String {
    use rand::Rng;
    let bytes: Vec<u8> = rand::thread_rng().sample_iter(rand::distributions::Standard).take(32).collect();
    hex::encode(bytes)
}

// ── Doctor ────────────────────────────────────────────────────────────────────

#[derive(Debug, PartialEq)]
enum CheckStatus { Ok, Warn, Fail }

struct Check {
    status: CheckStatus,
    detail: String,
}

impl Check {
    fn ok(msg: impl Into<String>) -> Self { Self { status: CheckStatus::Ok,   detail: msg.into() } }
    fn warn(msg: impl Into<String>) -> Self { Self { status: CheckStatus::Warn, detail: msg.into() } }
    fn fail(msg: impl Into<String>) -> Self { Self { status: CheckStatus::Fail, detail: msg.into() } }
}

async fn doctor() -> Result<()> {
    let mut checks: Vec<Check> = Vec::new();

    // ── remo config ───────────────────────────────────────────────────────────

    let cfg = match ServerConfig::load() {
        Ok(c) => {
            checks.push(Check::ok(format!("server config: {}", ServerConfig::system_path().display())));
            Some(c)
        }
        Err(_) => {
            checks.push(Check::warn(format!(
                "server config missing: {} — run `remo server install` first",
                ServerConfig::system_path().display()
            )));
            None
        }
    };

    // ── /etc/remo/ permissions ────────────────────────────────────────────────

    checks.extend(check_path_mode("/etc/remo", 0o700, true));
    checks.extend(check_path_mode("/etc/remo/master_token", 0o600, false));

    if !std::path::Path::new("/etc/remo/authorized_keys").exists() {
        checks.push(Check::warn(
            "/etc/remo/authorized_keys missing — git push auth won't work until SSH keys are added",
        ));
    } else {
        checks.push(Check::ok("/etc/remo/authorized_keys present"));
    }

    // ── /var/lib/remo/ layout ─────────────────────────────────────────────────

    let data_dir = cfg.as_ref().map(|c| c.data_dir.as_str()).unwrap_or("/var/lib/remo");
    for sub in &["apps", "git"] {
        let p = format!("{data_dir}/{sub}");
        if std::path::Path::new(&p).exists() {
            checks.push(Check::ok(format!("{p} exists")));
        } else {
            checks.push(Check::fail(format!("{p} missing")));
        }
    }
    let db = format!("{data_dir}/state.db");
    if std::path::Path::new(&db).exists() {
        checks.push(Check::ok(format!("SQLite DB: {db}")));
    } else {
        checks.push(Check::warn(format!("SQLite DB not found at {db}")));
    }

    // ── nano-rs reachability ──────────────────────────────────────────────────

    let nano_url = cfg.as_ref().map(|c| c.nano_socket.clone())
        .unwrap_or_else(|| "http://127.0.0.1:9000".into());
    checks.push(check_http_reachable(&nano_url, "nano-rs admin").await);
    checks.push(check_http_reachable("http://127.0.0.1:8080", "nano-rs data plane").await);

    // ── remo control plane ────────────────────────────────────────────────────

    let ctl_port = cfg.as_ref().map(|c| c.control_port).unwrap_or(7070);
    checks.push(check_http_reachable(
        &format!("http://127.0.0.1:{ctl_port}/health"),
        "remo control plane",
    ).await);

    // ── proxy ─────────────────────────────────────────────────────────────────

    let proxy = cfg.as_ref().map(|c| &c.proxy);
    match proxy {
        Some(ProxyBackend::Caddy) | None if caddy_installed() => {
            checks.extend(check_caddy());
        }
        _ => {
            checks.extend(check_nginx());
        }
    }

    // ── SSH user ──────────────────────────────────────────────────────────────

    let git_user_ok = std::process::Command::new("id")
        .arg("git")
        .output()
        .map(|o| o.status.success())
        .unwrap_or(false);
    if git_user_ok {
        checks.push(Check::ok("'git' system user exists"));
    } else {
        checks.push(Check::warn(
            "'git' system user not found — create with: adduser --system --shell /usr/sbin/nologin --no-create-home git",
        ));
    }

    // ── sshd authorized_keys config ───────────────────────────────────────────

    let sshd_cfg = std::fs::read_to_string("/etc/ssh/sshd_config").unwrap_or_default();
    let has_ak = sshd_cfg.contains("/etc/remo/authorized_keys");
    let has_match_git = sshd_cfg.contains("Match User git");
    if has_ak && has_match_git {
        checks.push(Check::ok("sshd_config: Match User git with /etc/remo/authorized_keys"));
    } else if has_ak && !has_match_git {
        checks.push(Check::fail(
            "sshd_config references /etc/remo/authorized_keys globally — add 'Match User git' block or global SSH access for other users breaks",
        ));
    } else {
        checks.push(Check::warn(
            "sshd_config missing Match User git + AuthorizedKeysFile /etc/remo/authorized_keys",
        ));
    }

    // ── control-plane nginx vhost ─────────────────────────────────────────────

    let ctl_conf = "/etc/nginx/sites-enabled/remo-control.conf";
    if std::path::Path::new(ctl_conf).exists() {
        checks.push(Check::ok(format!("remo control-plane nginx config present ({ctl_conf})")));
    } else {
        checks.push(Check::warn(format!(
            "remo control-plane nginx config missing ({ctl_conf}) — CLI and git SSH unreachable from outside"
        )));
    }

    // ── print results ─────────────────────────────────────────────────────────

    println!("remo doctor\n");
    let (mut errors, mut warnings) = (0u32, 0u32);
    for c in &checks {
        let tag = match c.status {
            CheckStatus::Ok   => " ok  ",
            CheckStatus::Warn => "warn ",
            CheckStatus::Fail => "FAIL ",
        };
        println!("  [{tag}] {}", c.detail);
        match c.status {
            CheckStatus::Ok   => {}
            CheckStatus::Warn => warnings += 1,
            CheckStatus::Fail => errors += 1,
        }
    }
    println!();
    if errors == 0 && warnings == 0 {
        println!("all checks passed");
    } else {
        println!("summary: {errors} error(s), {warnings} warning(s)");
        if errors > 0 {
            std::process::exit(1);
        }
    }
    Ok(())
}

fn check_path_mode(path: &str, expected_mode: u32, is_dir: bool) -> Vec<Check> {
    use std::os::unix::fs::MetadataExt;
    match std::fs::metadata(path) {
        Err(_) => {
            let kind = if is_dir { "directory" } else { "file" };
            vec![Check::fail(format!("{path} {kind} missing"))]
        }
        Ok(meta) => {
            let actual = meta.mode() & 0o777;
            if actual == expected_mode {
                vec![Check::ok(format!("{path} exists (mode {:04o})", actual))]
            } else {
                vec![
                    Check::ok(format!("{path} exists")),
                    Check::fail(format!(
                        "{path} mode is {:04o}, expected {:04o}",
                        actual, expected_mode
                    )),
                ]
            }
        }
    }
}

async fn check_http_reachable(url: &str, label: &str) -> Check {
    match reqwest::Client::builder()
        .timeout(std::time::Duration::from_secs(2))
        .build()
        .unwrap()
        .get(url)
        .send()
        .await
    {
        Ok(_) => Check::ok(format!("{label} reachable ({url})")),
        Err(_) => Check::fail(format!("{label} not reachable at {url}")),
    }
}

fn nginx_installed() -> bool {
    which("nginx")
}

fn caddy_installed() -> bool {
    which("caddy")
}

fn which(bin: &str) -> bool {
    std::process::Command::new("which")
        .arg(bin)
        .output()
        .map(|o| o.status.success())
        .unwrap_or(false)
}

fn systemctl_active(service: &str) -> bool {
    std::process::Command::new("systemctl")
        .args(["is-active", "--quiet", service])
        .output()
        .map(|o| o.status.success())
        .unwrap_or(false)
}

fn check_nginx() -> Vec<Check> {
    let mut out = Vec::new();

    if !nginx_installed() {
        out.push(Check::fail("nginx not found — install with: apt install nginx"));
        return out;
    }
    out.push(Check::ok("nginx installed"));

    if systemctl_active("nginx") {
        out.push(Check::ok("nginx running"));
    } else {
        out.push(Check::fail("nginx not running — start with: systemctl start nginx"));
    }

    let remo_conf = "/etc/nginx/sites-enabled/remo-wildcard.conf";
    if std::path::Path::new(remo_conf).exists() {
        out.push(Check::ok(format!("remo nginx config present ({remo_conf})")));
    } else {
        out.push(Check::warn(format!(
            "remo nginx config missing ({remo_conf}) — run `remo server install` or copy manually"
        )));
    }

    // Scan for default_server conflicts in other configs.
    if let Ok(entries) = std::fs::read_dir("/etc/nginx/sites-enabled") {
        for entry in entries.flatten() {
            let path = entry.path();
            let name = path.file_name().and_then(|n| n.to_str()).unwrap_or("");
            if name == "remo-wildcard.conf" {
                continue;
            }
            if let Ok(content) = std::fs::read_to_string(&path) {
                if content.contains("default_server") && content.contains("listen 80") {
                    out.push(Check::warn(format!(
                        "{} has 'listen 80 default_server' — may conflict with remo wildcard",
                        path.display()
                    )));
                }
            }
        }
    }

    // nginx -t config test.
    let test = std::process::Command::new("nginx").arg("-t").output();
    match test {
        Ok(o) if o.status.success() => out.push(Check::ok("nginx config valid (nginx -t)")),
        Ok(o) => {
            let stderr = String::from_utf8_lossy(&o.stderr);
            let first_error = stderr.lines().find(|l| l.contains("emerg") || l.contains("error"))
                .unwrap_or("nginx -t failed");
            out.push(Check::fail(format!("nginx config invalid: {first_error}")));
        }
        Err(_) => out.push(Check::warn("could not run 'nginx -t'")),
    }

    out
}

fn check_caddy() -> Vec<Check> {
    let mut out = Vec::new();

    if !caddy_installed() {
        out.push(Check::fail("caddy not found — install from https://caddyserver.com/docs/install"));
        return out;
    }
    out.push(Check::ok("caddy installed"));

    if systemctl_active("caddy") {
        out.push(Check::ok("caddy running"));
    } else {
        out.push(Check::fail("caddy not running — start with: systemctl start caddy"));
    }

    let remo_cf = "/etc/caddy/remo.caddyfile";
    if std::path::Path::new(remo_cf).exists() {
        out.push(Check::ok(format!("remo Caddyfile present ({remo_cf})")));
    } else {
        out.push(Check::warn(format!("remo Caddyfile missing ({remo_cf})")));
    }

    let main_cf = "/etc/caddy/Caddyfile";
    match std::fs::read_to_string(main_cf) {
        Ok(content) if content.contains("import /etc/caddy/remo.caddyfile") => {
            out.push(Check::ok(format!("{main_cf} imports remo.caddyfile")));
        }
        Ok(_) => out.push(Check::warn(format!(
            "{main_cf} does not import /etc/caddy/remo.caddyfile — add: import /etc/caddy/remo.caddyfile"
        ))),
        Err(_) => out.push(Check::warn(format!("{main_cf} not readable"))),
    }

    out
}

