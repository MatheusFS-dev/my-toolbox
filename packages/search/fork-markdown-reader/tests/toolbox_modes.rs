use std::process::{Command, Stdio};

fn command() -> Command {
    let mut command = Command::new(env!("CARGO_BIN_EXE_tb-markdown-reader"));
    command.stdin(Stdio::null());
    command.env_remove("DISPLAY").env_remove("WAYLAND_DISPLAY");
    command
}

#[test]
fn toolbox_self_check_is_terminal_free_and_exact() {
    let root = tempfile::tempdir().unwrap();
    let output = command()
        .arg("--tb-self-check")
        .env("XDG_CONFIG_HOME", root.path().join("config"))
        .env("XDG_STATE_HOME", root.path().join("state"))
        .output()
        .unwrap();
    assert!(output.status.success(), "{:?}", output);
    assert_eq!(output.stdout, b"tb-markdown-reader: ok\n");
    assert!(output.stderr.is_empty(), "{:?}", output);
    assert_eq!(std::fs::read_dir(root.path()).unwrap().count(), 0);
}

#[test]
fn toolbox_rejects_bad_contracts_before_terminal_or_stdin_handling() {
    let root = tempfile::tempdir().unwrap();
    let file = root.path().join("readme.md");
    std::fs::write(&file, "# Read me\n").unwrap();
    let file = file.to_str().unwrap();
    let dir = root.path().to_str().unwrap();
    for args in [
        vec!["--tb-embedded"],
        vec!["--tb-embedded", file, file],
        vec!["--tb-embedded", "/does-not-exist/toolbox.md"],
        vec!["--tb-embedded", dir],
        vec!["--tb-embedded", file, "--section", "Read"],
        vec!["--tb-self-check", file],
        vec!["--tb-self-check", "--tb-embedded", file],
    ] {
        let output = command().args(&args).output().unwrap();
        assert!(!output.status.success(), "{args:?}");
        let error = String::from_utf8_lossy(&output.stderr);
        assert!(!error.contains("unexpected argument '--tb-"), "{error}");
        assert!(!error.contains("/dev/tty"), "{error}");
        assert!(output.stdout.is_empty(), "{args:?}: {:?}", output.stdout);
    }

    for (path, message) in [
        (
            "/does-not-exist/toolbox.md",
            "could not resolve embedded file",
        ),
        (dir, "embedded input must be a regular file"),
    ] {
        let output = command().args(["--tb-embedded", path]).output().unwrap();
        assert!(String::from_utf8_lossy(&output.stderr).contains(message));
    }
    std::fs::write(file, [0xff]).unwrap();
    let output = command().args(["--tb-embedded", file]).output().unwrap();
    assert!(String::from_utf8_lossy(&output.stderr).contains("could not read embedded file"));
    assert!(output.stdout.is_empty());
}
