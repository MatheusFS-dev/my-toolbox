package main

import (
	"crypto/sha256"
	"fmt"
	"reflect"
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

func TestRepositoryCatalogPreservesV016ExecutionMetadata(t *testing.T) {
	catalog, err := LoadCatalogFile("../commands.json")
	if err != nil {
		t.Fatal(err)
	}
	signatures := []string{}
	for _, command := range catalog.Commands {
		entrypoints := []string{}
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
			strings.Join(entrypoints, ";"),
		}, "|"))
	}
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(signatures, "\n"))))
	const want = "d575a8f3cd6e00b5c2f38e6592875faba97866560c97427768e282752dccfd1f"
	if digest != want {
		t.Fatalf("execution metadata digest = %s, want v0.1.6 digest %s", digest, want)
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
		"install-codex":                   "Download and execute OpenAI’s official Codex installer for the current Linux or Windows platform when `codex` is not already available from PATH or its supported user installation path. Run with closed input; the toolbox performs no separate post-install executable or configuration check.",
		"install-claude":                  "Download and execute Anthropic’s official Claude Code installer for the current Linux or Windows platform when `claude` is not already available from PATH or its supported user installation path. Run with closed input; the toolbox performs no separate post-install executable or configuration check.",
		"install-antigravity":             "Download and execute Google’s official Antigravity CLI installer for the current Linux or Windows platform when `agy` is not already available from PATH or its supported user installation path. Run with closed input; the toolbox performs no separate post-install executable or configuration check.",
		"install-uv":                      "Download and execute Astral’s official uv installer for the current Linux or Windows platform when `uv` is unavailable. Set `UV_NO_MODIFY_PATH=1` so the installer does not edit shell PATH configuration; the toolbox performs no separate post-install executable check.",
		"install-gh":                      "Resolve the latest GitHub CLI release, download its platform archive and published SHA-256 checksums, verify the selected archive, and atomically install only the `gh` executable into `~/.local/bin` on Linux or `%LOCALAPPDATA%\\my-toolbox\\bin` on Windows. Print PATH guidance when that directory is not active.",
		"install-superpowers-codex":       "Require an installed Codex CLI with plugin management, inspect `codex plugin list`, and add `superpowers@openai-curated` when absent. Skip an existing Superpowers installation and leave other Codex plugins unchanged.",
		"install-superpowers-claude":      "Require an installed Claude Code CLI with plugin management, inspect `claude plugin list`, and install `superpowers@claude-plugins-official` at user scope when absent. Skip an existing installation and leave other Claude plugins unchanged.",
		"install-superpowers-antigravity": "Require an installed Antigravity CLI with plugin management, inspect `agy plugin list`, and install Superpowers from its GitHub repository when absent. Skip an existing installation and leave other Antigravity plugins unchanged.",
		"setup-agents-codex":              "Validate the packaged global instructions, Codex configuration template, optional profiles, and skill packages; render valid TOML into `~/.codex/config.toml`; and install selected profiles and packaged skills under `~/.codex`. List all managed conflicts before writing, require replacement confirmation, and optionally create adjacent backups.",
		"setup-agents-claude":             "Validate the packaged instructions, Claude settings JSON, and skill packages, then install `CLAUDE.md`, `settings.json`, and each packaged skill under `~/.claude`. List all managed conflicts before writing, require replacement confirmation, and optionally create adjacent backups.",
		"setup-agents-antigravity":        "Validate the packaged instructions, Antigravity settings JSON, and skill packages, then install `GEMINI.md` under `~/.gemini` and settings and skills under `~/.gemini/antigravity-cli`. List all managed conflicts before writing, require replacement confirmation, and optionally create adjacent backups.",
		"setup-agents-project":            "Prompt for an existing project and selected agent formats, install `AGENTS.md` for Codex or Antigravity and `CLAUDE.md` for Claude, and optionally append managed instruction and Superpowers paths to `.gitignore`. Preserve unrelated `.gitignore` content and optionally back up conflicting managed instruction files.",
		"setup-alacritty":                 "On native Debian or Ubuntu Linux, run through sudo and prompt for Alacritty, Zsh, Rust, Zellij, Starship, fonts, shell helpers, Nautilus integration, and default-terminal configuration. Apply system packages and user configuration under the invoking user’s home; optional module failures do not stop later modules, and selected Alacritty configuration replaces `~/.config/alacritty/alacritty.toml` without a backup.",
		"setup-kitty":                     "On native Debian or Ubuntu Linux, run through sudo and prompt for Kitty, Zsh, Rust, Zellij, Starship, fonts, shell helpers, Nautilus integration, and default-terminal configuration. Install Kitty under the invoking user’s home, continue after optional module failures, and back up an existing `~/.config/kitty/kitty.conf` before replacing it.",
		"setup-windows":                   "On native Windows 10 build 17763 or newer, or Windows 11, use WinGet to install or update Windows Terminal and PowerShell 7 plus selected fonts and terminal utilities. Write managed Terminal, PowerShell, Starship, Zellij, and VS Code configuration, attempt a timestamped backup under `%LOCALAPPDATA%\\project-template\\windows-backups`, and report each feature’s result.",
		"setup-wsl":                       "On Ubuntu 22.04 or 24.04 under WSL, run through sudo and install selected Zsh, Rust, Zellij, Starship, eza, fzf, and shell-integration features for the invoking user. Install apt dependencies in one pass, attempt backups under `~/.local/state/project-template/wsl-backups`, continue after optional feature failures, and report that the terminal must be restarted.",
		"set-vscode-wsl-cwd":              "On Windows, validate an absolute directory in the default WSL distribution, add or update the managed `WSL (project-template)` terminal profile in VS Code’s user `settings.json`, and open that WSL directory in VS Code. Preserve JSONC comments and file encoding, create a backup when settings change, and support `-Undo` for removing the managed profile.",
		"set-default-cwd":                 "On WSL, prompt for an explicit existing absolute directory and update managed blocks in `~/.bashrc` and `~/.zshrc` so shells starting in the home directory change to it. Preserve unrelated content, permissions, and line endings; back up changed existing files and reject symlinks or malformed markers.",
		"change-grub-order":               "On native Linux through sudo, parse `/boot/grub/grub.cfg`, display submenu-aware boot entries and the current default, and prompt for a replacement. Back up `/etc/default/grub`, update `GRUB_DEFAULT`, optionally disable `GRUB_SAVEDEFAULT`, run `update-grub`, and print the resulting configuration.",
		"setup-venv":                      "On Linux or WSL, install or remove a managed `venv` shell function in `~/.bashrc` and, when Zsh is installed, `~/.zshrc`. The function searches the current directory and its parents for `.venv/bin/activate`; unrelated shell content is retained, but this script does not create backups.",
		"toggle-nopasswd-sudo":            "On Linux or WSL through sudo, detect whether the selected user currently has passwordless sudo and offer the opposite action. Manage only `/etc/sudoers.d/99-<user>-nopasswd`, validate an enabling rule with `visudo`, verify the effective sudo state afterward, and report external rules when disabling cannot remove NOPASSWD access.",
		"create-env-alias":                "Validate a selected virtual environment or project containing `.venv`, prompt for an alias and explicit Bash, Zsh, or both selection, and preview activation changes to `~/.bashrc` and/or `~/.zshrc`. Require separate replacement confirmation, optionally back up conflicts, preserve unrelated content and permissions, and replace each file atomically.",
		"bootstrap-python-from-venv":      "Scan project Python files and optional notebooks once, exclude standard-library and local modules, and map remaining imports through a selected virtual environment’s distribution metadata. Generate unpinned requirements, `pyproject.toml`, and `.python-version`, preserve unrelated TOML, stop on malformed or ambiguous input, optionally back up conflicts, and optionally validate and run `uv lock`.",
		"create-project-template":         "Dynamically discover and recursively merge the packaged template into an explicit existing destination, including dotfiles, empty directories, and symlinks. Preserve destination-only paths, reject overlap and entry-type mismatches before copying, list same-type conflicts once, and overwrite them only after explicit confirmation without creating template backups.",
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
