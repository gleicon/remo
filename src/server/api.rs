use axum::{
    extract::{Extension, Path, State},
    http::StatusCode,
    response::IntoResponse,
    routing::{delete, get, post, put},
    Json, Router,
};
use chrono::Utc;
use serde::{Deserialize, Serialize};
use std::sync::Arc;
use uuid::Uuid;

use crate::db::{self, App, User};
use crate::server::auth::AuthUser;
use crate::server::AppState;

type ApiResult<T> = Result<T, ApiError>;

// ── Validation ────────────────────────────────────────────────────────────────

fn validate_app_name(name: &str) -> ApiResult<()> {
    if crate::validation::is_valid_app_name(name) {
        Ok(())
    } else {
        Err(ApiError::Bad(
            "app name must be 1-32 lowercase alphanum/hyphen chars, no leading/trailing hyphen".into(),
        ))
    }
}

fn validate_entrypoint(ep: &str) -> ApiResult<()> {
    // Relative paths only: safe filename chars per segment, no `..`, no leading slash.
    let ok = !ep.is_empty()
        && !ep.starts_with('/')
        && !ep.contains('\0')
        && ep.split('/').all(|seg| {
            !seg.is_empty()
                && seg != ".."
                && seg.chars().all(|c| c.is_ascii_alphanumeric() || matches!(c, '.' | '-' | '_'))
        });
    if ok {
        Ok(())
    } else {
        Err(ApiError::Bad(
            "entrypoint must be a relative path with safe components (no .., no leading /)".into(),
        ))
    }
}

fn validate_sha(sha: &str) -> ApiResult<()> {
    if sha.len() == 16 && sha.chars().all(|c| c.is_ascii_hexdigit()) {
        Ok(())
    } else {
        Err(ApiError::Bad("invalid deploy sha".into()))
    }
}

/// Verify that a path join stays under `base`. Rejects any `..` or absolute components.
fn safe_join(
    base: &std::path::Path,
    segment: &str,
) -> ApiResult<std::path::PathBuf> {
    use std::path::Component;
    for comp in std::path::Path::new(segment).components() {
        match comp {
            Component::Normal(_) => {}
            _ => return Err(ApiError::Bad("invalid path segment".into())),
        }
    }
    Ok(base.join(segment))
}

// ── Public ────────────────────────────────────────────────────────────────────

pub fn public_routes() -> Router<Arc<AppState>> {
    Router::new().route("/health", get(health))
}

async fn health() -> impl IntoResponse {
    Json(serde_json::json!({ "ok": true, "service": "remo" }))
}

// ── User routes (any valid token) ─────────────────────────────────────────────

pub fn user_routes() -> Router<Arc<AppState>> {
    Router::new()
        .route("/api/apps", get(apps_list).post(apps_create))
        .route("/api/apps/:name", get(apps_get).delete(apps_delete))
        .route("/api/apps/:name/deployments", get(deployments_list))
        .route("/api/apps/:name/rollback", post(apps_rollback))
        .route("/api/apps/:name/scale", post(apps_scale))
        .route("/api/apps/:name/env", get(env_list).post(env_set))
        .route("/api/apps/:name/env/:key", delete(env_unset))
        .route("/api/apps/:name/domain", put(domain_set).delete(domain_unset))
        .route("/api/apps/:name/logs", get(logs_get))
}

// ── Admin routes (master token or is_admin user) ──────────────────────────────

pub fn admin_routes() -> Router<Arc<AppState>> {
    Router::new()
        .route("/api/users", get(users_list).post(users_create))
        .route("/api/users/:name", delete(users_delete))
}

// ── App handlers ──────────────────────────────────────────────────────────────

#[derive(Deserialize)]
struct CreateAppBody {
    name: String,
    #[serde(rename = "type", default = "default_type")]
    app_type: String,
    #[serde(default = "default_entrypoint")]
    entrypoint: String,
}
fn default_type() -> String { "js".into() }
fn default_entrypoint() -> String { "index.js".into() }

#[derive(Serialize)]
struct AppResponse {
    name: String,
    hostname: String,
    owner: String,
    app_type: String,
    entrypoint: String,
    current_sha: Option<String>,
    custom_domain: Option<String>,
    created_at: String,
}

impl From<App> for AppResponse {
    fn from(a: App) -> Self {
        AppResponse {
            name: a.id,
            hostname: a.hostname,
            owner: a.owner,
            app_type: a.app_type,
            entrypoint: a.entrypoint,
            current_sha: a.current_sha,
            custom_domain: a.custom_domain,
            created_at: a.created_at.to_rfc3339(),
        }
    }
}

async fn apps_list(
    State(state): State<Arc<AppState>>,
    Extension(auth): Extension<AuthUser>,
) -> ApiResult<Json<Vec<AppResponse>>> {
    let apps = if auth.is_admin {
        db::app_list(&state.pool).await?
    } else {
        db::app_list_by_owner(&state.pool, &auth.name).await?
    };
    Ok(Json(apps.into_iter().map(Into::into).collect()))
}

async fn apps_get(
    State(state): State<Arc<AppState>>,
    Extension(auth): Extension<AuthUser>,
    Path(name): Path<String>,
) -> ApiResult<Json<AppResponse>> {
    validate_app_name(&name)?;
    let app = db::app_get(&state.pool, &name)
        .await?
        .ok_or(ApiError::NotFound)?;
    check_ownership(&auth, &app.owner)?;
    Ok(Json(app.into()))
}

async fn apps_create(
    State(state): State<Arc<AppState>>,
    Extension(auth): Extension<AuthUser>,
    Json(body): Json<CreateAppBody>,
) -> ApiResult<(StatusCode, Json<AppResponse>)> {
    validate_app_name(&body.name)?;
    validate_entrypoint(&body.entrypoint)?;
    if db::app_get(&state.pool, &body.name).await?.is_some() {
        return Err(ApiError::Conflict("app already exists".into()));
    }

    let now = Utc::now();
    // Canonical hostname is owner-namespaced to prevent subdomain squatting:
    // {owner}-{name}.{domain} ensures app names are unique per-user, not globally.
    let hostname = format!("{}-{}.{}", auth.name, body.name, state.cfg.domain);
    let app = App {
        id: body.name.clone(),
        hostname,
        owner: auth.name.clone(),
        app_type: body.app_type,
        entrypoint: body.entrypoint,
        current_sha: None,
        custom_domain: None,
        created_at: now,
        updated_at: now,
    };
    db::app_create(&state.pool, &app).await?;

    let app_dir = format!("{}/apps/{}", state.cfg.data_dir, body.name);
    std::fs::create_dir_all(format!("{app_dir}/deploys"))?;

    Ok((StatusCode::CREATED, Json(app.into())))
}

async fn apps_delete(
    State(state): State<Arc<AppState>>,
    Extension(auth): Extension<AuthUser>,
    Path(name): Path<String>,
) -> ApiResult<StatusCode> {
    validate_app_name(&name)?;
    let app = db::app_get(&state.pool, &name)
        .await?
        .ok_or(ApiError::NotFound)?;
    check_ownership(&auth, &app.owner)?;
    db::app_delete(&state.pool, &name).await?;
    Ok(StatusCode::NO_CONTENT)
}

// ── Deploy / rollback ─────────────────────────────────────────────────────────

async fn deployments_list(
    State(state): State<Arc<AppState>>,
    Extension(auth): Extension<AuthUser>,
    Path(name): Path<String>,
) -> ApiResult<Json<Vec<serde_json::Value>>> {
    validate_app_name(&name)?;
    let app = db::app_get(&state.pool, &name).await?.ok_or(ApiError::NotFound)?;
    check_ownership(&auth, &app.owner)?;
    let deploys = db::deployments_for_app(&state.pool, &name).await?;
    let out: Vec<_> = deploys
        .into_iter()
        .map(|d| serde_json::json!({
            "id": d.id,
            "status": d.status,
            "deployer": d.deployer,
            "created_at": d.created_at.to_rfc3339(),
        }))
        .collect();
    Ok(Json(out))
}

#[derive(Deserialize)]
struct RollbackBody { sha: Option<String> }

async fn apps_rollback(
    State(state): State<Arc<AppState>>,
    Extension(auth): Extension<AuthUser>,
    Path(name): Path<String>,
    Json(body): Json<RollbackBody>,
) -> ApiResult<StatusCode> {
    validate_app_name(&name)?;
    let app = db::app_get(&state.pool, &name).await?.ok_or(ApiError::NotFound)?;
    check_ownership(&auth, &app.owner)?;

    let deploys = db::deployments_for_app(&state.pool, &name).await?;
    let target_sha = match body.sha {
        Some(s) => { validate_sha(&s)?; s }
        None => {
            let current = app.current_sha.as_deref().unwrap_or("");
            deploys
                .iter()
                .find(|d| d.status == "active" && d.id != current)
                .map(|d| d.id.clone())
                .ok_or_else(|| ApiError::Bad("no previous deploy to roll back to".into()))?
        }
    };

    // Build paths using validated segments only.
    let data = std::path::Path::new(&state.cfg.data_dir);
    let deploy_dir = safe_join(&data.join("apps").join(&name).join("deploys"), &target_sha)?;
    if !deploy_dir.exists() {
        return Err(ApiError::Bad("deploy artifact not found".into()));
    }

    let link = data.join("apps").join(&name).join("current");
    let tmp = data.join("apps").join(&name).join(".current.tmp");
    if tmp.exists() { let _ = std::fs::remove_file(&tmp); }
    std::os::unix::fs::symlink(&deploy_dir, &tmp).map_err(|e| ApiError::Internal(e.to_string()))?;
    std::fs::rename(&tmp, &link).map_err(|e| ApiError::Internal(e.to_string()))?;

    db::app_set_sha(&state.pool, &name, &target_sha).await?;
    let nano = crate::nano_client::NanoClient::new(&state.cfg.nano_socket);
    nano.reload_app(&app.hostname).await?;
    Ok(StatusCode::NO_CONTENT)
}

#[derive(Deserialize)]
struct ScaleBody { workers: u32 }

async fn apps_scale(
    State(state): State<Arc<AppState>>,
    Extension(auth): Extension<AuthUser>,
    Path(name): Path<String>,
    Json(body): Json<ScaleBody>,
) -> ApiResult<StatusCode> {
    validate_app_name(&name)?;
    let app = db::app_get(&state.pool, &name).await?.ok_or(ApiError::NotFound)?;
    check_ownership(&auth, &app.owner)?;
    let nano = crate::nano_client::NanoClient::new(&state.cfg.nano_socket);
    nano.scale_app(&app.hostname, body.workers).await?;
    Ok(StatusCode::NO_CONTENT)
}

// ── Env ───────────────────────────────────────────────────────────────────────

async fn env_list(
    State(state): State<Arc<AppState>>,
    Extension(auth): Extension<AuthUser>,
    Path(name): Path<String>,
) -> ApiResult<Json<serde_json::Value>> {
    validate_app_name(&name)?;
    let app = db::app_get(&state.pool, &name).await?.ok_or(ApiError::NotFound)?;
    check_ownership(&auth, &app.owner)?;
    let vars = db::env_list(&state.pool, &name).await?;
    let obj: serde_json::Map<_, _> = vars
        .into_iter()
        .map(|(k, _)| (k, serde_json::Value::String("***".into())))
        .collect();
    Ok(Json(serde_json::Value::Object(obj)))
}

#[derive(Deserialize)]
struct EnvSetBody { vars: std::collections::HashMap<String, String> }

async fn env_set(
    State(state): State<Arc<AppState>>,
    Extension(auth): Extension<AuthUser>,
    Path(name): Path<String>,
    Json(body): Json<EnvSetBody>,
) -> ApiResult<StatusCode> {
    validate_app_name(&name)?;
    let app = db::app_get(&state.pool, &name).await?.ok_or(ApiError::NotFound)?;
    check_ownership(&auth, &app.owner)?;
    for (k, v) in &body.vars {
        let enc = base64::Engine::encode(&base64::engine::general_purpose::STANDARD, v);
        db::env_set(&state.pool, &name, k, &enc).await?;
    }
    let nano = crate::nano_client::NanoClient::new(&state.cfg.nano_socket);
    let pairs: Vec<_> = body.vars.into_iter().collect();
    nano.set_env(&app.hostname, pairs).await
        .map_err(|e| ApiError::Internal(format!("nano-rs rejected env update: {e}")))?;
    Ok(StatusCode::NO_CONTENT)
}

async fn env_unset(
    State(state): State<Arc<AppState>>,
    Extension(auth): Extension<AuthUser>,
    Path((name, key)): Path<(String, String)>,
) -> ApiResult<StatusCode> {
    validate_app_name(&name)?;
    let app = db::app_get(&state.pool, &name).await?.ok_or(ApiError::NotFound)?;
    check_ownership(&auth, &app.owner)?;
    db::env_unset(&state.pool, &name, &key).await?;
    // Apply remaining DB env to the running worker so the deleted key is removed immediately.
    let remaining = db::env_list_decoded(&state.pool, &name).await?;
    let nano = crate::nano_client::NanoClient::new(&state.cfg.nano_socket);
    nano.set_env(&app.hostname, remaining.into_iter().collect()).await
        .map_err(|e| ApiError::Internal(format!("nano-rs rejected env update: {e}")))?;
    Ok(StatusCode::NO_CONTENT)
}

// ── Custom domain ─────────────────────────────────────────────────────────────

#[derive(Deserialize)]
struct DomainBody { domain: String }

fn validate_domain(domain: &str) -> ApiResult<()> {
    let ok = !domain.is_empty()
        && domain.len() <= 253
        && !domain.contains('/')
        && !domain.contains(' ')
        && domain.contains('.')
        && domain.chars().all(|c| c.is_ascii_alphanumeric() || c == '-' || c == '.');
    if ok {
        Ok(())
    } else {
        Err(ApiError::Bad("invalid domain: must be a valid hostname (e.g. myapp.example.com)".into()))
    }
}

async fn domain_set(
    State(state): State<Arc<AppState>>,
    Extension(auth): Extension<AuthUser>,
    Path(name): Path<String>,
    Json(body): Json<DomainBody>,
) -> ApiResult<StatusCode> {
    validate_app_name(&name)?;
    validate_domain(&body.domain)?;
    let app = db::app_get(&state.pool, &name).await?.ok_or(ApiError::NotFound)?;
    check_ownership(&auth, &app.owner)?;
    db::app_set_custom_domain(&state.pool, &name, Some(&body.domain)).await?;
    Ok(StatusCode::NO_CONTENT)
}

async fn domain_unset(
    State(state): State<Arc<AppState>>,
    Extension(auth): Extension<AuthUser>,
    Path(name): Path<String>,
) -> ApiResult<StatusCode> {
    validate_app_name(&name)?;
    let app = db::app_get(&state.pool, &name).await?.ok_or(ApiError::NotFound)?;
    check_ownership(&auth, &app.owner)?;
    db::app_set_custom_domain(&state.pool, &name, None).await?;
    Ok(StatusCode::NO_CONTENT)
}

// ── Logs ──────────────────────────────────────────────────────────────────────

async fn logs_get(
    State(state): State<Arc<AppState>>,
    Extension(auth): Extension<AuthUser>,
    Path(name): Path<String>,
) -> ApiResult<String> {
    validate_app_name(&name)?;
    let app = db::app_get(&state.pool, &name).await?.ok_or(ApiError::NotFound)?;
    check_ownership(&auth, &app.owner)?;
    let deploys = db::deployments_for_app(&state.pool, &name).await?;
    let log = deploys
        .iter()
        .filter_map(|d| d.log_output.as_deref())
        .collect::<Vec<_>>()
        .join("\n---\n");
    Ok(log)
}

// ── User handlers (admin only) ────────────────────────────────────────────────

#[derive(Deserialize)]
struct CreateUserBody { name: String, ssh_pubkey: Option<String> }

async fn users_create(
    State(state): State<Arc<AppState>>,
    Json(body): Json<CreateUserBody>,
) -> ApiResult<(StatusCode, Json<serde_json::Value>)> {
    if db::user_get_by_name(&state.pool, &body.name).await?.is_some() {
        return Err(ApiError::Conflict("user already exists".into()));
    }
    let raw_token: String = {
        use rand::Rng;
        let b: Vec<u8> = rand::thread_rng()
            .sample_iter(rand::distributions::Standard)
            .take(32)
            .collect();
        hex::encode(b)
    };
    let token_hash = crate::server::auth::sha256_hex(&raw_token);

    let user = User {
        id: Uuid::new_v4().to_string(),
        name: body.name.clone(),
        token_hash,
        ssh_pubkey: body.ssh_pubkey,
        is_admin: false,
        created_at: Utc::now(),
    };
    db::user_create(&state.pool, &user).await?;
    Ok((
        StatusCode::CREATED,
        Json(serde_json::json!({ "name": user.name, "token": raw_token })),
    ))
}

async fn users_list(State(state): State<Arc<AppState>>) -> ApiResult<Json<Vec<serde_json::Value>>> {
    let users = db::user_list(&state.pool).await?;
    let out: Vec<_> = users
        .into_iter()
        .map(|u| serde_json::json!({ "name": u.name, "is_admin": u.is_admin }))
        .collect();
    Ok(Json(out))
}

async fn users_delete(
    State(state): State<Arc<AppState>>,
    Path(name): Path<String>,
) -> ApiResult<StatusCode> {
    if !db::user_delete(&state.pool, &name).await? {
        return Err(ApiError::NotFound);
    }
    Ok(StatusCode::NO_CONTENT)
}

// ── Ownership check ───────────────────────────────────────────────────────────

fn check_ownership(auth: &AuthUser, owner: &str) -> ApiResult<()> {
    if auth.is_admin || auth.name == owner {
        Ok(())
    } else {
        Err(ApiError::NotFound) // return 404 not 403 — don't leak app existence to other users
    }
}

// ── Error type ────────────────────────────────────────────────────────────────

#[derive(Debug)]
enum ApiError {
    NotFound,
    Conflict(String),
    Bad(String),
    Internal(String),
    Db(sqlx::Error),
    Anyhow(anyhow::Error),
    Io(std::io::Error),
}

impl From<sqlx::Error> for ApiError { fn from(e: sqlx::Error) -> Self { ApiError::Db(e) } }
impl From<anyhow::Error> for ApiError { fn from(e: anyhow::Error) -> Self { ApiError::Anyhow(e) } }
impl From<std::io::Error> for ApiError { fn from(e: std::io::Error) -> Self { ApiError::Io(e) } }

impl IntoResponse for ApiError {
    fn into_response(self) -> axum::response::Response {
        let (status, msg) = match self {
            ApiError::NotFound => (StatusCode::NOT_FOUND, "not found".into()),
            ApiError::Conflict(m) => (StatusCode::CONFLICT, m),
            ApiError::Bad(m) => (StatusCode::BAD_REQUEST, m),
            ApiError::Internal(m) => (StatusCode::INTERNAL_SERVER_ERROR, m),
            ApiError::Db(e) => (StatusCode::INTERNAL_SERVER_ERROR, e.to_string()),
            ApiError::Anyhow(e) => (StatusCode::INTERNAL_SERVER_ERROR, e.to_string()),
            ApiError::Io(e) => (StatusCode::INTERNAL_SERVER_ERROR, e.to_string()),
        };
        (status, Json(serde_json::json!({ "error": msg }))).into_response()
    }
}
