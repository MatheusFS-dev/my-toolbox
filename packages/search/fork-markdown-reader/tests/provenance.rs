use std::fs;
use std::path::{Path, PathBuf};
use std::process::Command;

const OFFICIAL_REPOSITORY: &str = "https://github.com/leboiko/markdown-reader";

fn vendored_root() -> PathBuf {
    PathBuf::from(env!("CARGO_MANIFEST_DIR"))
}

fn repository_root() -> PathBuf {
    vendored_root()
        .parent()
        .and_then(Path::parent)
        .and_then(Path::parent)
        .expect("vendored crate must live at packages/search/fork-markdown-reader")
        .to_path_buf()
}

fn collect_named(root: &Path, name: &str, matches: &mut Vec<PathBuf>) {
    for entry in fs::read_dir(root).expect("directory must be readable") {
        let path = entry.expect("directory entry must be readable").path();
        if path.file_name().is_some_and(|file_name| file_name == name) {
            matches.push(path.clone());
        }
        if path.is_dir() {
            collect_named(&path, name, matches);
        }
    }
}

fn contains_official_git_dependency(value: &toml::Value) -> bool {
    match value {
        toml::Value::Table(table) => {
            table
                .get("git")
                .and_then(toml::Value::as_str)
                .is_some_and(|url| url.trim_end_matches(".git") == OFFICIAL_REPOSITORY)
                || table.values().any(contains_official_git_dependency)
        }
        toml::Value::Array(items) => items.iter().any(contains_official_git_dependency),
        _ => false,
    }
}

fn binary() -> PathBuf {
    if let Ok(path) = std::env::var("CARGO_BIN_EXE_tb-markdown-reader") {
        return path.into();
    }
    vendored_root()
        .join("target")
        .join("debug")
        .join("tb-markdown-reader")
}

#[test]
fn vendored_snapshot_has_no_nested_git_directory() {
    let mut nested_git = Vec::new();
    collect_named(&vendored_root(), ".git", &mut nested_git);
    assert!(
        nested_git.is_empty(),
        "vendored snapshot must not contain nested .git entries: {nested_git:?}"
    );
}

#[test]
fn vendored_reader_is_not_a_submodule() {
    let gitmodules = fs::read_to_string(repository_root().join(".gitmodules"))
        .expect("toolbox .gitmodules must remain readable");
    assert!(
        !gitmodules.contains("packages/search/fork-markdown-reader")
            && !gitmodules.contains("toolbox-reader"),
        "vendored reader must not be registered as a submodule"
    );
}

#[test]
fn cargo_manifests_do_not_fetch_the_official_repository() {
    let mut manifests = Vec::new();
    collect_named(&vendored_root(), "Cargo.toml", &mut manifests);

    for manifest in manifests {
        let text = fs::read_to_string(&manifest).expect("Cargo manifest must be readable");
        let parsed: toml::Value = toml::from_str(&text).expect("Cargo manifest must parse");
        assert!(
            !contains_official_git_dependency(&parsed),
            "{} must not depend on the official repository via Git",
            manifest.display()
        );
    }
}

#[test]
fn fork_identity_and_update_isolation_are_explicit() {
    let manifest_text = fs::read_to_string(vendored_root().join("Cargo.toml")).unwrap();
    let manifest: toml::Value = toml::from_str(&manifest_text).unwrap();
    assert_eq!(manifest["package"]["name"].as_str(), Some("tb-markdown-reader"));
    assert_eq!(manifest["bin"][0]["name"].as_str(), Some("tb-markdown-reader"));

    let main = fs::read_to_string(vendored_root().join("src/main.rs")).unwrap();
    assert!(main.contains("name = \"tb-markdown-reader\""));
    assert!(!main.contains("mod version_check;"));
    assert!(!main.contains("version_check::"));
}

#[test]
fn clap_help_uses_only_the_fork_binary_identity() {
    let output = Command::new(binary())
        .arg("--help")
        .output()
        .expect("fork binary must run");
    assert!(output.status.success());

    let help = String::from_utf8(output.stdout).expect("Clap help must be UTF-8");
    assert!(help.contains("Usage: tb-markdown-reader"));
    assert!(
        !help.contains("| markdown-reader"),
        "Clap help must not advertise the upstream binary name"
    );
}
