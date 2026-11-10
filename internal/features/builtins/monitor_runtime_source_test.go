//go:build !windows

package builtins

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMonitorRuntimeEnvironmentUsesActiveBundledSource(t *testing.T) {
	root := t.TempDir()
	stale := t.TempDir()
	for _, test := range []struct {
		root    string
		content string
	}{
		{root: root, content: "import json, subprocess, sys\nrequest = json.load(sys.stdin)\nresult = subprocess.run([sys.executable, request['scripts'][0], *request['script_args'][0]], capture_output=True, text=True, check=True)\nprint(result.stdout, end='')\n"},
		{root: stale, content: "raise SystemExit('stale runtime was selected')\n"},
	} {
		packagePath := filepath.Join(test.root, "packages", "monitor_runtime", "monitor_runtime")
		if test.root == stale {
			packagePath = filepath.Join(test.root, "monitor_runtime")
		}
		if err := os.MkdirAll(packagePath, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(packagePath, "__init__.py"), nil, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(packagePath, "__main__.py"), []byte(test.content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	script := filepath.Join(root, "argv.py")
	if err := os.WriteFile(script, []byte("import sys\nprint(repr(sys.argv[1:]))\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal(err)
	}
	request, err := json.Marshal(map[string]any{"scripts": []string{script}, "script_args": [][]string{{"--epochs", "10", "first run"}}})
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(python, "-m", "monitor_runtime")
	command.Env = monitorRuntimeEnvironment(root, []string{"PATH=" + os.Getenv("PATH"), "PYTHONPATH=" + stale})
	command.Stdin = strings.NewReader(string(request))
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("active runtime failed: %v\n%s", err, output)
	}
	if string(output) != "['--epochs', '10', 'first run']\n" {
		t.Fatalf("target argv = %q", output)
	}
}
