use std::env;
use std::path::PathBuf;

pub fn dir() -> PathBuf {
    PathBuf::from(env::var("NOTES_DIR").unwrap_or_default())
}

pub fn flag(name: &str) -> bool {
    env::var_os(name).is_some()
}
