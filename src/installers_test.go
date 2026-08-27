package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPluginPreflightAllowsPrecedingFreshAgentInstallation(t *testing.T) {
	// Configuration happens before any selected tool runs, so an absent agent
	// cannot reject a batch that selected its installer first.
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir())
	builtins := NewToolboxBuiltins("", "linux-amd64", "0.1.1", io.Discard)
	reason, err := builtins.SkipReason("install-superpowers-codex")
	if err != nil || reason != "" {
		t.Fatalf("SkipReason() = %q, %v", reason, err)
	}
}

func TestPluginPreflightRejectsInstalledAgentWithoutPluginManagement(t *testing.T) {
	bin := t.TempDir()
	path := filepath.Join(bin, "codex")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	builtins := NewToolboxBuiltins("", "linux-amd64", "0.1.1", io.Discard)
	_, err := builtins.SkipReason("install-superpowers-codex")
	if err == nil || !strings.Contains(err.Error(), "plugin-management") {
		t.Fatalf("SkipReason() error = %v", err)
	}
}

func TestPluginRunFindsFreshAgentOutsideCurrentPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", t.TempDir())
	outputPath := filepath.Join(home, "arguments.txt")
	t.Setenv("TOOLBOX_TEST_OUTPUT", outputPath)
	bin := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	agentPath := filepath.Join(bin, "codex")
	if err := os.WriteFile(agentPath, []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" > \"$TOOLBOX_TEST_OUTPUT\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := runAgentPlugin("codex", []string{"plugin", "add", "superpowers@openai-curated"}, "linux-amd64", io.Discard); err != nil {
		t.Fatal(err)
	}
	arguments, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(arguments) != "plugin add superpowers@openai-curated\n" {
		t.Fatalf("plugin arguments = %q", arguments)
	}
}

func TestDirectoryOnPathUsesWindowsCaseInsensitiveComparison(t *testing.T) {
	t.Setenv("PATH", strings.Join([]string{filepath.Join("Users", "Name", "Bin"), "/usr/bin"}, string(os.PathListSeparator)))
	if !directoryOnPath(filepath.Join("users", "name", "bin"), true) {
		t.Fatal("directoryOnPath() missed case-insensitive Windows path")
	}
}
