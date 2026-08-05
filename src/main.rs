use clap::{Parser, Subcommand};

mod cli;
mod config;
mod db;
mod deploy;
mod nano_client;
mod proxy;
mod server;
mod validation;

#[derive(Parser)]
#[command(name = "remo", version, about = "remo: edge PaaS on nano-rs")]
struct Cli {
    #[command(subcommand)]
    command: Commands,
}

#[derive(Subcommand)]
enum Commands {
    /// Server management (run on VPS as root)
    Server {
        #[command(subcommand)]
        cmd: cli::server::ServerCmd,
    },
    /// Interactive first-time setup: generate SSH key, create user, configure client
    Setup(cli::setup::SetupArgs),
    /// Authenticate CLI with a remo server
    Login(cli::login::LoginArgs),
    /// App management
    Apps {
        #[command(subcommand)]
        cmd: cli::apps::AppsCmd,
    },
    /// User management (admin only)
    Users {
        #[command(subcommand)]
        cmd: cli::users::UsersCmd,
    },
    /// Environment variable management
    Env {
        #[command(subcommand)]
        cmd: cli::env::EnvCmd,
    },
    /// Scale app workers
    Scale(cli::deploy::ScaleArgs),
    /// Deployment history for an app
    Deployments(cli::deploy::DeploymentsArgs),
    /// Roll back to previous deploy
    Rollback(cli::deploy::RollbackArgs),
    /// Tail runtime logs
    Logs(cli::logs::LogsArgs),
    /// Deploy current directory (git push remo main)
    Push,
    /// git-hook — called by SSH forced command (internal)
    #[command(hide = true)]
    GitHook(cli::git_hook::GitHookArgs),
    /// Data-plane agent for worker nodes (future multi-node)
    #[command(hide = true)]
    Agent(cli::agent::AgentArgs),
}

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    tracing_subscriber::fmt()
        .with_env_filter(
            tracing_subscriber::EnvFilter::try_from_default_env()
                .unwrap_or_else(|_| "remo=info".into()),
        )
        .init();

    let cli = Cli::parse();

    match cli.command {
        Commands::Server { cmd } => cli::server::run(cmd).await,
        Commands::Setup(args) => cli::setup::run(args).await,
        Commands::Login(args) => cli::login::run(args).await,
        Commands::Apps { cmd } => cli::apps::run(cmd).await,
        Commands::Users { cmd } => cli::users::run(cmd).await,
        Commands::Env { cmd } => cli::env::run(cmd).await,
        Commands::Scale(args) => cli::deploy::scale(args).await,
        Commands::Deployments(args) => cli::deploy::deployments(args).await,
        Commands::Rollback(args) => cli::deploy::rollback(args).await,
        Commands::Logs(args) => cli::logs::run(args).await,
        Commands::Push => cli::push::run().await,
        Commands::GitHook(args) => cli::git_hook::run(args).await,
        Commands::Agent(args) => cli::agent::run(args).await,
    }
}
