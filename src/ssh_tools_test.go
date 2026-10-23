//go:build linux

package main

import (
	"bytes"
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

func TestSSHDashboardResponsiveDirectPanel(t *testing.T) {
	profiles := []SSHProfile{{Alias: "lab", User: "dev", Hostname: "192.0.2.2", Port: 22}}
	wide := newSSHDashboard(profiles, "linux-native", 100, 20)
	wide.status = "Direct Ethernet active"
	if !strings.Contains(wide.View().Content, "Saved profiles") ||
		!strings.Contains(wide.View().Content, "Direct Ethernet") {
		t.Fatalf("wide view = %q", wide.View().Content)
	}
	updated, _ := wide.Update(tea.WindowSizeMsg{Width: 40, Height: 20})
	narrow := updated.(sshDashboard)
	if !strings.Contains(narrow.View().Content, "Direct Ethernet") ||
		strings.Contains(narrow.View().Content, "│") {
		t.Fatalf("narrow view = %q", narrow.View().Content)
	}
}

func TestSSHDashboardDisablesUnavailableDirectActions(t *testing.T) {
	model := newSSHDashboard(nil, "linux-native", 80, 20)
	model.directDisabled = true
	model.status = "Install Python 3 to manage direct Ethernet links."
	for _, key := range []rune{'d', 's'} {
		updated, _ := model.Update(tea.KeyPressMsg{Code: key})
		if updated.(sshDashboard).action != "" {
			t.Fatalf("key %q activated disabled direct action", key)
		}
	}
	if !strings.Contains(model.View().Content, "Install Python 3") {
		t.Fatal("missing actionable guidance")
	}
}

func TestSSHDirectStatusReportsServices(t *testing.T) {
	snapshot := sshDirectSnapshot{Config: &struct {
		Interface string `json:"interface"`
	}{Interface: "enp2s0"}}
	snapshot.Runtime.Active = true
	snapshot.Runtime.DNSMasq = true
	snapshot.Runtime.SSHServer = false
	snapshot.Runtime.Firewall = "ufw"
	status := renderSSHDirectStatus(snapshot)
	for _, expected := range []string{"enp2s0", "active", "dnsmasq running", "SSH server stopped", "ufw"} {
		if !strings.Contains(status, expected) {
			t.Fatalf("status %q lacks %q", status, expected)
		}
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
