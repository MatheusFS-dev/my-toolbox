package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestREADMERequirementLinesMatchResolvedRepositoryCatalog(t *testing.T) {
	catalog, err := LoadCatalogFile("../commands.json")
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile("../README.md")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(content), "\n")
	for _, command := range catalog.Commands {
		bullet := "- `" + command.Name + "`"
		lineIndex := -1
		for index, line := range lines {
			if strings.HasPrefix(line, bullet) {
				lineIndex = index
				break
			}
		}
		if lineIndex < 0 {
			t.Fatalf("README is missing catalog entry %s", command.Name)
		}
		labels := map[string]bool{}
		for _, environment := range command.Environments {
			capabilities, err := ResolveRequirements(command, environment)
			if err != nil {
				t.Fatal(err)
			}
			for _, capability := range capabilities {
				labels[capability.Label] = true
			}
		}
		if len(labels) == 0 {
			if lineIndex+1 < len(lines) && strings.HasPrefix(lines[lineIndex+1], "  Requires:") {
				t.Fatalf("README gives %s a requirement line, want none", command.Name)
			}
			continue
		}
		if lineIndex+1 >= len(lines) || !strings.HasPrefix(lines[lineIndex+1], "  Requires:") {
			t.Fatalf("README is missing requirement line after %s", command.Name)
		}
		documented := strings.ReplaceAll(lines[lineIndex+1], "`", "")
		for label := range labels {
			if !strings.Contains(documented, label) {
				t.Fatalf("README requirement for %s = %q, missing %q", command.Name, documented, label)
			}
		}
	}
}

func TestLoadCatalogRejectsInvalidCommands(t *testing.T) {
	// A broken validator could expose reserved names or platform-incomplete
	// tools that cannot execute from one of the published archives.
	tests := []struct {
		name string
		json string
		want string
	}{
		{"duplicate", `{"commands":[{"name":"a","category":"Test","description":"A","package":"p","visibility":"list","protocol":"builtin","environments":["linux-native","linux-wsl","windows"],"entrypoints":{"linux-amd64":["builtin","a"],"linux-arm64":["builtin","a"],"windows-amd64":["builtin","a"]}},{"name":"a","category":"Test","description":"B","package":"p","visibility":"list","protocol":"builtin","environments":["linux-native","linux-wsl","windows"],"entrypoints":{"linux-amd64":["builtin","a"],"linux-arm64":["builtin","a"],"windows-amd64":["builtin","a"]}}]}`, "duplicate"},
		{"reserved", `{"commands":[{"name":"list","description":"A","package":"p","visibility":"list","protocol":"builtin","entrypoints":{"linux-amd64":["builtin","a"],"linux-arm64":["builtin","a"],"windows-amd64":["builtin","a"]}}]}`, "reserved"},
		{"description", `{"commands":[{"name":"a","description":"","package":"p","visibility":"list","protocol":"builtin","entrypoints":{"linux-amd64":["builtin","a"],"linux-arm64":["builtin","a"],"windows-amd64":["builtin","a"]}}]}`, "description"},
		{"missing visibility", `{"commands":[{"name":"a","description":"A","package":"p","protocol":"builtin","entrypoints":{"linux-amd64":["builtin","a"],"linux-arm64":["builtin","a"],"windows-amd64":["builtin","a"]}}]}`, "visibility"},
		{"invalid visibility", `{"commands":[{"name":"a","description":"A","package":"p","visibility":"hidden","protocol":"builtin","entrypoints":{"linux-amd64":["builtin","a"],"linux-arm64":["builtin","a"],"windows-amd64":["builtin","a"]}}]}`, "visibility"},
		{"protocol", `{"commands":[{"name":"a","description":"A","package":"p","visibility":"list","protocol":"shell","entrypoints":{"linux-amd64":["builtin","a"],"linux-arm64":["builtin","a"],"windows-amd64":["builtin","a"]}}]}`, "protocol"},
		{"platform", `{"commands":[{"name":"a","description":"A","package":"p","visibility":"list","protocol":"builtin","environments":["linux-native","linux-wsl","windows"],"entrypoints":{"linux-amd64":["builtin","a"],"linux-arm64":["builtin","a"]}}]}`, "windows-amd64"},
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

func TestRepositoryCatalogPreservesExecutionMetadata(t *testing.T) {
	catalog, err := LoadCatalogFile("../commands.json")
	if err != nil {
		t.Fatal(err)
	}
	signatures := []string{}
	for _, command := range catalog.Commands {
		entrypoints := []string{}
		requirements := []string{}
		for _, environment := range []string{"linux-native", "linux-wsl", "windows"} {
			if ids, exists := command.Requirements[environment]; exists {
				requirements = append(requirements, environment+"="+strings.Join(ids, ","))
			}
		}
		for _, platform := range supportedPlatforms {
			if entrypoint, exists := command.Entrypoints[platform]; exists {
				entrypoints = append(entrypoints, platform+"="+strings.Join(entrypoint, ","))
			}
		}
		signatures = append(signatures, strings.Join([]string{
			command.Name,
			command.Package,
			command.Visibility,
			command.Protocol,
			strings.Join(command.Environments, ","),
			command.Elevation,
			strings.Join(command.DefaultArguments, ","),
			strings.Join(requirements, ";"),
			strings.Join(entrypoints, ";"),
		}, "|"))
	}
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(signatures, "\n"))))
	const want = "14229cddf628a051e12e73fcd394e2221bc894d780748af58bf504d6843a4e59"
	if digest != want {
		t.Fatalf("execution metadata digest = %s, want %s", digest, want)
	}
}

func TestLoadCatalogRejectsEmptyCategory(t *testing.T) {
	// A missing category would leave the presentation layer unable to place a
	// valid command without inventing a fallback group.
	json := `{"commands":[{"name":"a","category":"","description":"A","package":"p","visibility":"list","protocol":"builtin","environments":["linux-native","linux-wsl","windows"],"entrypoints":{"linux-amd64":["builtin","a"],"linux-arm64":["builtin","a"],"windows-amd64":["builtin","a"]}}]}`
	_, err := LoadCatalog(strings.NewReader(json))
	if err == nil || !strings.Contains(err.Error(), "category") {
		t.Fatalf("LoadCatalog() error = %v, want category failure", err)
	}
}

func TestLoadCatalogAcceptsInteractivePythonMetadata(t *testing.T) {
	json := `{"commands":[{"name":"setup","category":"Test","description":"Setup","package":"p","visibility":"direct","protocol":"interactive-python","environments":["linux-native","linux-wsl","windows"],"default_arguments":["--defaults"],"entrypoints":{"linux-amd64":["python-script","python3.py","python2.py"],"linux-arm64":["python-script","python3.py","python2.py"],"windows-amd64":["python-script","windows.py"]}}]}`
	catalog, err := LoadCatalog(strings.NewReader(json))
	if err != nil {
		t.Fatal(err)
	}
	command := catalog.Commands[0]
	if command.Visibility != "direct" || len(command.DefaultArguments) != 1 || command.DefaultArguments[0] != "--defaults" {
		t.Fatalf("command metadata = %#v", command)
	}
}

func TestLoadCatalogAcceptsSparseInteractiveScriptEntrypoints(t *testing.T) {
	json := `{"commands":[{"name":"linux-setup","category":"Test","description":"Linux setup","package":"scripts","visibility":"list","protocol":"interactive-script","environments":["linux-native"],"elevation":"sudo","entrypoints":{"linux-amd64":["bash-script","packages/scripts/setup.sh"],"linux-arm64":["bash-script","packages/scripts/setup.sh"]}},{"name":"windows-setup","category":"Test","description":"Windows setup","package":"scripts","visibility":"list","protocol":"interactive-script","environments":["windows"],"entrypoints":{"windows-amd64":["powershell-script","packages/scripts/setup.ps1"]}}]}`
	catalog, err := LoadCatalog(strings.NewReader(json))
	if err != nil {
		t.Fatal(err)
	}
	if catalog.Commands[0].Elevation != "sudo" || len(catalog.Commands[0].Environments) != 1 {
		t.Fatalf("Linux command metadata = %#v", catalog.Commands[0])
	}
}

func TestRepositoryCatalogUsesApprovedCategoriesAndDescriptions(t *testing.T) {
	catalog, err := LoadCatalogFile("../commands.json")
	if err != nil {
		t.Fatal(err)
	}
	wantCategories := []string{
		"Agents", "Base Tools", "Agent Plugins", "Agent Workspace", "Terminal",
		"System Utilities", "Project Utilities",
	}
	wantDescriptions := map[string]string{
		"install-codex":                   "Install Codex for the current user on Linux or Windows. Skips installation when `codex` is already available.",
		"install-claude":                  "Install Claude Code for the current user on Linux or Windows. Skips installation when `claude` is already available.",
		"install-antigravity":             "Install Antigravity for the current user on Linux or Windows. Skips installation when `agy` is already available.",
		"install-uv":                      "Install uv for the current user on Linux or Windows without changing shell PATH configuration. Skips installation when `uv` is already available.",
		"install-gh":                      "Download, verify, and install the latest GitHub CLI for the current user. Shows PATH guidance when needed.",
		"install-superpowers-codex":       "Add the Superpowers plugin to Codex. Requires Codex plugin management, skips an existing installation, and leaves other plugins unchanged.",
		"install-superpowers-claude":      "Add the Superpowers plugin to Claude Code for the current user. Requires plugin management, skips an existing installation, and leaves other plugins unchanged.",
		"install-superpowers-antigravity": "Add the Superpowers plugin to Antigravity from its GitHub repository. Requires plugin management, skips an existing installation, and leaves other plugins unchanged.",
		"setup-agents-codex":              "Set up global Codex instructions, configuration, optional profiles, and packaged skills. Shows every conflict before asking whether to replace or back it up.",
		"setup-agents-claude":             "Set up global Claude Code instructions, settings, and packaged skills. Shows every conflict before asking whether to replace or back it up.",
		"setup-agents-antigravity":        "Set up global Antigravity instructions, settings, and packaged skills. Shows every conflict before asking whether to replace or back it up.",
		"setup-agents-project":            "Add instruction files for selected agents to an existing project. Can update `.gitignore` and back up conflicting managed instruction files.",
		"setup-alacritty":                 "Build an Alacritty-based terminal setup on Debian or Ubuntu. Choose shell tools, fonts, desktop integrations, and default-terminal options; the existing Alacritty configuration is replaced without a backup.",
		"setup-kitty":                     "Build a Kitty-based terminal setup on Debian or Ubuntu. Choose shell tools, fonts, desktop integrations, and default-terminal options; the existing Kitty configuration is backed up before replacement.",
		"setup-windows":                   "Set up Windows Terminal, PowerShell 7, selected fonts, and terminal tools with WinGet. Backs up managed configuration when possible and reports each result.",
		"setup-wsl":                       "Set up selected shell and terminal tools on Ubuntu 22.04 or 24.04 under WSL. Uses sudo for system dependencies, backs up managed configuration when possible, and continues past optional feature failures.",
		"set-vscode-wsl-cwd":              "Open a chosen WSL directory in VS Code and use it as the working directory of a managed terminal profile. Preserves JSONC comments, backs up changed settings, and supports `-Undo`.",
		"set-default-cwd":                 "Make Bash and Zsh start in a chosen WSL directory when opened from home. Preserves unrelated shell configuration and backs up changed files.",
		"change-grub-order":               "Choose the default GRUB boot entry from an interactive list. Backs up the current GRUB settings before applying the change.",
		"setup-venv":                      "Add or remove a `venv` shell command that activates the nearest `.venv`. Keeps unrelated Bash and Zsh configuration but does not create backups.",
		"toggle-nopasswd-sudo":            "Enable or disable passwordless sudo for one Linux or WSL user. Validates enabling changes and only manages the toolbox-owned sudoers file.",
		"create-env-alias":                "Create a Bash or Zsh alias that activates a chosen `.venv`. Previews changes, confirms replacements separately, and can back up conflicts.",
		"bootstrap-python-from-venv":      "Generate requirements, `pyproject.toml`, and `.python-version` from imports found in Python files and optional notebooks. Preserves unrelated TOML, stops on ambiguous input, and can run `uv lock`.",
		"create-project-template":         "Merge the packaged project template into an existing directory without deleting destination-only files. Checks every conflict before asking to overwrite and does not create backups.",
	}
	gotCategories := []string{}
	seenCategories := map[string]bool{}
	for _, command := range catalog.Commands {
		if !seenCategories[command.Category] {
			gotCategories = append(gotCategories, command.Category)
			seenCategories[command.Category] = true
		}
		if command.Description != wantDescriptions[command.Name] {
			t.Fatalf("command %q description = %q, want %q", command.Name, command.Description, wantDescriptions[command.Name])
		}
	}
	if !reflect.DeepEqual(gotCategories, wantCategories) {
		t.Fatalf("category order = %v, want %v", gotCategories, wantCategories)
	}
}

func TestLoadCatalogRejectsInvalidEnvironmentMetadata(t *testing.T) {
	tests := []struct {
		name string
		json string
		want string
	}{
		{"missing environments", `{"commands":[{"name":"a","description":"A","package":"p","visibility":"list","protocol":"interactive-script","entrypoints":{"linux-amd64":["bash-script","a.sh"],"linux-arm64":["bash-script","a.sh"]}}]}`, "environment"},
		{"unknown environment", `{"commands":[{"name":"a","description":"A","package":"p","visibility":"list","protocol":"interactive-script","environments":["macos"],"entrypoints":{"linux-amd64":["bash-script","a.sh"],"linux-arm64":["bash-script","a.sh"]}}]}`, "macos"},
		{"unsupported elevation", `{"commands":[{"name":"a","description":"A","package":"p","visibility":"list","protocol":"interactive-script","environments":["linux-native"],"elevation":"root","entrypoints":{"linux-amd64":["bash-script","a.sh"],"linux-arm64":["bash-script","a.sh"]}}]}`, "elevation"},
		{"missing Linux entrypoint", `{"commands":[{"name":"a","description":"A","package":"p","visibility":"list","protocol":"interactive-script","environments":["linux-native"],"entrypoints":{"linux-amd64":["bash-script","a.sh"]}}]}`, "linux-arm64"},
		{"unexpected Windows entrypoint", `{"commands":[{"name":"a","description":"A","package":"p","visibility":"list","protocol":"interactive-script","environments":["linux-native"],"entrypoints":{"linux-amd64":["bash-script","a.sh"],"linux-arm64":["bash-script","a.sh"],"windows-amd64":["powershell-script","a.ps1"]}}]}`, "windows-amd64"},
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
		"setup-alacritty", "setup-kitty", "setup-windows", "setup-wsl", "set-vscode-wsl-cwd",
		"set-default-cwd", "change-grub-order", "setup-venv", "toggle-nopasswd-sudo",
		"create-env-alias", "bootstrap-python-from-venv", "create-project-template",
	}
	if len(catalog.Commands) != len(want) {
		t.Fatalf("got %d commands, want %d", len(catalog.Commands), len(want))
	}
	for index, name := range want {
		if catalog.Commands[index].Name != name {
			t.Fatalf("command %d = %q, want %q", index, catalog.Commands[index].Name, name)
		}
	}
	for _, command := range catalog.Commands {
		wantVisibility := "list"
		if command.Name == "setup-agents-project" {
			wantVisibility = "direct"
		}
		if command.Visibility != wantVisibility {
			t.Fatalf("command %q visibility = %q, want %q", command.Name, command.Visibility, wantVisibility)
		}
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
