package main

import (
	"strings"
	"testing"
)

func TestLoadCatalogRejectsInvalidCommands(t *testing.T) {
	// A broken validator could expose reserved names or platform-incomplete
	// tools that cannot execute from one of the published archives.
	tests := []struct {
		name string
		json string
		want string
	}{
		{"duplicate", `{"commands":[{"name":"a","description":"A","package":"p","visibility":"list","protocol":"builtin","entrypoints":{"linux-amd64":["builtin","a"],"linux-arm64":["builtin","a"],"windows-amd64":["builtin","a"]}},{"name":"a","description":"B","package":"p","visibility":"list","protocol":"builtin","entrypoints":{"linux-amd64":["builtin","a"],"linux-arm64":["builtin","a"],"windows-amd64":["builtin","a"]}}]}`, "duplicate"},
		{"reserved", `{"commands":[{"name":"list","description":"A","package":"p","visibility":"list","protocol":"builtin","entrypoints":{"linux-amd64":["builtin","a"],"linux-arm64":["builtin","a"],"windows-amd64":["builtin","a"]}}]}`, "reserved"},
		{"description", `{"commands":[{"name":"a","description":"","package":"p","visibility":"list","protocol":"builtin","entrypoints":{"linux-amd64":["builtin","a"],"linux-arm64":["builtin","a"],"windows-amd64":["builtin","a"]}}]}`, "description"},
		{"missing visibility", `{"commands":[{"name":"a","description":"A","package":"p","protocol":"builtin","entrypoints":{"linux-amd64":["builtin","a"],"linux-arm64":["builtin","a"],"windows-amd64":["builtin","a"]}}]}`, "visibility"},
		{"invalid visibility", `{"commands":[{"name":"a","description":"A","package":"p","visibility":"hidden","protocol":"builtin","entrypoints":{"linux-amd64":["builtin","a"],"linux-arm64":["builtin","a"],"windows-amd64":["builtin","a"]}}]}`, "visibility"},
		{"protocol", `{"commands":[{"name":"a","description":"A","package":"p","visibility":"list","protocol":"shell","entrypoints":{"linux-amd64":["builtin","a"],"linux-arm64":["builtin","a"],"windows-amd64":["builtin","a"]}}]}`, "protocol"},
		{"platform", `{"commands":[{"name":"a","description":"A","package":"p","visibility":"list","protocol":"builtin","entrypoints":{"linux-amd64":["builtin","a"],"linux-arm64":["builtin","a"]}}]}`, "windows-amd64"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := LoadCatalog(strings.NewReader(test.json))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("LoadCatalog() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestLoadCatalogAcceptsInteractivePythonMetadata(t *testing.T) {
	json := `{"commands":[{"name":"setup","description":"Setup","package":"p","visibility":"direct","protocol":"interactive-python","default_arguments":["--defaults"],"entrypoints":{"linux-amd64":["python-script","python3.py","python2.py"],"linux-arm64":["python-script","python3.py","python2.py"],"windows-amd64":["python-script","windows.py"]}}]}`
	catalog, err := LoadCatalog(strings.NewReader(json))
	if err != nil {
		t.Fatal(err)
	}
	command := catalog.Commands[0]
	if command.Visibility != "direct" || len(command.DefaultArguments) != 1 || command.DefaultArguments[0] != "--defaults" {
		t.Fatalf("command metadata = %#v", command)
	}
}

func TestRepositoryCatalogContainsExpectedToolsInOrder(t *testing.T) {
	catalog, err := LoadCatalogFile("../commands.json")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"install-codex", "install-claude", "install-antigravity", "install-uv", "install-gh",
		"install-superpowers-codex", "install-superpowers-claude", "install-superpowers-antigravity",
		"setup-agents-codex", "setup-agents-claude", "setup-agents-antigravity", "setup-agents-project",
	}
	if len(catalog.Commands) != len(want) {
		t.Fatalf("got %d commands, want %d", len(catalog.Commands), len(want))
	}
	for index, name := range want {
		if catalog.Commands[index].Name != name {
			t.Fatalf("command %d = %q, want %q", index, catalog.Commands[index].Name, name)
		}
	}
	for _, command := range catalog.Commands[:len(catalog.Commands)-1] {
		if command.Visibility != "list" {
			t.Fatalf("command %q visibility = %q, want list", command.Name, command.Visibility)
		}
	}
	if catalog.Commands[len(catalog.Commands)-1].Visibility != "direct" {
		t.Fatalf("setup-agents-project visibility = %q, want direct", catalog.Commands[len(catalog.Commands)-1].Visibility)
	}
	installerNames := []string{"install_codex.py", "install_claude.py", "install_antigravity.py", "install_project.py"}
	for index, installerName := range installerNames {
		command := catalog.Commands[index+8]
		if command.Protocol != "interactive-python" {
			t.Fatalf("command %q protocol = %q", command.Name, command.Protocol)
		}
		if len(command.DefaultArguments) != 0 {
			t.Fatalf("command %q default arguments = %v, want none", command.Name, command.DefaultArguments)
		}
		linuxEntrypoint := command.Entrypoints["linux-amd64"]
		windowsEntrypoint := command.Entrypoints["windows-amd64"]
		if linuxEntrypoint[0] != "python-script" || !strings.HasSuffix(linuxEntrypoint[1], "/python3/"+installerName) || !strings.HasSuffix(linuxEntrypoint[2], "/python2/"+installerName) {
			t.Fatalf("command %q Linux entrypoint = %v", command.Name, linuxEntrypoint)
		}
		if windowsEntrypoint[0] != "python-script" || !strings.HasSuffix(windowsEntrypoint[1], "/windows/"+installerName) {
			t.Fatalf("command %q Windows entrypoint = %v", command.Name, windowsEntrypoint)
		}
	}
}
