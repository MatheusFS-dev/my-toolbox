package main

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestResolveRequirementsDerivesProtocolElevationAndSpecialCapabilities(t *testing.T) {
	command := Command{
		Name:         "setup",
		Protocol:     "interactive-script",
		Elevation:    "sudo",
		Environments: []string{"linux-native"},
		Requirements: map[string][]string{"linux-native": {"apt-get", "debian-ubuntu"}},
		Entrypoints:  map[string][]string{"linux-amd64": {"bash-script", "setup.sh"}},
	}
	got, err := ResolveRequirements(command, "linux-native")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"bash", "sudo", "apt-get", "debian-ubuntu"}
	if !reflect.DeepEqual(capabilityIDs(got), want) {
		t.Fatalf("requirements = %v, want %v", capabilityIDs(got), want)
	}
}

func TestResolveRequirementsUsesEnvironmentSpecificPythonAlternatives(t *testing.T) {
	command := Command{
		Name:         "workspace",
		Protocol:     "interactive-python",
		Environments: []string{"linux-native", "windows"},
		Entrypoints: map[string][]string{
			"linux-amd64":   {"python-script", "python3.py", "python2.py"},
			"windows-amd64": {"python-script", "windows.py"},
		},
	}
	linux, err := ResolveRequirements(command, "linux-native")
	if err != nil {
		t.Fatal(err)
	}
	windows, err := ResolveRequirements(command, "windows")
	if err != nil {
		t.Fatal(err)
	}
	if got := capabilityIDs(linux); !reflect.DeepEqual(got, []string{"python-workspace-linux"}) {
		t.Fatalf("Linux requirements = %v", got)
	}
	if got := capabilityIDs(windows); !reflect.DeepEqual(got, []string{"python39"}) {
		t.Fatalf("Windows requirements = %v", got)
	}
	if windows[0].Label != "Python 3.9+" {
		t.Fatalf("Windows Python capability label = %q, want Python 3.9+", windows[0].Label)
	}
	if linux[0].Label != "Python 3.9+, or Python 2.7 with toml==0.10.2" {
		t.Fatalf("Linux Python capability label = %q", linux[0].Label)
	}
}

func TestResolveRequirementsDerivesOfficialInstallerShellButNotGitHubCLI(t *testing.T) {
	installer := Command{Name: "install-uv", Protocol: "builtin", Environments: []string{"linux-native", "windows"}}
	linux, _ := ResolveRequirements(installer, "linux-native")
	windows, _ := ResolveRequirements(installer, "windows")
	github, _ := ResolveRequirements(Command{Name: "install-gh", Protocol: "builtin", Environments: []string{"linux-native"}}, "linux-native")
	if !reflect.DeepEqual(capabilityIDs(linux), []string{"bash"}) || !reflect.DeepEqual(capabilityIDs(windows), []string{"powershell"}) {
		t.Fatalf("installer requirements: Linux=%v Windows=%v", capabilityIDs(linux), capabilityIDs(windows))
	}
	if len(github) != 0 {
		t.Fatalf("GitHub CLI requirements = %v, want none", capabilityIDs(github))
	}
}

func TestLoadCatalogRejectsInvalidCapabilityDeclarations(t *testing.T) {
	base := `{"commands":[{"name":"a","category":"A","description":"A","package":"p","visibility":"list","protocol":"builtin","environments":["linux-native"],"requirements":%s,"entrypoints":{"linux-amd64":["builtin","a"],"linux-arm64":["builtin","a"]}}]}`
	tests := []struct {
		name         string
		requirements string
		want         string
	}{
		{"unknown", `{"linux-native":["mystery"]}`, "unknown capability"},
		{"duplicate", `{"linux-native":["apt-get","apt-get"]}`, "repeats capability"},
		{"undeclared environment", `{"windows":["winget"]}`, "undeclared environment"},
		{"incompatible", `{"linux-native":["winget"]}`, "incompatible"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := LoadCatalog(strings.NewReader(fmt.Sprintf(base, test.requirements)))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("LoadCatalog() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestLoadCatalogRejectsExplicitDerivedCapability(t *testing.T) {
	input := `{"commands":[{"name":"install-uv","category":"A","description":"A","package":"p","visibility":"list","protocol":"builtin","environments":["linux-native"],"requirements":{"linux-native":["bash"]},"entrypoints":{"linux-amd64":["builtin","install-uv"],"linux-arm64":["builtin","install-uv"]}}]}`
	_, err := LoadCatalog(strings.NewReader(input))
	if err == nil || !strings.Contains(err.Error(), "derived capability") {
		t.Fatalf("LoadCatalog() error = %v", err)
	}
}

func TestRepositoryScriptRequirementsIncludeEveryPreStartUtility(t *testing.T) {
	catalog, err := LoadCatalogFile("../commands.json")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{
		"setup-wsl": {
			"bash", "sudo", "wsl-ubuntu-supported", "apt-get", "cut", "dirname", "env", "getent", "grep", "sort",
		},
		"set-default-cwd": {
			"bash", "awk", "chmod", "cmp", "cp", "date", "grep", "mktemp", "mv", "od", "rm", "tail", "tr",
		},
		"toggle-nopasswd-sudo": {
			"bash", "sudo", "visudo", "cat", "chmod", "grep", "id", "install", "mktemp", "rm",
		},
	}
	for name, expected := range want {
		command, ok := catalog.Find(name)
		if !ok {
			t.Fatalf("missing command %q", name)
		}
		environment := command.Environments[0]
		got, err := ResolveRequirements(command, environment)
		if err != nil {
			t.Fatalf("resolve %s: %v", name, err)
		}
		if ids := capabilityIDs(got); !reflect.DeepEqual(ids, expected) {
			t.Errorf("%s requirements = %v, want %v", name, ids, expected)
		}
	}
}

func capabilityIDs(capabilities []Capability) []string {
	ids := make([]string, len(capabilities))
	for index, capability := range capabilities {
		ids[index] = capability.ID
	}
	return ids
}
