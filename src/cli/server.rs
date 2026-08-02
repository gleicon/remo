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
    #[arg(long, default_value = "http://127.0.0.1:9000")]
    pub nano_url: String,

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

    println!("Installing remo on {}", args.domain);

    // Create directory layout. /etc/remo is 0o700 so the master_token is
    // never readable by other accounts even before we set its permissions.
    use std::os::unix::fs::DirBuilderExt;
    let data_dir = "/var/lib/remo";
    for dir in &[
        format!("{data_dir}/apps"),
        format!("{data_dir}/git"),
        "/var/log/remo".to_string(),
    ] {
        std::fs::create_dir_all(dir)?;
        println!("  created {dir}");
    }
    std::fs::DirBuilder::new().recursive(true).mode(0o700).create("/etc/remo")?;
    println!("  created /etc/remo");

    // Generate master token — write atomically with 0o600 so there is no
    // window between creation and permission tightening.
    let token = generate_token();
    let token_path = "/etc/remo/master_token";
    {
        use std::io::Write;
        use std::os::unix::fs::OpenOptionsExt;
        let mut f = std::fs::OpenOptions::new()
            .write(true).create(true).truncate(true).mode(0o600)
            .open(token_path)?;
        f.write_all(token.as_bytes())?;
    }

    // Write server config.
    let cfg = ServerConfig {
        domain: args.domain.clone(),
        data_dir: data_dir.to_string(),
        nano_socket: args.nano_url,
        proxy: args.proxy.clone(),
        control_port: 7070,
    };
    cfg.save()?;
    println!("  wrote {}", ServerConfig::system_path().display());

    // Initialize SQLite.
    let db_path = format!("{data_dir}/state.db");
    let pool = crate::db::open(&db_path).await?;
    drop(pool);
    println!("  initialized {db_path}");

    // Write nginx/Caddy config snippet.
    write_proxy_config(&args.domain, &args.proxy)?;

    println!();
    println!("Installation complete.");
    println!();
    println!("Admin token (save this — shown once):");
    println!("  {token}");
    println!();
    println!("Login from your laptop:");
    println!("  remo login --server https://{} --token <token>", args.domain);
    println!();
    println!("DNS: add wildcard A record to your domain:");
    println!("  *.{} A <this VPS IP>", args.domain);

    Ok(())
}

fn write_proxy_config(domain: &str, backend: &ProxyBackend) -> Result<()> {
    match backend {
        ProxyBackend::Nginx => {
            let path = format!("/etc/nginx/sites-enabled/remo-wildcard.conf");
            let cfg = format!(
                r#"# remo wildcard — DO NOT EDIT (managed by remo)
# Per-app HTTPS added by certbot on `remo apps create`.
server {{
    listen 80 default_server;
    server_name *.{domain};
    location / {{
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }}
}}
"#
            );
            std::fs::write(&path, cfg)?;
            println!("  wrote {path}");
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

