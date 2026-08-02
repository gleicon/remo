use axum::{
    body::Body,
    extract::State,
    http::{Request, StatusCode},
    middleware::Next,
    response::Response,
};
use std::sync::Arc;

use crate::server::AppState;

/// Identity inserted into request extensions after successful auth.
#[derive(Clone, Debug)]
pub struct AuthUser {
    pub name: String,
    pub is_admin: bool,
}

/// Outer middleware: validates bearer token, inserts `AuthUser` extension.
pub async fn require_auth(
    State(state): State<Arc<AppState>>,
    mut req: Request<Body>,
    next: Next,
) -> Result<Response, StatusCode> {
    let token = bearer_token(req.headers()).ok_or(StatusCode::UNAUTHORIZED)?;
    let user = resolve_token(&state, token)
        .await
        .ok_or(StatusCode::UNAUTHORIZED)?;
    req.extensions_mut().insert(user);
    Ok(next.run(req).await)
}

/// Inner middleware: requires `AuthUser.is_admin`. Must run after `require_auth`.
pub async fn require_admin(req: Request<Body>, next: Next) -> Result<Response, StatusCode> {
    let user = req
        .extensions()
        .get::<AuthUser>()
        .ok_or(StatusCode::UNAUTHORIZED)?;
    if !user.is_admin {
        return Err(StatusCode::FORBIDDEN);
    }
    Ok(next.run(req).await)
}

fn bearer_token(headers: &axum::http::HeaderMap) -> Option<&str> {
    headers
        .get(axum::http::header::AUTHORIZATION)?
        .to_str()
        .ok()?
        .strip_prefix("Bearer ")
}

async fn resolve_token(state: &AppState, token: &str) -> Option<AuthUser> {
    // Master token → admin identity (constant-time compare).
    if constant_eq(token.as_bytes(), state.master_token.as_bytes()) {
        return Some(AuthUser { name: "admin".into(), is_admin: true });
    }

    // User tokens: O(1) SHA-256 hash lookup (API tokens have 32-byte random entropy,
    // so bcrypt slowness is not needed and makes auth a DoS vector).
    let hash = sha256_hex(token);
    let user = crate::db::user_get_by_token_hash(&state.pool, &hash).await.ok()??;
    Some(AuthUser { name: user.name, is_admin: user.is_admin })
}

pub fn sha256_hex(data: &str) -> String {
    use sha2::{Digest, Sha256};
    hex::encode(Sha256::digest(data.as_bytes()))
}

fn constant_eq(a: &[u8], b: &[u8]) -> bool {
    if a.is_empty() || a.len() != b.len() {
        return false;
    }
    a.iter().zip(b.iter()).fold(0u8, |acc, (x, y)| acc | (x ^ y)) == 0
}
