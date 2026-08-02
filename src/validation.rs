/// Returns true if `name` is a valid remo app name:
/// lowercase alphanumeric + hyphens, 1-32 chars, no leading/trailing hyphens.
pub fn is_valid_app_name(name: &str) -> bool {
    !name.is_empty()
        && name.len() <= 32
        && !name.starts_with('-')
        && !name.ends_with('-')
        && name.chars().all(|c| c.is_ascii_lowercase() || c.is_ascii_digit() || c == '-')
}
