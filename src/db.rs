use anyhow::Result;
use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use sqlx::{sqlite::SqlitePool, Row};

pub async fn open(path: &str) -> Result<SqlitePool> {
    let pool = SqlitePool::connect(&format!("sqlite:{path}?mode=rwc")).await?;
    migrate(&pool).await?;
    Ok(pool)
}

async fn migrate(pool: &SqlitePool) -> Result<()> {
    // Safe on existing DBs: ignore errors (column already exists, table already dropped).
    let _ = sqlx::query("ALTER TABLE deployments ADD COLUMN sha TEXT")
        .execute(pool)
        .await;
    let _ = sqlx::query("DROP TABLE IF EXISTS nodes")
        .execute(pool)
        .await;

    sqlx::query(
        r#"
        CREATE TABLE IF NOT EXISTS apps (
            id            TEXT PRIMARY KEY,
            hostname      TEXT NOT NULL UNIQUE,
            owner         TEXT NOT NULL,
            app_type      TEXT NOT NULL DEFAULT 'js',
            entrypoint    TEXT NOT NULL,
            current_sha   TEXT,
            custom_domain TEXT,
            created_at    TEXT NOT NULL,
            updated_at    TEXT NOT NULL
        );

        CREATE TABLE IF NOT EXISTS users (
            id          TEXT PRIMARY KEY,
            name        TEXT NOT NULL UNIQUE,
            token_hash  TEXT NOT NULL,
            ssh_pubkey  TEXT,
            is_admin    INTEGER NOT NULL DEFAULT 0,
            created_at  TEXT NOT NULL
        );

        CREATE TABLE IF NOT EXISTS deployments (
            id          TEXT PRIMARY KEY,
            app_id      TEXT NOT NULL REFERENCES apps(id),
            deployer    TEXT NOT NULL,
            sha         TEXT,
            status      TEXT NOT NULL DEFAULT 'pending',
            log_output  TEXT,
            created_at  TEXT NOT NULL
        );

        CREATE TABLE IF NOT EXISTS env_vars (
            app_id      TEXT NOT NULL REFERENCES apps(id),
            key         TEXT NOT NULL,
            value_enc   TEXT NOT NULL,
            PRIMARY KEY (app_id, key)
        );

        CREATE TABLE IF NOT EXISTS invites (
            id          TEXT PRIMARY KEY,
            token_hash  TEXT NOT NULL UNIQUE,
            username    TEXT NOT NULL,
            email       TEXT,
            expires_at  TEXT NOT NULL,
            used_at     TEXT
        );
        "#,
    )
    .execute(pool)
    .await?;
    Ok(())
}

// ── App ──────────────────────────────────────────────────────────────────────

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct App {
    pub id: String,
    pub hostname: String,
    pub owner: String,
    pub app_type: String,
    pub entrypoint: String,
    pub current_sha: Option<String>,
    /// User-configured CNAME target. When set, the proxy serves this domain
    /// in addition to the canonical `{owner}-{name}.{domain}` hostname.
    pub custom_domain: Option<String>,
    pub created_at: DateTime<Utc>,
    pub updated_at: DateTime<Utc>,
}

pub async fn app_create(pool: &SqlitePool, app: &App) -> Result<()> {
    sqlx::query(
        "INSERT INTO apps (id,hostname,owner,app_type,entrypoint,current_sha,custom_domain,created_at,updated_at)
         VALUES (?,?,?,?,?,?,?,?,?)",
    )
    .bind(&app.id)
    .bind(&app.hostname)
    .bind(&app.owner)
    .bind(&app.app_type)
    .bind(&app.entrypoint)
    .bind(&app.current_sha)
    .bind(&app.custom_domain)
    .bind(app.created_at.to_rfc3339())
    .bind(app.updated_at.to_rfc3339())
    .execute(pool)
    .await?;
    Ok(())
}

pub async fn app_set_custom_domain(
    pool: &SqlitePool,
    name: &str,
    domain: Option<&str>,
) -> Result<()> {
    sqlx::query("UPDATE apps SET custom_domain=?, updated_at=? WHERE id=?")
        .bind(domain)
        .bind(Utc::now().to_rfc3339())
        .bind(name)
        .execute(pool)
        .await?;
    Ok(())
}

pub async fn app_get(pool: &SqlitePool, name: &str) -> Result<Option<App>> {
    let row = sqlx::query("SELECT * FROM apps WHERE id=?")
        .bind(name)
        .fetch_optional(pool)
        .await?;
    Ok(row.map(row_to_app))
}

pub async fn app_list(pool: &SqlitePool) -> Result<Vec<App>> {
    let rows = sqlx::query("SELECT * FROM apps ORDER BY created_at")
        .fetch_all(pool)
        .await?;
    Ok(rows.into_iter().map(row_to_app).collect())
}

pub async fn app_list_by_owner(pool: &SqlitePool, owner: &str) -> Result<Vec<App>> {
    let rows = sqlx::query("SELECT * FROM apps WHERE owner=? ORDER BY created_at")
        .bind(owner)
        .fetch_all(pool)
        .await?;
    Ok(rows.into_iter().map(row_to_app).collect())
}

pub async fn app_delete(pool: &SqlitePool, name: &str) -> Result<bool> {
    let n = sqlx::query("DELETE FROM apps WHERE id=?")
        .bind(name)
        .execute(pool)
        .await?
        .rows_affected();
    Ok(n > 0)
}

pub async fn app_set_sha(pool: &SqlitePool, name: &str, sha: &str) -> Result<()> {
    sqlx::query("UPDATE apps SET current_sha=?, updated_at=? WHERE id=?")
        .bind(sha)
        .bind(Utc::now().to_rfc3339())
        .bind(name)
        .execute(pool)
        .await?;
    Ok(())
}

fn row_to_app(row: sqlx::sqlite::SqliteRow) -> App {
    App {
        id: row.get("id"),
        hostname: row.get("hostname"),
        owner: row.get("owner"),
        app_type: row.get("app_type"),
        entrypoint: row.get("entrypoint"),
        current_sha: row.get("current_sha"),
        custom_domain: row.get("custom_domain"),
        created_at: row
            .get::<String, _>("created_at")
            .parse()
            .unwrap_or(Utc::now()),
        updated_at: row
            .get::<String, _>("updated_at")
            .parse()
            .unwrap_or(Utc::now()),
    }
}

// ── User ─────────────────────────────────────────────────────────────────────

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct User {
    pub id: String,
    pub name: String,
    pub token_hash: String,
    pub ssh_pubkey: Option<String>,
    pub is_admin: bool,
    pub created_at: DateTime<Utc>,
}

pub async fn user_create(pool: &SqlitePool, user: &User) -> Result<()> {
    sqlx::query(
        "INSERT INTO users (id,name,token_hash,ssh_pubkey,is_admin,created_at)
         VALUES (?,?,?,?,?,?)",
    )
    .bind(&user.id)
    .bind(&user.name)
    .bind(&user.token_hash)
    .bind(&user.ssh_pubkey)
    .bind(user.is_admin as i32)
    .bind(user.created_at.to_rfc3339())
    .execute(pool)
    .await?;
    Ok(())
}

pub async fn user_get_by_name(pool: &SqlitePool, name: &str) -> Result<Option<User>> {
    let row = sqlx::query("SELECT * FROM users WHERE name=?")
        .bind(name)
        .fetch_optional(pool)
        .await?;
    Ok(row.map(row_to_user))
}

pub async fn user_get_by_token_hash(pool: &SqlitePool, hash: &str) -> Result<Option<User>> {
    let row = sqlx::query("SELECT * FROM users WHERE token_hash=?")
        .bind(hash)
        .fetch_optional(pool)
        .await?;
    Ok(row.map(row_to_user))
}

pub async fn user_list(pool: &SqlitePool) -> Result<Vec<User>> {
    let rows = sqlx::query("SELECT * FROM users ORDER BY name")
        .fetch_all(pool)
        .await?;
    Ok(rows.into_iter().map(row_to_user).collect())
}

pub async fn user_delete(pool: &SqlitePool, name: &str) -> Result<bool> {
    let n = sqlx::query("DELETE FROM users WHERE name=?")
        .bind(name)
        .execute(pool)
        .await?
        .rows_affected();
    Ok(n > 0)
}

fn row_to_user(row: sqlx::sqlite::SqliteRow) -> User {
    User {
        id: row.get("id"),
        name: row.get("name"),
        token_hash: row.get("token_hash"),
        ssh_pubkey: row.get("ssh_pubkey"),
        is_admin: row.get::<i32, _>("is_admin") != 0,
        created_at: row
            .get::<String, _>("created_at")
            .parse()
            .unwrap_or(Utc::now()),
    }
}

// ── Deployment ───────────────────────────────────────────────────────────────

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Deployment {
    pub id: String,
    pub app_id: String,
    pub deployer: String,
    pub sha: Option<String>,
    pub status: String,
    pub log_output: Option<String>,
    pub created_at: DateTime<Utc>,
}

pub async fn deployment_create(pool: &SqlitePool, d: &Deployment) -> Result<()> {
    sqlx::query(
        "INSERT INTO deployments (id,app_id,deployer,sha,status,log_output,created_at)
         VALUES (?,?,?,?,?,?,?)",
    )
    .bind(&d.id)
    .bind(&d.app_id)
    .bind(&d.deployer)
    .bind(&d.sha)
    .bind(&d.status)
    .bind(&d.log_output)
    .bind(d.created_at.to_rfc3339())
    .execute(pool)
    .await?;
    Ok(())
}

pub async fn deployment_update_status(
    pool: &SqlitePool,
    id: &str,
    status: &str,
    sha: Option<&str>,
    log: Option<&str>,
) -> Result<()> {
    sqlx::query("UPDATE deployments SET status=?, sha=?, log_output=? WHERE id=?")
        .bind(status)
        .bind(sha)
        .bind(log)
        .bind(id)
        .execute(pool)
        .await?;
    Ok(())
}

pub async fn deployments_for_app(pool: &SqlitePool, app_id: &str) -> Result<Vec<Deployment>> {
    let rows = sqlx::query(
        "SELECT * FROM deployments WHERE app_id=? ORDER BY created_at DESC LIMIT 20",
    )
    .bind(app_id)
    .fetch_all(pool)
    .await?;
    Ok(rows.into_iter().map(row_to_deployment).collect())
}

fn row_to_deployment(row: sqlx::sqlite::SqliteRow) -> Deployment {
    Deployment {
        id: row.get("id"),
        app_id: row.get("app_id"),
        deployer: row.get("deployer"),
        sha: row.get("sha"),
        status: row.get("status"),
        log_output: row.get("log_output"),
        created_at: row
            .get::<String, _>("created_at")
            .parse()
            .unwrap_or(Utc::now()),
    }
}

// ── Env vars ─────────────────────────────────────────────────────────────────

pub async fn env_set(pool: &SqlitePool, app_id: &str, key: &str, value_enc: &str) -> Result<()> {
    sqlx::query(
        "INSERT INTO env_vars (app_id,key,value_enc) VALUES (?,?,?)
         ON CONFLICT(app_id,key) DO UPDATE SET value_enc=excluded.value_enc",
    )
    .bind(app_id)
    .bind(key)
    .bind(value_enc)
    .execute(pool)
    .await?;
    Ok(())
}

pub async fn env_list(pool: &SqlitePool, app_id: &str) -> Result<Vec<(String, String)>> {
    let rows = sqlx::query("SELECT key, value_enc FROM env_vars WHERE app_id=? ORDER BY key")
        .bind(app_id)
        .fetch_all(pool)
        .await?;
    Ok(rows
        .into_iter()
        .map(|r| (r.get::<String, _>("key"), r.get::<String, _>("value_enc")))
        .collect())
}

/// Load env vars from DB and base64-decode values. Silently skips entries that
/// fail to decode (they were never correctly stored).
pub async fn env_list_decoded(
    pool: &SqlitePool,
    app_id: &str,
) -> Result<std::collections::HashMap<String, String>> {
    use base64::Engine as _;
    let pairs = env_list(pool, app_id).await?;
    Ok(pairs
        .into_iter()
        .filter_map(|(k, enc)| {
            let raw = base64::engine::general_purpose::STANDARD.decode(&enc).ok()?;
            let v = String::from_utf8(raw).ok()?;
            Some((k, v))
        })
        .collect())
}

pub async fn env_unset(pool: &SqlitePool, app_id: &str, key: &str) -> Result<bool> {
    let n = sqlx::query("DELETE FROM env_vars WHERE app_id=? AND key=?")
        .bind(app_id)
        .bind(key)
        .execute(pool)
        .await?
        .rows_affected();
    Ok(n > 0)
}

// ── Invite ────────────────────────────────────────────────────────────────────

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Invite {
    pub id: String,
    pub token_hash: String,
    pub username: String,
    pub email: Option<String>,
    pub expires_at: DateTime<Utc>,
    pub used_at: Option<DateTime<Utc>>,
}

pub async fn invite_create(pool: &SqlitePool, invite: &Invite) -> Result<()> {
    sqlx::query(
        "INSERT INTO invites (id,token_hash,username,email,expires_at,used_at)
         VALUES (?,?,?,?,?,?)",
    )
    .bind(&invite.id)
    .bind(&invite.token_hash)
    .bind(&invite.username)
    .bind(&invite.email)
    .bind(invite.expires_at.to_rfc3339())
    .bind(invite.used_at.map(|t| t.to_rfc3339()))
    .execute(pool)
    .await?;
    Ok(())
}

pub async fn invite_get_by_token_hash(pool: &SqlitePool, hash: &str) -> Result<Option<Invite>> {
    let row = sqlx::query("SELECT * FROM invites WHERE token_hash=?")
        .bind(hash)
        .fetch_optional(pool)
        .await?;
    Ok(row.map(row_to_invite))
}

pub async fn invite_mark_used(pool: &SqlitePool, id: &str) -> Result<()> {
    sqlx::query("UPDATE invites SET used_at=? WHERE id=?")
        .bind(Utc::now().to_rfc3339())
        .bind(id)
        .execute(pool)
        .await?;
    Ok(())
}

pub async fn invite_list(pool: &SqlitePool) -> Result<Vec<Invite>> {
    let rows = sqlx::query("SELECT * FROM invites ORDER BY expires_at DESC")
        .fetch_all(pool)
        .await?;
    Ok(rows.into_iter().map(row_to_invite).collect())
}

fn row_to_invite(row: sqlx::sqlite::SqliteRow) -> Invite {
    Invite {
        id: row.get("id"),
        token_hash: row.get("token_hash"),
        username: row.get("username"),
        email: row.get("email"),
        expires_at: row.get::<String, _>("expires_at").parse().unwrap_or(Utc::now()),
        used_at: row.get::<Option<String>, _>("used_at")
            .and_then(|s| s.parse().ok()),
    }
}
