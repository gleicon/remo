/// Returns true if `name` is a valid remo app name:
/// lowercase alphanumeric + hyphens, 1-32 chars, no leading/trailing hyphens.
pub fn is_valid_app_name(name: &str) -> bool {
    !name.is_empty()
        && name.len() <= 32
        && !name.starts_with('-')
        && !name.ends_with('-')
        && name.chars().all(|c| c.is_ascii_lowercase() || c.is_ascii_digit() || c == '-')
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
