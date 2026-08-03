pub mod api;
pub mod auth;

use anyhow::Result;
use axum::{middleware, Router};
use std::sync::Arc;

use crate::config::ServerConfig;
use crate::db;

#[derive(Clone)]
pub struct AppState {
    pub pool: sqlx::SqlitePool,
    pub cfg: ServerConfig,
    pub master_token: String,
}

pub async fn start(port: u16) -> Result<()> {
    let cfg = ServerConfig::load()?;
    let db_path = format!("{}/state.db", cfg.data_dir);
    let pool = db::open(&db_path).await?;

    let master_token = std::fs::read_to_string("/etc/remo/master_token")
        .map(|s| s.trim().to_string())
        .unwrap_or_default();
    if master_token.is_empty() {
        tracing::warn!("master_token is empty — /etc/remo/master_token missing or blank; admin auth is disabled");
    }

    let bind_addr = cfg.bind_addr.clone();
    let state = Arc::new(AppState { pool, cfg, master_token });

    let app = Router::new()
        .merge(api::public_routes())
        // User-accessible routes: any valid token.
        .merge(
            api::user_routes()
                .layer(middleware::from_fn_with_state(
                    state.clone(),
                    auth::require_auth,
                )),
        )
        // Admin-only routes: valid token + is_admin.
        // Middleware stacks bottom-up: require_auth runs first (outermost).
        .merge(
            api::admin_routes()
                .layer(middleware::from_fn(auth::require_admin))
                .layer(middleware::from_fn_with_state(
                    state.clone(),
                    auth::require_auth,
                )),
        )
        .with_state(state);

    let addr = format!("{bind_addr}:{port}");
    let listener = tokio::net::TcpListener::bind(&addr).await?;
    tracing::info!("remo control plane listening on {addr}");
    axum::serve(listener, app).await?;
    Ok(())
}
