package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestAddOpenCodeSuperpowersPreservesConfigAndSkipsDuplicate(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "opencode.json")
	if err := os.WriteFile(path, []byte(`{"permission":"allow","plugins":["existing"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if installed, err := addOpenCodeSuperpowers(path, "plugins"); err != nil || !installed {
		t.Fatalf("installed=%v err=%v", installed, err)
	}
	if installed, err := addOpenCodeSuperpowers(path, "plugins"); err != nil || installed {
		t.Fatalf("second installed=%v err=%v", installed, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	if config["permission"] != "allow" {
		t.Fatalf("permission=%v", config["permission"])
	}
	entries := config["plugins"].([]any)
	if len(entries) != 2 || entries[0] != "existing" || entries[1] != openCodeSuperpowersPackage {
		t.Fatalf("plugins=%v", entries)
	}
}

func TestAddOpenCodeSuperpowersRejectsInvalidConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "opencode.json")
	if err := os.WriteFile(path, []byte(`{"plugins":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := addOpenCodeSuperpowers(path, "plugins"); err == nil {
		t.Fatal("expected invalid plugins error")
	}
	data, _ := os.ReadFile(path)
	if string(data) != `{"plugins":{}}` {
		t.Fatal("invalid config changed")
	}
}

func TestOpenCodePluginKey(t *testing.T) {
	for _, test := range []struct{ version, want string }{{"1.2.3", "plugin"}, {"opencode 2.0.4", "plugins"}, {"opencode v2.1.4", "plugins"}, {"2.0.3", ""}, {"invalid", ""}} {
		if got := openCodePluginKey(test.version); got != test.want {
			t.Fatalf("%q: %q, want %q", test.version, got, test.want)
		}
	}
}

func TestOpenCodeSuperpowersBuiltinWithFakeOpenCode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake executable uses POSIX shell")
	}
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "opencode"), []byte("#!/bin/sh\necho 'opencode v2.1.4'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	t.Setenv("OPENCODE_CONFIG_DIR", filepath.Join(root, "config"))
	builtins := NewToolboxBuiltins("", "linux-amd64", "0.1.1", io.Discard)
	if err := builtins.Run("install-superpowers-opencode", nil); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "config", "opencode.json")
	reason, err := builtins.SkipReason("install-superpowers-opencode")
	if err != nil || reason == "" {
		t.Fatalf("skip reason=%q err=%v", reason, err)
	}
	config, err := openCodeConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := openCodePluginEntries(config, "plugins")
	if err != nil || len(entries) != 1 || entries[0] != openCodeSuperpowersPackage {
		t.Fatalf("entries=%v err=%v", entries, err)
	}
}
