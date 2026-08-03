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

use crate::db::{self, App, Invite, User};
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
    if crate::validation::is_valid_entrypoint(ep) {
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
    Router::new()
        .route("/health", get(health))
        .route("/api/invites/{token}/claim", post(invite_claim))
}

async fn health() -> impl IntoResponse {
    Json(serde_json::json!({ "ok": true, "service": "remo" }))
}

// ── User routes (any valid token) ─────────────────────────────────────────────

pub fn user_routes() -> Router<Arc<AppState>> {
    Router::new()
        .route("/api/apps", get(apps_list).post(apps_create))
        .route("/api/apps/{name}", get(apps_get).delete(apps_delete))
        .route("/api/apps/{name}/deployments", get(deployments_list))
        .route("/api/apps/{name}/rollback", post(apps_rollback))
        .route("/api/apps/{name}/scale", post(apps_scale))
        .route("/api/apps/{name}/env", get(env_list).post(env_set))
        .route("/api/apps/{name}/env/{key}", delete(env_unset))
        .route("/api/apps/{name}/domain", put(domain_set).delete(domain_unset))
        .route("/api/apps/{name}/logs", get(logs_get))
}

// ── Admin routes (master token or is_admin user) ──────────────────────────────

pub fn admin_routes() -> Router<Arc<AppState>> {
    Router::new()
        .route("/api/users", get(users_list).post(users_create))
        .route("/api/users/{name}", delete(users_delete))
        .route("/api/admin/invites", post(invite_create).get(invites_list))
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
    let nano = crate::nano_client::NanoClient::from_config(&state.cfg);
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
    let nano = crate::nano_client::NanoClient::from_config(&state.cfg);
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
    let nano = crate::nano_client::NanoClient::from_config(&state.cfg);
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
    let nano = crate::nano_client::NanoClient::from_config(&state.cfg);
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
    let raw_token = gen_token();
    let token_hash = crate::server::auth::sha256_hex(&raw_token);

    let user = User {
        id: Uuid::new_v4().to_string(),
        name: body.name.clone(),
        token_hash,
        ssh_pubkey: body.ssh_pubkey.clone(),
        is_admin: false,
        created_at: Utc::now(),
    };
    db::user_create(&state.pool, &user).await?;

    if let Some(ref pubkey) = body.ssh_pubkey {
        if let Err(e) = write_authorized_key(&user.name, pubkey) {
            tracing::warn!("user {} created but authorized_keys write failed: {e}", user.name);
        }
    }

    Ok((
        StatusCode::CREATED,
        Json(serde_json::json!({ "name": user.name, "token": raw_token })),
    ))
}

// ── Invite handlers ───────────────────────────────────────────────────────────

#[derive(Deserialize)]
struct CreateInviteBody {
    username: String,
    email: Option<String>,
    #[serde(default = "default_invite_expires")]
    expires_in_secs: u64,
}

fn default_invite_expires() -> u64 { 3600 }

async fn invite_create(
    State(state): State<Arc<AppState>>,
    Json(body): Json<CreateInviteBody>,
) -> ApiResult<(StatusCode, Json<serde_json::Value>)> {
    if !crate::validation::is_valid_app_name(&body.username) {
        return Err(ApiError::Bad(
            "username must be 1-32 lowercase alphanum/hyphen chars".into(),
        ));
    }
    if db::user_get_by_name(&state.pool, &body.username).await?.is_some() {
        return Err(ApiError::Conflict("user already exists".into()));
    }

    let raw_token = gen_token();
    let token_hash = crate::server::auth::sha256_hex(&raw_token);
    let expires_at = Utc::now()
        + chrono::Duration::seconds(body.expires_in_secs as i64);

    let invite = Invite {
        id: Uuid::new_v4().to_string(),
        token_hash,
        username: body.username.clone(),
        email: body.email.clone(),
        expires_at,
        used_at: None,
    };
    db::invite_create(&state.pool, &invite).await?;

    Ok((
        StatusCode::CREATED,
        Json(serde_json::json!({
            "token": raw_token,
            "username": body.username,
            "expires_at": expires_at.to_rfc3339(),
            "claim_command": format!("remo setup --invite {raw_token}"),
        })),
    ))
}

async fn invites_list(
    State(state): State<Arc<AppState>>,
) -> ApiResult<Json<Vec<serde_json::Value>>> {
    let invites = db::invite_list(&state.pool).await?;
    let out = invites
        .into_iter()
        .map(|i| serde_json::json!({
            "username": i.username,
            "email": i.email,
            "expires_at": i.expires_at.to_rfc3339(),
            "used": i.used_at.is_some(),
        }))
        .collect();
    Ok(Json(out))
}

#[derive(Deserialize)]
struct ClaimInviteBody {
    ssh_pubkey: String,
}

async fn invite_claim(
    State(state): State<Arc<AppState>>,
    Path(token): Path<String>,
    Json(body): Json<ClaimInviteBody>,
) -> ApiResult<(StatusCode, Json<serde_json::Value>)> {
    let token_hash = crate::server::auth::sha256_hex(&token);

    let invite = db::invite_get_by_token_hash(&state.pool, &token_hash)
        .await?
        .ok_or(ApiError::NotFound)?;

    if invite.used_at.is_some() {
        return Err(ApiError::Conflict("invite already used".into()));
    }
    if Utc::now() > invite.expires_at {
        return Err(ApiError::Bad("invite expired".into()));
    }
    if db::user_get_by_name(&state.pool, &invite.username).await?.is_some() {
        return Err(ApiError::Conflict("username already taken".into()));
    }

    let pubkey = body.ssh_pubkey.trim();
    if !pubkey.starts_with("ssh-") && !pubkey.starts_with("ecdsa-sk-") {
        return Err(ApiError::Bad("invalid ssh public key format".into()));
    }

    let raw_token = gen_token();
    let token_hash_user = crate::server::auth::sha256_hex(&raw_token);

    let user = User {
        id: Uuid::new_v4().to_string(),
        name: invite.username.clone(),
        token_hash: token_hash_user,
        ssh_pubkey: Some(pubkey.to_string()),
        is_admin: false,
        created_at: Utc::now(),
    };
    db::user_create(&state.pool, &user).await?;

    if let Err(e) = write_authorized_key(&user.name, pubkey) {
        tracing::warn!("user {} created via invite but authorized_keys write failed: {e}", user.name);
    }

    db::invite_mark_used(&state.pool, &invite.id).await?;

    Ok((
        StatusCode::CREATED,
        Json(serde_json::json!({ "username": user.name, "token": raw_token })),
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

// ── Shared helpers ────────────────────────────────────────────────────────────

fn gen_token() -> String {
    use rand::Rng;
    let b: Vec<u8> = rand::thread_rng()
        .sample_iter(rand::distributions::Standard)
        .take(32)
        .collect();
    hex::encode(b)
}

/// Append a forced-command authorized_keys line for `username`.
/// Idempotent: skips write if the pubkey is already present.
pub fn write_authorized_key(username: &str, pubkey: &str) -> std::io::Result<()> {
    use std::io::Write as _;
    let pubkey = pubkey.trim();
    let path = std::path::Path::new("/etc/remo/authorized_keys");
    if path.exists() {
        let existing = std::fs::read_to_string(path)?;
        if existing.contains(pubkey) {
            return Ok(());
        }
    }
    let line = format!("command=\"remo git-hook --user {username}\" {pubkey}\n");
    let mut file = std::fs::OpenOptions::new()
        .create(true)
        .append(true)
        .open(path)?;
    file.write_all(line.as_bytes())
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

// ── Tests ─────────────────────────────────────────────────────────────────────

#[cfg(test)]
mod tests {
    use super::*;
    use axum::{body::Body, http::Request, middleware};
    use tower::ServiceExt;

    async fn make_state() -> Arc<AppState> {
        let pool = crate::db::open(":memory:").await.expect("in-memory db");
        Arc::new(AppState {
            pool,
            cfg: crate::config::ServerConfig {
                domain: "test.example".into(),
                data_dir: "/tmp".into(),
                nano_socket: "http://127.0.0.1:9999".into(),
                nano_admin_key: None,
                proxy: crate::config::ProxyBackend::Nginx,
                control_port: 7070,
                bind_addr: "127.0.0.1".into(),
            },
            master_token: "master-test-token".into(),
        })
    }

    fn test_router(state: Arc<AppState>) -> Router<()> {
        Router::new()
            .merge(public_routes())
            .merge(
                admin_routes()
                    .layer(middleware::from_fn(crate::server::auth::require_admin))
                    .layer(middleware::from_fn_with_state(
                        state.clone(),
                        crate::server::auth::require_auth,
                    )),
            )
            .with_state(state)
    }

    async fn call(
        router: &Router<()>,
        method: &str,
        path: &str,
        token: &str,
        body: serde_json::Value,
    ) -> (StatusCode, serde_json::Value) {
        let req = Request::builder()
            .method(method)
            .uri(path)
            .header("Authorization", format!("Bearer {token}"))
            .header("Content-Type", "application/json")
            .body(Body::from(body.to_string()))
            .unwrap();
        let resp = router.clone().oneshot(req).await.unwrap();
        let status = resp.status();
        let bytes = axum::body::to_bytes(resp.into_body(), 1 << 20).await.unwrap();
        let json = serde_json::from_slice(&bytes).unwrap_or(serde_json::Value::Null);
        (status, json)
    }

    #[tokio::test]
    async fn invite_create_and_claim() {
        let state = make_state().await;
        let router = test_router(state);

        let (status, body) = call(
            &router, "POST", "/api/admin/invites", "master-test-token",
            serde_json::json!({ "username": "alice", "email": "alice@example.com" }),
        ).await;
        assert_eq!(status, StatusCode::CREATED, "{body}");
        let token = body["token"].as_str().expect("token").to_string();
        assert!(body["claim_command"].as_str().unwrap().contains(&token));

        let (status, body) = call(
            &router, "POST", &format!("/api/invites/{token}/claim"), "",
            serde_json::json!({ "ssh_pubkey": "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAATEST alice" }),
        ).await;
        assert_eq!(status, StatusCode::CREATED, "{body}");
        assert_eq!(body["username"].as_str().unwrap(), "alice");
        assert!(!body["token"].as_str().unwrap().is_empty());
    }

    #[tokio::test]
    async fn invite_claim_twice_rejected() {
        let state = make_state().await;
        let router = test_router(state);

        let (_, body) = call(
            &router, "POST", "/api/admin/invites", "master-test-token",
            serde_json::json!({ "username": "bob" }),
        ).await;
        let token = body["token"].as_str().unwrap().to_string();
        let claim = serde_json::json!({ "ssh_pubkey": "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAATEST bob" });

        let (s, _) = call(&router, "POST", &format!("/api/invites/{token}/claim"), "", claim.clone()).await;
        assert_eq!(s, StatusCode::CREATED);
        let (s, b) = call(&router, "POST", &format!("/api/invites/{token}/claim"), "", claim).await;
        assert_eq!(s, StatusCode::CONFLICT, "second claim must fail: {b}");
    }

    #[tokio::test]
    async fn invite_invalid_token_is_404() {
        let state = make_state().await;
        let router = test_router(state);
        let (status, _) = call(
            &router, "POST", "/api/invites/deadbeef/claim", "",
            serde_json::json!({ "ssh_pubkey": "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAATEST x" }),
        ).await;
        assert_eq!(status, StatusCode::NOT_FOUND);
    }

    #[tokio::test]
    async fn invite_expired_token_rejected() {
        let state = make_state().await;
        let router = test_router(state);

        let (_, body) = call(
            &router, "POST", "/api/admin/invites", "master-test-token",
            serde_json::json!({ "username": "carol", "expires_in_secs": 0 }),
        ).await;
        let token = body["token"].as_str().unwrap().to_string();

        let (status, _) = call(
            &router, "POST", &format!("/api/invites/{token}/claim"), "",
            serde_json::json!({ "ssh_pubkey": "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAATEST carol" }),
        ).await;
        assert_eq!(status, StatusCode::BAD_REQUEST);
    }

    #[tokio::test]
    async fn invite_bad_pubkey_rejected() {
        let state = make_state().await;
        let router = test_router(state);

        let (_, body) = call(
            &router, "POST", "/api/admin/invites", "master-test-token",
            serde_json::json!({ "username": "dave" }),
        ).await;
        let token = body["token"].as_str().unwrap().to_string();

        let (status, _) = call(
            &router, "POST", &format!("/api/invites/{token}/claim"), "",
            serde_json::json!({ "ssh_pubkey": "not-a-valid-key blah" }),
        ).await;
        assert_eq!(status, StatusCode::BAD_REQUEST);
    }

    #[tokio::test]
    async fn invite_requires_admin() {
        let state = make_state().await;
        let router = test_router(state);
        let (status, _) = call(
            &router, "POST", "/api/admin/invites", "wrong-token",
            serde_json::json!({ "username": "eve" }),
        ).await;
        assert_eq!(status, StatusCode::UNAUTHORIZED);
    }
}
