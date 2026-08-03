// Tests for input validation functions extracted from server/api.rs.
// These are pure-function tests — no DB, no network, no V8.

// ── App name validation ───────────────────────────────────────────────────────

fn validate_app_name(name: &str) -> bool {
    !name.is_empty()
        && name.len() <= 32
        && !name.starts_with('-')
        && !name.ends_with('-')
        && name.chars().all(|c| c.is_ascii_lowercase() || c.is_ascii_digit() || c == '-')
}

#[test]
fn valid_app_names() {
    for name in &["myapp", "my-app", "app123", "a", "x".repeat(32).as_str(), "hello-world-2"] {
        assert!(validate_app_name(name), "expected '{name}' to be valid");
    }
}

#[test]
fn invalid_app_names() {
    for (name, reason) in &[
        ("", "empty"),
        ("-myapp", "leading hyphen"),
        ("myapp-", "trailing hyphen"),
        ("MyApp", "uppercase"),
        ("my_app", "underscore"),
        ("my.app", "dot"),
        ("my/app", "slash"),
        ("my app", "space"),
        (&"x".repeat(33), "too long (33 chars)"),
        ("../etc/passwd", "path traversal"),
        ("app\x00name", "null byte"),
    ] {
        assert!(!validate_app_name(name), "expected '{name}' ({reason}) to be invalid");
    }
}

// ── Entrypoint validation ─────────────────────────────────────────────────────

fn is_valid_entrypoint(ep: &str) -> bool {
    !ep.is_empty()
        && !ep.starts_with('/')
        && !ep.contains('\0')
        && ep.split('/').all(|seg| {
            !seg.is_empty()
                && seg != ".."
                && seg.chars().all(|c| c.is_ascii_alphanumeric() || matches!(c, '.' | '-' | '_'))
        })
}

#[test]
fn valid_entrypoints() {
    for ep in &["index.js", "dist/main.js", "src/app.js", "a", "my-app_v2.js"] {
        assert!(is_valid_entrypoint(ep), "expected '{ep}' to be valid");
    }
}

#[test]
fn invalid_entrypoints() {
    for (ep, reason) in &[
        ("", "empty"),
        ("/index.js", "leading slash"),
        ("../index.js", "dot-dot traversal"),
        ("a/../index.js", "dot-dot mid-path"),
        ("a//b.js", "empty segment"),
        ("index.js\x00evil", "null byte"),
        ("index js", "space in name"),
        ("../../../etc/passwd", "full traversal"),
        ("/absolute/path", "absolute path"),
    ] {
        assert!(!is_valid_entrypoint(ep), "expected '{ep}' ({reason}) to be invalid");
    }
}

// ── SHA validation ────────────────────────────────────────────────────────────

fn validate_sha(sha: &str) -> bool {
    sha.len() == 16 && sha.chars().all(|c| c.is_ascii_hexdigit())
}

#[test]
fn valid_shas() {
    assert!(validate_sha("deadbeef01234567"));
    assert!(validate_sha("0000000000000000"));
    assert!(validate_sha("ffffffffffffffff"));
    assert!(validate_sha("ABCDEF0123456789")); // uppercase hex also valid
}

#[test]
fn invalid_shas() {
    assert!(!validate_sha(""));
    assert!(!validate_sha("deadbeef")); // too short
    assert!(!validate_sha("deadbeef012345678")); // too long (17)
    assert!(!validate_sha("deadbeefghijklmn")); // non-hex chars
    assert!(!validate_sha("deadbeef/0123456")); // slash
    assert!(!validate_sha("../etc/passwd000")); // path traversal
}

// ── safe_join (path segment traversal) ───────────────��───────────────────────

fn safe_join_ok(base: &str, segment: &str) -> bool {
    use std::path::{Component, Path};
    for comp in Path::new(segment).components() {
        match comp {
            Component::Normal(_) => {}
            _ => return false,
        }
    }
    let joined = Path::new(base).join(segment);
    joined.starts_with(base)
}

#[test]
fn safe_join_valid() {
    assert!(safe_join_ok("/var/lib/remo", "myapp"));
    assert!(safe_join_ok("/var/lib/remo", "abc123"));
    assert!(safe_join_ok("/var/lib/remo", "hello-world"));
}

#[test]
fn safe_join_traversal_rejected() {
    assert!(!safe_join_ok("/var/lib/remo", ".."));
    assert!(!safe_join_ok("/var/lib/remo", "../etc/passwd"));
    assert!(!safe_join_ok("/var/lib/remo", "app/../../etc"));
    assert!(!safe_join_ok("/var/lib/remo", "/absolute/path"));
}

// ── Auth: constant-time comparison ────────────────���──────────────────────────

fn constant_eq(a: &[u8], b: &[u8]) -> bool {
    if a.is_empty() || a.len() != b.len() {
        return false;
    }
    a.iter().zip(b.iter()).fold(0u8, |acc, (x, y)| acc | (x ^ y)) == 0
}

#[test]
fn constant_eq_matching() {
    assert!(constant_eq(b"secret", b"secret"));
    assert!(constant_eq(b"a", b"a"));
}

#[test]
fn constant_eq_mismatch() {
    assert!(!constant_eq(b"secret", b"secret!")); // different length
    assert!(!constant_eq(b"secret", b"SECRET")); // case differs
    assert!(!constant_eq(b"abc", b"abd")); // single byte differs
    assert!(!constant_eq(b"", b"")); // empty not equal (prevents empty token auth)
    assert!(!constant_eq(b"", b"nonempty"));
}

// ── Git hook: parse_app_name ────────────��─────────────────────���───────────────

fn parse_app_name(ssh_cmd: &str) -> Option<String> {
    let parts: Vec<&str> = ssh_cmd.split_whitespace().collect();
    let raw = parts.get(1)?;
    let name = raw.trim_matches('\'').trim_matches('"').trim_matches('/');

    let ok = !name.is_empty()
        && name.len() <= 32
        && !name.starts_with('-')
        && !name.ends_with('-')
        && name.chars().all(|c| c.is_ascii_lowercase() || c.is_ascii_digit() || c == '-');

    if ok { Some(name.to_string()) } else { None }
}

#[test]
fn parse_app_name_normal() {
    assert_eq!(parse_app_name("git-receive-pack 'myapp'"), Some("myapp".into()));
    assert_eq!(parse_app_name("git-receive-pack \"my-app\""), Some("my-app".into()));
}

#[test]
fn parse_app_name_strip_slashes() {
    assert_eq!(parse_app_name("git-receive-pack '/myapp'"), Some("myapp".into()));
}

#[test]
fn parse_app_name_injection_rejected() {
    assert_eq!(parse_app_name("git-receive-pack 'app;rm -rf /'"), None);
    assert_eq!(parse_app_name("git-receive-pack '../../../etc'"), None);
    assert_eq!(parse_app_name("git-receive-pack 'app$(id)'"), None);
    assert_eq!(parse_app_name("git-receive-pack ''"), None); // empty
}

#[test]
fn parse_app_name_missing_arg() {
    assert_eq!(parse_app_name("git-receive-pack"), None);
    assert_eq!(parse_app_name(""), None);
}

// ── SSH pubkey validation ─────────────────────────────────────────────────────

fn is_valid_ssh_pubkey(key: &str) -> bool {
    let key = key.trim();
    if key.is_empty() { return false; }
    if key.chars().any(|c| c.is_control()) { return false; }
    let mut parts = key.splitn(3, ' ');
    let algo = match parts.next() { Some(a) => a, None => return false };
    let b64 = match parts.next() { Some(b) => b, None => return false };
    let valid_algos = ["ssh-ed25519", "ssh-rsa", "ssh-ecdsa", "ecdsa-sha2-nistp256",
                       "ecdsa-sha2-nistp384", "ecdsa-sha2-nistp521", "ecdsa-sk-sha2-nistp256",
                       "sk-ssh-ed25519@openssh.com"];
    if !valid_algos.contains(&algo) { return false; }
    !b64.is_empty() && b64.chars().all(|c| c.is_ascii_alphanumeric() || matches!(c, '+' | '/' | '='))
}

#[test]
fn valid_ssh_pubkeys() {
    let keys = [
        "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIG user@host",
        "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIG",
        "ssh-rsa AAAAB3NzaC1yc2EAAAADAQAB user@laptop",
        "ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTYAAAA= comment",
    ];
    for key in &keys {
        assert!(is_valid_ssh_pubkey(key), "expected valid: {key}");
    }
}

#[test]
fn invalid_ssh_pubkeys_rejected() {
    let bad = [
        "",                                              // empty
        "not-a-key blah blah",                          // unknown algo
        "ssh-ed25519",                                   // missing base64 field
        "ssh-ed25519 AAAA\ncommand=\"\" evil",          // newline injection
        "ssh-ed25519 AAAA\rcommand=\"\" evil",          // carriage return injection
        "ssh-ed25519 AAAA\x00null",                     // null byte
        "ssh-ed25519 AAAA!!!invalid",                   // non-base64 chars in key field
    ];
    for key in &bad {
        assert!(!is_valid_ssh_pubkey(key), "expected invalid: {key:?}");
    }
}
