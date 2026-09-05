pub mod api;
pub mod auth;

use anyhow::Result;
use axum::{middleware, Router};
use std::sync::Arc;

use crate::config::ServerConfig;
use crate::db;
use crate::nano_client::{AppLimits, CreateAppRequest, NanoClient, UpdateAppRequest, is_not_found};

#[derive(Clone)]
pub struct AppState {
    pub pool: sqlx::SqlitePool,
    pub cfg: ServerConfig,
    pub master_token: String,
    pub proxy: std::sync::Arc<dyn crate::proxy::ProxyBackend>,
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
    let proxy = crate::proxy::from_config(&cfg);
    let state = Arc::new(AppState { pool, cfg, master_token, proxy });

    // Re-register all deployed apps with nano-rs on startup.
    // nano-rs is stateless; remo is the source of truth.
    {
        let pool = state.pool.clone();
        let cfg = state.cfg.clone();
        tokio::spawn(async move {
            sync_apps_to_nano(&pool, &cfg).await;
        });
    }

    // Watch nano-rs health every 30 s; re-register when it comes back up.
    {
        let pool = state.pool.clone();
        let cfg = state.cfg.clone();
        tokio::spawn(async move {
            watch_nano(&pool, &cfg).await;
        });
    }

    let app = Router::new()
        .merge(api::public_routes())
        .merge(
            api::user_routes()
                .layer(middleware::from_fn_with_state(
                    state.clone(),
                    auth::require_auth,
                )),
        )
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

/// Re-register every deployed app with nano-rs.
/// Safe to call multiple times — tries update first, falls back to create on 404.
async fn sync_apps_to_nano(pool: &sqlx::SqlitePool, cfg: &ServerConfig) {
    let nano = NanoClient::from_config(cfg);
    let apps = match db::app_list(pool).await {
        Ok(a) => a,
        Err(e) => {
            tracing::error!("sync: db::app_list failed: {e}");
            return;
        }
    };

    let mut ok = 0u32;
    let mut skip = 0u32;
    let mut fail = 0u32;

    for app in apps {
        let Some(ref sha) = app.current_sha else {
            skip += 1;
            continue; // never deployed
        };

        let hostname = app.hostname.clone();
        let entrypoint = format!("{}/apps/{}/current/{}", cfg.data_dir, app.id, app.entrypoint);

        let env_vars: std::collections::HashMap<String, String> =
            db::env_list_decoded(pool, &app.id).await
                .unwrap_or_default()
                .into_iter()
                .collect();

        let compat = if app.app_type == "gas" { Some("gas".to_string()) } else { None };

        let update_res = nano.update_app(&hostname, &UpdateAppRequest {
            entrypoint: Some(entrypoint.clone()),
            env_vars: if env_vars.is_empty() { None } else { Some(env_vars.clone()) },
            limits: None,
            compat: compat.clone(),
        }).await;

        let registered = match update_res {
            Ok(()) => true,
            Err(e) if is_not_found(&e) => {
                match nano.create_app(&CreateAppRequest {
                    hostname: hostname.clone(),
                    entrypoint,
                    env_vars,
                    limits: AppLimits { workers: 2, ..Default::default() },
                    activate: true,
                    compat,
                }).await {
                    Ok(()) => true,
                    Err(e) => {
                        tracing::warn!("sync: create {hostname} failed: {e}");
                        fail += 1;
                        false
                    }
                }
            }
            Err(e) => {
                tracing::warn!("sync: update {hostname} failed: {e}");
                fail += 1;
                false
            }
        };

        if registered {
            match nano.reload_app(&hostname).await {
                Ok(()) => {
                    tracing::info!("sync: registered {hostname} sha={sha}");
                    ok += 1;
                }
                Err(e) => {
                    tracing::warn!("sync: reload {hostname} failed: {e}");
                    fail += 1;
                }
            }
        }
    }

    tracing::info!("sync complete: {ok} registered, {skip} not-yet-deployed, {fail} failed");
}

/// Poll nano-rs every 30 s. Re-register all apps when it recovers from down.
async fn watch_nano(pool: &sqlx::SqlitePool, cfg: &ServerConfig) {
    let nano_url = std::env::var("REMO_NANO_SOCKET")
        .unwrap_or_else(|_| cfg.nano_socket.clone());
    // Admin API base URL — any HTTP response (even 4xx) means nano-rs is up.
    let probe = format!("{nano_url}/admin/apps");
    let client = reqwest::Client::builder()
        .timeout(std::time::Duration::from_secs(5))
        .build()
        .unwrap_or_default();

    // On startup nano-rs is guaranteed up (Docker depends_on healthy).
    let mut was_up = true;

    loop {
        tokio::time::sleep(std::time::Duration::from_secs(30)).await;
        let is_up = client.get(&probe).send().await
            .map(|_| true)
            .unwrap_or(false);

        if is_up && !was_up {
            tracing::info!("nano-rs recovered — re-registering all apps");
            sync_apps_to_nano(pool, cfg).await;
        } else if !is_up && was_up {
            tracing::warn!("nano-rs appears down (probe: {probe})");
        }

        was_up = is_up;
    }
}
