//! Integration test: `--export-html` produces valid, non-empty HTML.
//!
//! Runs the compiled binary against a temporary Markdown document and checks
//! that the output looks like a complete HTML document.

use std::path::Path;
use std::process::Command;

/// Resolve the path to the compiled debug binary.
fn binary() -> std::path::PathBuf {
    // `CARGO_BIN_EXE_tb-markdown-reader` is set by Cargo when running integration
    // tests for binaries in the same workspace package. Fall back to a relative
    // path so the test still works if run manually.
    if let Ok(p) = std::env::var("CARGO_BIN_EXE_tb-markdown-reader") {
        return p.into();
    }
    Path::new(env!("CARGO_MANIFEST_DIR"))
        .join("target")
        .join("debug")
        .join("tb-markdown-reader")
}

/// Happy-path: exporting Markdown produces a non-empty self-contained HTML
/// document.
#[test]
fn export_html_produces_valid_html_document() {
    let directory = tempfile::tempdir().expect("temporary directory must be created");
    let sample = directory.path().join("sample.md");
    std::fs::write(&sample, "# Sample\n\nBody.\n").expect("sample Markdown must be written");

    let output = Command::new(binary())
        .args(["--export-html", &sample.to_string_lossy()])
        .output()
        .expect("failed to run tb-markdown-reader binary");

    assert!(
        output.status.success(),
        "binary exited with non-zero status: {}\nstderr: {}",
        output.status,
        String::from_utf8_lossy(&output.stderr),
    );

    let html = String::from_utf8(output.stdout).expect("output is not valid UTF-8");

    assert!(!html.is_empty(), "expected non-empty HTML output");
    assert!(
        html.starts_with("<!DOCTYPE html>"),
        "output must start with DOCTYPE declaration"
    );
    assert!(
        html.contains("<style>"),
        "output must contain an inline <style> block"
    );
    assert!(
        html.contains("</html>"),
        "output must be a complete HTML document"
    );
    // The input contains one heading.
    assert!(
        html.contains("<h1") || html.contains("<h2"),
        "expected at least one heading element in output"
    );
}
