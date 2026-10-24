//go:build linux

package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestSSHProfileAliasAndConfigSafety(t *testing.T) {
	for _, alias := range []string{"", "../x", "foo bar", "-bad", "*.example", "a/b"} {
		if validSSHAlias(alias) {
			t.Errorf("accepted %q", alias)
		}
	}
	if !validSSHAlias("lab-node_1") {
		t.Fatal("rejected safe alias")
	}
	if !hasSSHHostCollision("Host lab-node_1 other\n  HostName x\n", "lab-node_1") {
		t.Fatal("missed exact host collision")
	}
	if hasSSHHostCollision("Host lab-*\n", "lab-node_1") {
		t.Fatal("wildcard is not an exact collision")
	}
}

func TestSSHDirectCommandRejectsArguments(t *testing.T) {
	var output bytes.Buffer
	executor := ProcessExecutor{Root: t.TempDir(), Environment: "linux-native", Output: &output}
	err := executor.Run(Command{Name: "ssh-tools", Protocol: "builtin"}, nil, []string{"unexpected"})
	if err == nil || !strings.Contains(err.Error(), "does not accept arguments") {
		t.Fatalf("error = %v", err)
	}
}

func TestSSHPromptsUseVisibleDefaultsAndRetryRequiredInput(t *testing.T) {
	var output bytes.Buffer
	session := sshToolsSession{input: strings.NewReader("\nvalue\n\n"), messages: newToolOutput(&output, "ssh-tools", nil, colorModeNever)}
	value, err := session.prompt("Alias", "", true)
	if err != nil || value != "value" {
		t.Fatalf("required prompt = %q, %v", value, err)
	}
	confirmed, err := session.confirm("Continue?", true)
	if err != nil || !confirmed {
		t.Fatalf("confirmation = %v, %v", confirmed, err)
	}
	for _, phrase := range []string{"(required)", "A value is required.", "[Y/n] (default: Y)"} {
		if !strings.Contains(output.String(), phrase) {
			t.Fatalf("missing %q in %q", phrase, output.String())
		}
	}
}

func TestSSHProfileRejectsReadablePrivateKey(t *testing.T) {
	home := t.TempDir()
	if err := saveSSHProfile(home, SSHProfile{Alias: "lab", Hostname: "example.org", User: "dev", Port: 22}); err != nil {
		t.Fatal(err)
	}
	key := filepath.Join(home, ".ssh", "ssh-tools", "keys", "lab")
	if err := os.WriteFile(key, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadSSHProfiles(home); err == nil || !strings.Contains(err.Error(), "private key") {
		t.Fatalf("expected insecure key error, got %v", err)
	}
}

func TestSSHProfileReportsDamagedKey(t *testing.T) {
	home := t.TempDir()
	if err := saveSSHProfile(home, SSHProfile{Alias: "lab", Hostname: "example.org", User: "dev", Port: 22}); err != nil {
		t.Fatal(err)
	}
	key := filepath.Join(home, ".ssh", "ssh-tools", "keys", "lab")
	if err := os.WriteFile(key, []byte("damaged"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(key+".pub", []byte("damaged"), 0644); err != nil {
		t.Fatal(err)
	}
	profiles, err := loadSSHProfiles(home)
	if err != nil || len(profiles) != 1 || profiles[0].Health != "damaged key" {
		t.Fatalf("profiles = %#v, %v", profiles, err)
	}
}

func TestSSHProfileSetupCommandOrderAndOutput(t *testing.T) {
	home := t.TempDir()
	var output bytes.Buffer
	type invocation struct {
		name string
		args []string
	}
	var calls []invocation
	session := sshToolsSession{
		home: home, input: strings.NewReader("lab\n192.0.2.10\ndev\n\ny\n"),
		output: &output, messages: newToolOutput(&output, "ssh-tools", nil, colorModeNever),
		execRun: func(command string, arguments ...string) error {
			calls = append(calls, invocation{command, append([]string(nil), arguments...)})
			if command == "ssh-keygen" {
				key := arguments[5]
				if err := os.WriteFile(key, []byte("private"), 0600); err != nil {
					return err
				}
				if err := os.WriteFile(key+".pub", []byte("public"), 0644); err != nil {
					return err
				}
			}
			_, err := output.Write([]byte("EXTERNAL> unchanged\n"))
			return err
		},
	}
	if err := session.configureProfile(sshDashboard{action: "add"}, nil); err != nil {
		t.Fatal(err)
	}
	key := filepath.Join(home, ".ssh", "ssh-tools", "keys", "lab")
	want := []invocation{
		{"ssh-keygen", []string{"-t", "ed25519", "-N", "", "-f", key, "-C", "ssh-tools-lab"}},
		{"ssh-copy-id", []string{"-i", key + ".pub", "-p", "22", "dev@192.0.2.10"}},
		{"ssh", []string{"-o", "BatchMode=yes", "-o", "ConnectTimeout=5", "-i", key, "-p", "22", "dev@192.0.2.10", "true"}},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %#v", calls)
	}
	if strings.Count(output.String(), "EXTERNAL> unchanged\n") != 3 ||
		strings.Index(output.String(), "Generating") > strings.Index(output.String(), "EXTERNAL>") {
		t.Fatalf("progress/output = %q", output.String())
	}
}

func TestSSHDashboardNavigationAndWSL(t *testing.T) {
	profiles := []SSHProfile{{Alias: "one"}, {Alias: "two"}}
	model := newSSHDashboard(profiles, "linux-wsl", 40, 20)
	if strings.Contains(model.View().Content, "Direct Ethernet") {
		t.Fatal("WSL shows direct link")
	}
	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	model = updated.(sshDashboard)
	if model.cursor != 1 {
		t.Fatalf("cursor = %d", model.cursor)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: 't'})
	model = updated.(sshDashboard)
	if model.action != "test" || model.alias != "two" {
		t.Fatalf("action = %q alias = %q", model.action, model.alias)
	}
}

func TestSSHLauncherRendersPlatformActions(t *testing.T) {
	linux := newSSHLauncher("linux-native", "", 80, 24)
	if !linux.View().AltScreen {
		t.Fatal("launcher does not use the alternate screen")
	}
	for _, action := range []string{
		"Manage passwordless SSH profiles",
		"Set up direct Ethernet",
		"Undo direct Ethernet setup",
		"Install desktop-login status monitor",
		"Remove desktop-login status monitor",
	} {
		if !strings.Contains(linux.View().Content, action) {
			t.Fatalf("native launcher missing %q: %q", action, linux.View().Content)
		}
	}

	wsl := newSSHLauncher("linux-wsl", "", 80, 24)
	if !strings.Contains(wsl.View().Content, "Manage passwordless SSH profiles") {
		t.Fatalf("WSL launcher = %q", wsl.View().Content)
	}
	for _, action := range []string{"direct Ethernet", "status monitor"} {
		if strings.Contains(wsl.View().Content, action) {
			t.Fatalf("WSL launcher exposes %q: %q", action, wsl.View().Content)
		}
	}
}

func TestSSHLauncherKeyboardSelectionAndQuit(t *testing.T) {
	model := newSSHLauncher("linux-native", "", 80, 24)
	for range 3 {
		updated, _ := model.Update(tea.KeyPressMsg{Code: 'j'})
		model = updated.(sshLauncher)
	}
	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(sshLauncher)
	if model.action != "monitor-install" {
		t.Fatalf("action = %q", model.action)
	}

	for _, key := range []tea.KeyPressMsg{
		{Code: 'q'},
		{Code: tea.KeyEscape},
		{Code: 'c', Mod: tea.ModCtrl},
	} {
		model = newSSHLauncher("linux-native", "", 80, 24)
		updated, command := model.Update(key)
		if updated.(sshLauncher).action != "" || command == nil {
			t.Fatalf("key %#v did not quit cleanly", key)
		}
	}
}

func TestSSHLauncherDisablesUnavailableDirectActions(t *testing.T) {
	model := newSSHLauncher("linux-native", "Install Python 3 to manage direct Ethernet links.", 80, 24)
	if !strings.Contains(model.View().Content, "Install Python 3") {
		t.Fatalf("missing remediation: %q", model.View().Content)
	}
	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	updated, _ = updated.(sshLauncher).Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if updated.(sshLauncher).action != "" {
		t.Fatalf("disabled action = %q", updated.(sshLauncher).action)
	}
}

func TestSSHDashboardContainsOnlySavedProfiles(t *testing.T) {
	profiles := []SSHProfile{{Alias: "lab", User: "dev", Hostname: "192.0.2.2", Port: 22}}
	for _, environment := range []string{"linux-native", "linux-wsl"} {
		model := newSSHDashboard(profiles, environment, 100, 20)
		if !strings.Contains(model.View().Content, "Saved profiles") ||
			strings.Contains(model.View().Content, "Direct Ethernet") {
			t.Fatalf("%s dashboard = %q", environment, model.View().Content)
		}
		if !strings.Contains(model.View().Content, "q back") {
			t.Fatalf("%s dashboard help = %q", environment, model.View().Content)
		}
	}
}

func TestSSHToolsReturnsFromProfileDashboardToLauncher(t *testing.T) {
	var output bytes.Buffer
	var models []string
	session := sshToolsSession{
		environment: "linux-wsl", home: t.TempDir(), output: &output, errorOutput: &bytes.Buffer{},
		messages: newToolOutput(&output, "ssh-tools", nil, colorModeNever),
	}
	launcherCalls := 0
	session.runProgram = func(model tea.Model) (tea.Model, error) {
		switch model.(type) {
		case sshLauncher:
			models = append(models, "launcher")
			launcherCalls++
			if launcherCalls == 1 {
				return sshLauncher{action: "profiles"}, nil
			}
			return sshLauncher{}, nil
		case sshDashboard:
			models = append(models, "dashboard")
			return sshDashboard{}, nil
		default:
			return nil, fmt.Errorf("unexpected model %T", model)
		}
	}
	if err := session.runSSHTools(); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(models, ","), "launcher,dashboard,launcher"; got != want {
		t.Fatalf("models = %q, want %q", got, want)
	}
	if !strings.Contains(output.String(), "No changes made.") || strings.Contains(output.String(), "SUCCESS") {
		t.Fatalf("output = %q", output.String())
	}
}

func TestSSHToolsCancellationReturnsToLauncherWithoutSuccess(t *testing.T) {
	var output bytes.Buffer
	root := t.TempDir()
	helper := filepath.Join(root, "packages", "ssh_tools", "direct_link.py")
	if err := os.MkdirAll(filepath.Dir(helper), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(helper, nil, 0644); err != nil {
		t.Fatal(err)
	}
	session := sshToolsSession{
		root: root, environment: "linux-native", home: t.TempDir(), input: strings.NewReader("n\n"),
		output: &output, errorOutput: &bytes.Buffer{}, messages: newToolOutput(&output, "ssh-tools", nil, colorModeNever),
	}
	launcherCalls := 0
	session.runProgram = func(model tea.Model) (tea.Model, error) {
		if _, ok := model.(sshLauncher); !ok {
			return nil, fmt.Errorf("unexpected model %T", model)
		}
		launcherCalls++
		if launcherCalls == 1 {
			return sshLauncher{action: "direct-undo"}, nil
		}
		return sshLauncher{}, nil
	}
	if err := session.runSSHTools(); err != nil {
		t.Fatal(err)
	}
	if session.successes != 0 || !strings.Contains(output.String(), "Cancelled; nothing changed.") ||
		strings.Contains(output.String(), "SUCCESS") {
		t.Fatalf("successes = %d, output = %q", session.successes, output.String())
	}
}

func TestSSHToolsSuccessfulDirectSetupReportsOneSuccess(t *testing.T) {
	var output bytes.Buffer
	root := t.TempDir()
	helper := filepath.Join(root, "packages", "ssh_tools", "direct_link.py")
	if err := os.MkdirAll(filepath.Dir(helper), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(helper, nil, 0644); err != nil {
		t.Fatal(err)
	}
	var setupInput string
	session := sshToolsSession{
		root: root, environment: "linux-native", home: t.TempDir(), input: strings.NewReader("eth0\n\n\ny\n"),
		output: &output, errorOutput: &bytes.Buffer{}, messages: newToolOutput(&output, "ssh-tools", nil, colorModeNever),
		execRunInput: func(input, command string, arguments ...string) error {
			setupInput = input
			return nil
		},
	}
	launcherCalls := 0
	session.runProgram = func(model tea.Model) (tea.Model, error) {
		if _, ok := model.(sshLauncher); !ok {
			return nil, fmt.Errorf("unexpected model %T", model)
		}
		launcherCalls++
		if launcherCalls == 1 {
			return sshLauncher{action: "direct-setup"}, nil
		}
		return sshLauncher{}, nil
	}
	if err := session.runSSHTools(); err != nil {
		t.Fatal(err)
	}
	if session.successes != 1 || !strings.Contains(setupInput, `"interface":"eth0"`) ||
		!strings.Contains(output.String(), "SUCCESS] Completed 1 SSH operation(s).") {
		t.Fatalf("successes = %d, setup input = %q, output = %q", session.successes, setupInput, output.String())
	}
}

func TestSSHDirectActionsRunMatchingHelperCommands(t *testing.T) {
	var output bytes.Buffer
	var calls [][]string
	var inputs []string
	root := t.TempDir()
	helper := filepath.Join(root, "packages", "ssh_tools", "direct_link.py")
	if err := os.MkdirAll(filepath.Dir(helper), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(helper, nil, 0644); err != nil {
		t.Fatal(err)
	}
	session := sshToolsSession{
		root: root, environment: "linux-native",
		input:    strings.NewReader("eth0\n\n\ny\n"),
		messages: newToolOutput(&output, "ssh-tools", nil, colorModeNever),
		execRun: func(command string, arguments ...string) error {
			calls = append(calls, append([]string{command}, arguments...))
			return nil
		},
		execRunInput: func(input, command string, arguments ...string) error {
			inputs = append(inputs, input)
			calls = append(calls, append([]string{command}, arguments...))
			return nil
		},
	}
	if err := session.directSetup(); err != nil {
		t.Fatal(err)
	}
	if got, want := calls[0], []string{"sudo", "python3", helper, "setup"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("setup call = %#v, want %#v", got, want)
	}
	if !strings.Contains(inputs[0], `"interface":"eth0"`) {
		t.Fatalf("setup input = %q", inputs[0])
	}

	for _, test := range []struct {
		name  string
		run   func() error
		want  string
		input string
	}{
		{name: "undo", run: session.directUndo, want: "undo", input: "y\n"},
		{name: "install", run: func() error { return session.updateDirectMonitor("install-status") }, want: "install-status", input: "y\n"},
		{name: "remove", run: func() error { return session.updateDirectMonitor("uninstall-status") }, want: "uninstall-status", input: "y\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			session.input = strings.NewReader(test.input)
			session.reader = nil
			if err := test.run(); err != nil {
				t.Fatal(err)
			}
			if got := calls[len(calls)-1]; got[len(got)-1] != test.want {
				t.Fatalf("call = %#v, want helper action %q", got, test.want)
			}
		})
	}
}

func TestSSHProfileWriteAndRemoval(t *testing.T) {
	home := t.TempDir()
	profile := SSHProfile{Alias: "lab", Hostname: "192.0.2.1", User: "dev", Port: 2222}
	if err := saveSSHProfile(home, profile); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(home, ".ssh", "config"))
	if err != nil || !strings.HasPrefix(string(data), sshIncludeBlock) {
		t.Fatalf("include = %q, %v", data, err)
	}
	profiles, err := loadSSHProfiles(home)
	if err != nil || len(profiles) != 1 || profiles[0].Alias != "lab" {
		t.Fatalf("profiles = %#v, %v", profiles, err)
	}
	if err := removeSSHProfile(home, "lab"); err != nil {
		t.Fatal(err)
	}
	profiles, err = loadSSHProfiles(home)
	if err != nil || len(profiles) != 0 {
		t.Fatalf("profiles after remove = %#v, %v", profiles, err)
	}
}

func TestSSHProfileRefusesUnmanagedHostAndSymlinkedConfig(t *testing.T) {
	home := t.TempDir()
	ssh := filepath.Join(home, ".ssh")
	if err := os.Mkdir(ssh, 0700); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(ssh, "config")
	if err := os.WriteFile(config, []byte("Host lab\n  HostName example.org\n"), 0600); err != nil {
		t.Fatal(err)
	}
	profile := SSHProfile{Alias: "lab", Hostname: "192.0.2.1", User: "dev", Port: 22}
	if err := checkSSHProfileDestination(home, profile); err == nil || !strings.Contains(err.Error(), "unmanaged") {
		t.Fatalf("collision error = %v", err)
	}
	if err := os.Remove(config); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(home, "elsewhere"), config); err != nil {
		t.Fatal(err)
	}
	if err := checkSSHProfileDestination(home, profile); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("symlink error = %v", err)
	}
}

func TestSSHProfileSupportsHomePathWithSpaces(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home with spaces")
	if err := os.Mkdir(home, 0700); err != nil {
		t.Fatal(err)
	}
	profile := SSHProfile{Alias: "lab", Hostname: "example.org", User: "dev", Port: 22}
	if err := saveSSHProfile(home, profile); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(home, ".ssh", "ssh-tools", "config.d", "lab.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "IdentityFile \"") {
		t.Fatalf("identity is not quoted: %q", data)
	}
	profiles, err := loadSSHProfiles(home)
	if err != nil || len(profiles) != 1 {
		t.Fatalf("profiles = %#v, %v", profiles, err)
	}
}

func TestSSHProfileRejectsUnexpectedSSHDirective(t *testing.T) {
	home := t.TempDir()
	profile := SSHProfile{Alias: "lab", Hostname: "example.org", User: "dev", Port: 22}
	if err := saveSSHProfile(home, profile); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".ssh", "ssh-tools", "config.d", "lab.conf")
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("    ProxyCommand dangerous-command\n"); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := loadSSHProfiles(home); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("expected damaged profile error, got %v", err)
	}
}
