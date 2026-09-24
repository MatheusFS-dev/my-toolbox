package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func refreshTestApp(t *testing.T, ui *fakeUI, executor *fakeExecutor, names ...string) (App, string, *bytes.Buffer) {
	t.Helper()
	if executor.arguments == nil {
		executor.arguments = map[string][]string{}
	}
	base := t.TempDir()
	t.Setenv("XDG_DATA_HOME", base)
	t.Setenv("HOME", t.TempDir())
	dataRoot := filepath.Join(base, "my-toolbox")
	writeActiveTestCatalog(t, dataRoot, "0.1.1", testCatalog(names...))
	output := &bytes.Buffer{}
	return App{Catalog: testCatalog(names...), Environment: "linux-native", Platform: "linux-amd64", UI: ui, Executor: executor, Output: output}, dataRoot, output
}

func writeActiveTestCatalog(t *testing.T, dataRoot, version string, catalog Catalog) {
	t.Helper()
	reference, err := LoadCatalogFile("../commands.json")
	if err != nil {
		t.Fatal(err)
	}
	commands := make([]Command, 0, len(catalog.Commands))
	for _, requested := range catalog.Commands {
		command, exists := reference.Find(requested.Name)
		if !exists {
			t.Fatalf("missing fixture command %s", requested.Name)
		}
		commands = append(commands, command)
	}
	versionRoot := filepath.Join(dataRoot, "versions", version)
	if err := os.MkdirAll(versionRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	content, err := json.Marshal(Catalog{Commands: commands})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(versionRoot, "commands.json"), content, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataRoot, "current.txt"), []byte(version+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestOnlyGlobalAgentSetupsAreRemembered(t *testing.T) {
	executor := &fakeExecutor{responses: map[string][]ProtocolResponse{"install-codex": {{Status: "skipped", Reason: "already installed"}}}}
	ui := &fakeUI{selected: []string{"setup-agents-codex", "setup-agents-claude", "setup-agents-antigravity", "setup-agents-project", "install-codex", "install-monitor", "install-superpowers-codex"}}
	app, dataRoot, _ := refreshTestApp(t, ui, executor, "setup-agents-codex", "setup-agents-claude", "setup-agents-antigravity", "setup-agents-project", "install-codex", "install-monitor", "install-superpowers-codex")
	if err := app.Execute([]string{"list"}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"setup-agents-codex", "setup-agents-claude", "setup-agents-antigravity"} {
		if _, err := os.Stat(filepath.Join(dataRoot, "used-tools", name)); err != nil {
			t.Fatalf("marker %s: %v", name, err)
		}
	}
	for _, name := range []string{"setup-agents-project", "install-codex", "install-monitor", "install-superpowers-codex"} {
		if _, err := os.Stat(filepath.Join(dataRoot, "used-tools", name)); !os.IsNotExist(err) {
			t.Fatalf("unexpected marker %s: %v", name, err)
		}
	}
}

func TestFailedAndCancelledCommandsAreNotRemembered(t *testing.T) {
	for _, tc := range []struct {
		name     string
		ui       *fakeUI
		executor *fakeExecutor
	}{
		{"failure", &fakeUI{}, &fakeExecutor{fail: "setup-agents-codex"}},
		{"cancellation", &fakeUI{err: ErrCancelled}, &fakeExecutor{responses: map[string][]ProtocolResponse{"setup-agents-codex": {{Status: "question", Question: &Question{ID: "choice", Type: "confirm", Title: "Continue?"}}}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, dataRoot, _ := refreshTestApp(t, tc.ui, tc.executor, "setup-agents-codex")
			if err := app.Execute([]string{"setup-agents-codex"}); err == nil {
				t.Fatal("expected error")
			}
			if _, err := os.Stat(filepath.Join(dataRoot, "used-tools", "setup-agents-codex")); !os.IsNotExist(err) {
				t.Fatalf("unexpected marker: %v", err)
			}
		})
	}
}

func TestUpdateOffersRememberedCommandsInCatalogOrder(t *testing.T) {
	ui := &fakeUI{answers: map[string]any{"refresh-remembered-tools": true}}
	executor := &fakeExecutor{}
	app, dataRoot, output := refreshTestApp(t, ui, executor, "setup-agents-codex", "setup-agents-claude", "setup-agents-antigravity")
	for _, name := range []string{"setup-agents-antigravity", "setup-agents-claude", "setup-agents-codex"} {
		path := filepath.Join(dataRoot, "used-tools", name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	runs := []string{}
	app.refreshRunner = func(name string) error { runs = append(runs, name); return nil }
	if err := app.Execute([]string{"update"}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(runs, []string{"setup-agents-codex", "setup-agents-claude", "setup-agents-antigravity"}) {
		t.Fatalf("runs = %v", runs)
	}
	if !strings.Contains(output.String(), "setup-agents-codex, setup-agents-claude, setup-agents-antigravity") {
		t.Fatalf("output = %q", output.String())
	}
}

func TestUpdateUsesNewCatalogOrder(t *testing.T) {
	ui := &fakeUI{answers: map[string]any{"refresh-remembered-tools": true}}
	app, dataRoot, _ := refreshTestApp(t, ui, &fakeExecutor{}, "setup-agents-codex", "setup-agents-claude")
	for _, name := range []string{"setup-agents-codex", "setup-agents-claude"} {
		if err := rememberTool("linux-amd64", name); err != nil {
			t.Fatal(err)
		}
	}
	writeActiveTestCatalog(t, dataRoot, "0.1.2", testCatalog("setup-agents-claude", "setup-agents-codex"))
	runs := []string{}
	app.refreshRunner = func(name string) error { runs = append(runs, name); return nil }
	if err := app.Execute([]string{"update"}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(runs, []string{"setup-agents-claude", "setup-agents-codex"}) {
		t.Fatalf("runs = %v", runs)
	}
}

func TestUpdateNoHistoryAndDeclineDoNotRefresh(t *testing.T) {
	for _, history := range []bool{false, true} {
		ui := &fakeUI{answers: map[string]any{"refresh-remembered-tools": false}}
		app, dataRoot, _ := refreshTestApp(t, ui, &fakeExecutor{}, "setup-agents-codex", "install-codex")
		obsoleteMarker := filepath.Join(dataRoot, "used-tools", "install-codex")
		if err := os.MkdirAll(filepath.Dir(obsoleteMarker), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(obsoleteMarker, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		if history {
			path := filepath.Join(dataRoot, "used-tools", "setup-agents-codex")
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, nil, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		app.refreshRunner = func(string) error { t.Fatal("unexpected refresh"); return nil }
		if err := app.Execute([]string{"update"}); err != nil {
			t.Fatal(err)
		}
		wantPrompts := 0
		if history {
			wantPrompts = 1
		}
		if len(ui.asked) != wantPrompts {
			t.Fatalf("prompts = %d, history = %t", len(ui.asked), history)
		}
	}
}

func TestGlobalAgentSetupIsRemembered(t *testing.T) {
	app, dataRoot, _ := refreshTestApp(t, &fakeUI{}, &fakeExecutor{}, "setup-agents-codex")
	if err := app.Execute([]string{"setup-agents-codex"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dataRoot, "used-tools", "setup-agents-codex")); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateAutomaticallyInstallsOnlyOlderMonitor(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Monitor supports Linux only")
	}
	for _, tc := range []struct {
		name, installed string
		want            bool
	}{
		{"older", "1.0.0", true},
		{"current", "1.1.0", false},
		{"newer", "1.2.0", false},
		{"absent", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ui := &fakeUI{}
			app, dataRoot, _ := refreshTestApp(t, ui, &fakeExecutor{}, "install-monitor")
			bundled := filepath.Join(dataRoot, "versions", "0.1.1", "packages", "monitor_runtime", "monitor_runtime", "__init__.py")
			if err := os.MkdirAll(filepath.Dir(bundled), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(bundled, []byte("__version__ = \"1.1.0\"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if tc.installed != "" {
				runtimeRoot := filepath.Join(os.Getenv("HOME"), ".monitor", "runtime")
				installed := filepath.Join(runtimeRoot, "app", "monitor_runtime", "__init__.py")
				if err := os.MkdirAll(filepath.Dir(installed), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(runtimeRoot, "owned.json"), []byte("{\"owner\":\"my-toolbox\",\"schema_version\":1}\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(installed, []byte("__version__ = \""+tc.installed+"\"\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			runs := []string{}
			app.refreshRunner = func(name string) error { runs = append(runs, name); return nil }
			if err := app.Execute([]string{"update"}); err != nil {
				t.Fatal(err)
			}
			if tc.want != reflect.DeepEqual(runs, []string{"install-monitor"}) {
				t.Fatalf("runs = %v, want monitor=%t", runs, tc.want)
			}
			if len(ui.asked) != 0 {
				t.Fatalf("unexpected prompts: %v", ui.asked)
			}
		})
	}
}

type alwaysSkippedBuiltins struct{}

func (alwaysSkippedBuiltins) SkipReason(string) (string, error) { return "already installed", nil }
func (alwaysSkippedBuiltins) Run(string, []string) error        { return nil }

func TestRefreshKeepsBuiltinSkip(t *testing.T) {
	executor := ProcessExecutor{Builtins: alwaysSkippedBuiltins{}}
	for _, tc := range []struct{ name, want string }{{"install-codex", "skipped"}, {"install-superpowers-codex", "skipped"}} {
		response, err := executor.Questions(Command{Name: tc.name, Protocol: "builtin"}, nil, nil)
		if err != nil || response.Status != tc.want {
			t.Fatalf("%s response = %#v, %v", tc.name, response, err)
		}
	}
}

func TestActiveToolRunsNewPayloadAndPreservesMarkers(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture uses Unix executable")
	}
	base := t.TempDir()
	t.Setenv("XDG_DATA_HOME", base)
	root := filepath.Join(base, "my-toolbox")
	oldVersion := filepath.Join(root, "versions", "0.1.1")
	newVersion := filepath.Join(root, "versions", "0.1.2")
	for _, path := range []string{oldVersion, newVersion} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := rememberTool("linux-amd64", "setup-agents-codex"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "current.txt"), []byte("0.1.2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\nprintf '%s' \"$1\"\n"
	if err := os.WriteFile(filepath.Join(newVersion, "tb"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	output := &bytes.Buffer{}
	if err := runActiveTool("linux-amd64", "setup-agents-codex", output, output); err != nil {
		t.Fatal(err)
	}
	if output.String() != "setup-agents-codex" {
		t.Fatalf("new payload output = %q", output.String())
	}
	if _, err := os.Stat(filepath.Join(root, "used-tools", "setup-agents-codex")); err != nil {
		t.Fatalf("marker lost: %v", err)
	}
}

func TestAlreadyCurrentUpdateStillOffersRefresh(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Linux update preflight requires Bash")
	}
	app, _, output := refreshTestApp(t, &fakeUI{answers: map[string]any{"refresh-remembered-tools": true}}, &fakeExecutor{}, "setup-agents-codex")
	if err := rememberTool("linux-amd64", "setup-agents-codex"); err != nil {
		t.Fatal(err)
	}
	builtins := NewToolboxBuiltins("", "linux-amd64", "0.1.2", output)
	builtins.client = &http.Client{Transport: roundTripFunction(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/repos/MatheusFS-dev/my-toolbox/releases/latest" {
			t.Fatalf("unexpected request: %s", request.URL)
		}
		return responseWithBody(`{"tag_name":"v0.1.2"}`), nil
	})}
	app.Executor = ProcessExecutor{Builtins: builtins, Environment: "linux-native"}
	runs := []string{}
	app.refreshRunner = func(name string) error { runs = append(runs, name); return nil }
	if err := app.Execute([]string{"update"}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(runs, []string{"setup-agents-codex"}) {
		t.Fatalf("runs = %v", runs)
	}
}

func TestUpgradeRefreshUsesNewPayload(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture uses Unix installer")
	}
	ui := &fakeUI{answers: map[string]any{"refresh-remembered-tools": true}}
	app, dataRoot, output := refreshTestApp(t, ui, &fakeExecutor{}, "setup-agents-codex")
	oldRoot := filepath.Join(dataRoot, "versions", "0.1.1")
	newRoot := filepath.Join(dataRoot, "versions", "0.1.2")
	writeActiveTestCatalog(t, dataRoot, "0.1.2", testCatalog("setup-agents-codex", "install-monitor"))
	if err := os.WriteFile(filepath.Join(dataRoot, "current.txt"), []byte("0.1.1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(oldRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := rememberTool("linux-amd64", "setup-agents-codex"); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	bundledMonitor := filepath.Join(newRoot, "packages", "monitor_runtime", "monitor_runtime", "__init__.py")
	installedRuntime := filepath.Join(home, ".monitor", "runtime")
	installedMonitor := filepath.Join(installedRuntime, "app", "monitor_runtime", "__init__.py")
	for _, path := range []string{bundledMonitor, installedMonitor} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for path, content := range map[string]string{
		bundledMonitor:   "__version__ = \"1.1.0\"\n",
		installedMonitor: "__version__ = \"1.0.0\"\n",
		filepath.Join(installedRuntime, "owned.json"): "{\"owner\":\"my-toolbox\",\"schema_version\":1}\n",
	} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	wrapper := filepath.Join(home, ".local", "bin", "tb")
	if err := os.MkdirAll(filepath.Dir(wrapper), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(wrapper, []byte(linuxToolboxWrapper), 0o755); err != nil {
		t.Fatal(err)
	}
	newBinary := "#!/bin/sh\nprintf 'new payload: %s\\n' \"$1\"\n"
	installer := fmt.Sprintf("#!/bin/sh\nset -eu\nmkdir -p %q\ncat > %q <<'SCRIPT'\n%sSCRIPT\nchmod 755 %q\nprintf '0.1.2\\n' > %q\n", newRoot, filepath.Join(newRoot, "tb"), newBinary, filepath.Join(newRoot, "tb"), filepath.Join(dataRoot, "current.txt"))
	builtins := NewToolboxBuiltins(oldRoot, "linux-amd64", "0.1.1", io.Discard)
	builtins.client = &http.Client{Transport: roundTripFunction(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path == "/repos/MatheusFS-dev/my-toolbox/releases/latest" {
			return responseWithBody(`{"tag_name":"v0.1.2"}`), nil
		}
		if request.URL.String() == toolboxLinuxInstallerURL {
			return responseWithBody(installer), nil
		}
		t.Fatalf("unexpected request: %s", request.URL)
		return nil, nil
	})}
	app.Executor = ProcessExecutor{Builtins: builtins, Environment: "linux-native"}
	if err := app.Execute([]string{"update"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "new payload: install-monitor\n") || !strings.Contains(output.String(), "new payload: setup-agents-codex\n") {
		t.Fatalf("output = %q", output.String())
	}
	if strings.Index(output.String(), "new payload: install-monitor") > strings.Index(output.String(), "new payload: setup-agents-codex") {
		t.Fatalf("Monitor was updated after setup refresh: %q", output.String())
	}
	if _, err := os.Stat(filepath.Join(dataRoot, "used-tools", "setup-agents-codex")); err != nil {
		t.Fatal(err)
	}
}

func TestRefreshFailureReportsRemainingCommands(t *testing.T) {
	ui := &fakeUI{answers: map[string]any{"refresh-remembered-tools": true}}
	app, dataRoot, output := refreshTestApp(t, ui, &fakeExecutor{}, "setup-agents-codex", "setup-agents-claude", "setup-agents-antigravity")
	for _, name := range []string{"setup-agents-codex", "setup-agents-claude", "setup-agents-antigravity"} {
		path := filepath.Join(dataRoot, "used-tools", name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	runs := []string{}
	app.refreshRunner = func(name string) error {
		runs = append(runs, name)
		if name == "setup-agents-claude" {
			return errors.New("failed")
		}
		return nil
	}
	if err := app.Execute([]string{"update"}); err == nil {
		t.Fatal("expected refresh failure")
	}
	if !reflect.DeepEqual(runs, []string{"setup-agents-codex", "setup-agents-claude"}) {
		t.Fatalf("runs = %v", runs)
	}
	for _, expected := range []string{"executed: setup-agents-codex", "failed: setup-agents-claude", "not run: setup-agents-antigravity"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("output missing %q: %q", expected, output.String())
		}
	}
}
