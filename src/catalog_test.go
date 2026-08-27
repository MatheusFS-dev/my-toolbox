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
		{"duplicate", `{"commands":[{"name":"a","description":"A","package":"p","protocol":"builtin","entrypoints":{"linux-amd64":["builtin","a"],"linux-arm64":["builtin","a"],"windows-amd64":["builtin","a"]}},{"name":"a","description":"B","package":"p","protocol":"builtin","entrypoints":{"linux-amd64":["builtin","a"],"linux-arm64":["builtin","a"],"windows-amd64":["builtin","a"]}}]}`, "duplicate"},
		{"reserved", `{"commands":[{"name":"list","description":"A","package":"p","protocol":"builtin","entrypoints":{"linux-amd64":["builtin","a"],"linux-arm64":["builtin","a"],"windows-amd64":["builtin","a"]}}]}`, "reserved"},
		{"description", `{"commands":[{"name":"a","description":"","package":"p","protocol":"builtin","entrypoints":{"linux-amd64":["builtin","a"],"linux-arm64":["builtin","a"],"windows-amd64":["builtin","a"]}}]}`, "description"},
		{"protocol", `{"commands":[{"name":"a","description":"A","package":"p","protocol":"shell","entrypoints":{"linux-amd64":["builtin","a"],"linux-arm64":["builtin","a"],"windows-amd64":["builtin","a"]}}]}`, "protocol"},
		{"platform", `{"commands":[{"name":"a","description":"A","package":"p","protocol":"builtin","entrypoints":{"linux-amd64":["builtin","a"],"linux-arm64":["builtin","a"]}}]}`, "windows-amd64"},
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
}
