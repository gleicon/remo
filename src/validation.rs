/// Returns true if `name` is a valid remo app name:
/// lowercase alphanumeric + hyphens, 1-32 chars, no leading/trailing hyphens.
pub fn is_valid_app_name(name: &str) -> bool {
    !name.is_empty()
        && name.len() <= 32
        && !name.starts_with('-')
        && !name.ends_with('-')
        && name.chars().all(|c| c.is_ascii_lowercase() || c.is_ascii_digit() || c == '-')
}

/// Returns true if `key` is a structurally valid SSH public key:
/// - Exactly one non-empty line (no newlines, no control characters)
/// - Starts with a known algorithm prefix
/// - Second field is non-empty base64 (alphanum + /+=)
/// Does NOT do a full cryptographic parse — use `ssh-key` crate if that's needed.
pub fn is_valid_ssh_pubkey(key: &str) -> bool {
    let key = key.trim();
    if key.is_empty() { return false; }
    // Reject any control characters (newline, CR, NUL, etc.) — these could inject extra lines
    if key.chars().any(|c| c.is_control()) { return false; }
    let mut parts = key.splitn(3, ' ');
    let algo = match parts.next() { Some(a) => a, None => return false };
    let b64 = match parts.next() { Some(b) => b, None => return false };
    let valid_algos = ["ssh-ed25519", "ssh-rsa", "ssh-ecdsa", "ecdsa-sha2-nistp256",
                       "ecdsa-sha2-nistp384", "ecdsa-sha2-nistp521", "ecdsa-sk-sha2-nistp256",
                       "sk-ssh-ed25519@openssh.com"];
    if !valid_algos.contains(&algo) { return false; }
    // base64 field must be non-empty and contain only valid base64 chars
    !b64.is_empty() && b64.chars().all(|c| c.is_ascii_alphanumeric() || matches!(c, '+' | '/' | '='))
}

/// Returns true if `ep` is a safe relative entrypoint path:
/// non-empty, no leading slash, no null bytes, each segment is non-empty,
/// not `..`, and uses only `[a-zA-Z0-9._-]` characters.
pub fn is_valid_entrypoint(ep: &str) -> bool {
    !ep.is_empty()
        && !ep.starts_with('/')
        && !ep.contains('\0')
        && ep.split('/').all(|seg| {
            !seg.is_empty()
                && seg != ".."
                && seg.chars().all(|c| c.is_ascii_alphanumeric() || matches!(c, '.' | '-' | '_'))
        })
}
