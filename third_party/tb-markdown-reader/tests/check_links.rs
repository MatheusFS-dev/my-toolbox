//! Integration test: `--check-links` validates markdown links in a directory.
//!
//! Creates a temporary directory with two `.md` files — one that has a mix of
//! valid and broken links, one that serves as a link target — then invokes the
//! compiled binary and asserts the exit code and output format.

use std::fs;
use std::path::{Path, PathBuf};
use std::process::Command;

/// Resolve the path to the compiled debug binary.
fn binary() -> PathBuf {
    // `CARGO_BIN_EXE_tb-markdown-reader` is set by Cargo when running integration
    // tests for binaries in the same workspace package. Fall back to a relative
    // path for manual invocation.
    if let Ok(p) = std::env::var("CARGO_BIN_EXE_tb-markdown-reader") {
        return p.into();
    }
    Path::new(env!("CARGO_MANIFEST_DIR"))
        .join("target")
        .join("debug")
        .join("tb-markdown-reader")
}

/// Happy path: a directory with only valid links exits with status 0.
#[test]
fn check_links_exits_zero_when_all_links_valid() {
    let dir = tempfile::tempdir().expect("tempdir");
    let root = dir.path();

    fs::write(root.join("target.md"), "# Target Section\n\nContent.\n").unwrap();
    fs::write(
        root.join("source.md"),
        "# Source\n\n[cross](./target.md#target-section)\n\n[anchor](#source)\n",
    )
    .unwrap();

    let output = Command::new(binary())
        .args(["--check-links", &root.to_string_lossy()])
        .output()
        .expect("failed to run binary");

    assert!(
        output.status.success(),
        "expected exit 0 for valid links; exit={}\nstdout={}",
        output.status,
        String::from_utf8_lossy(&output.stdout),
    );

    let stdout = String::from_utf8(output.stdout).unwrap();
    assert!(
        stdout.contains("All links OK"),
        "expected 'All links OK' in output:\n{stdout}"
    );
}

/// Broken path: exit code is 1 and stdout names the broken link.
#[test]
fn check_links_exits_one_and_reports_broken_link() {
    let dir = tempfile::tempdir().expect("tempdir");
    let root = dir.path();

    // `target.md` has one heading; `source.md` links to a non-existent anchor.
    fs::write(root.join("target.md"), "# Real Section\n\nContent.\n").unwrap();
    fs::write(
        root.join("source.md"),
        "# Source\n\n[bad cross](./target.md#does-not-exist)\n\n[bad anchor](#nowhere)\n",
    )
    .unwrap();

    let output = Command::new(binary())
        .args(["--check-links", &root.to_string_lossy()])
        .output()
        .expect("failed to run binary");

    assert_eq!(
        output.status.code(),
        Some(1),
        "expected exit code 1 for broken links;\nstdout={}",
        String::from_utf8_lossy(&output.stdout),
    );

    let stdout = String::from_utf8(output.stdout).unwrap();

    // The tool should mention the broken anchor in source.md.
    assert!(
        stdout.contains("#does-not-exist") || stdout.contains("does-not-exist"),
        "expected broken cross-file anchor in output:\n{stdout}"
    );
    assert!(
        stdout.contains("#nowhere") || stdout.contains("nowhere"),
        "expected broken same-file anchor in output:\n{stdout}"
    );
    // Summary line should appear.
    assert!(
        stdout.contains("broken link"),
        "expected 'broken link' summary in output:\n{stdout}"
    );
}
