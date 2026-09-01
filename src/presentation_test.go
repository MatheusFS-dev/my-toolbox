package main

import (
	"os"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestFilteredCommandsGroupAfterEnvironmentAndVisibilityFiltering(t *testing.T) {
	catalog := Catalog{Commands: []Command{
		{Name: "native-list", Category: "Native", Visibility: "list", Environments: []string{"linux-native"}},
		{Name: "native-direct", Category: "Direct", Visibility: "direct", Environments: []string{"linux-native"}},
		{Name: "windows-list", Category: "Windows", Visibility: "list", Environments: []string{"windows"}},
	}}
	groups := groupCommands(filteredCommands(catalog, "linux-native", false))
	if len(groups) != 1 || groups[0].Category != "Native" || len(groups[0].Commands) != 1 {
		t.Fatalf("list groups = %#v", groups)
	}
	groups = groupCommands(filteredCommands(catalog, "linux-native", true))
	if len(groups) != 2 || groups[1].Category != "Direct" {
		t.Fatalf("help groups = %#v", groups)
	}
}

func TestRedirectedHelpMatchesEnvironmentFixtures(t *testing.T) {
	catalog := Catalog{Commands: []Command{
		{Name: "direct-project", Category: "Agent Workspace", Description: "A supported direct-only project tool.", Visibility: "direct", Environments: []string{"linux-native", "linux-wsl", "windows"}},
		{Name: "native-tool", Category: "Terminal", Description: "A native Linux tool.", Visibility: "list", Environments: []string{"linux-native"}},
		{Name: "wsl-tool", Category: "Terminal", Description: "A WSL tool.", Visibility: "list", Environments: []string{"linux-wsl"}},
		{Name: "windows-tool", Category: "Terminal", Description: "A Windows tool.", Visibility: "list", Environments: []string{"windows"}},
	}}
	tests := []struct {
		environment string
		fixture     string
	}{
		{environment: "linux-native", fixture: "testdata/help-linux-native.txt"},
		{environment: "linux-wsl", fixture: "testdata/help-linux-wsl.txt"},
		{environment: "windows", fixture: "testdata/help-windows.txt"},
	}
	for _, test := range tests {
		t.Run(test.environment, func(t *testing.T) {
			want, err := os.ReadFile(test.fixture)
			if err != nil {
				t.Fatal(err)
			}
			got := renderHelp(filteredCommands(catalog, test.environment, true), "1.2.3", 80, false)
			if got != string(want) {
				t.Fatalf("redirected help mismatch\n--- got ---\n%s--- want ---\n%s", got, want)
			}
			if strings.Contains(got, "\x1b[") {
				t.Fatalf("redirected help contains ANSI: %q", got)
			}
		})
	}
}

func TestPlainHelpHasExactHierarchyAndWrapping(t *testing.T) {
	commands := []Command{
		{Name: "native-tool", Category: "Tools", Description: "A native tool with a description that wraps onto another line at this width."},
		{Name: "direct-tool", Category: "Direct Tools", Description: "A direct-only tool."},
	}
	want := "TOOLBOX 1.2.3\n" +
		"  Portable terminal tools for supported Linux, WSL, and Windows\n" +
		"  environments.\n\n" +
		"USAGE\n" +
		"  tb list\n" +
		"    Select supported tools interactively and run them in catalog order.\n" +
		"  tb <tool> [arguments...]\n" +
		"    Run one supported catalog tool directly, forwarding its arguments.\n" +
		"  tb update\n" +
		"    Reinstall the toolbox when a newer release is available.\n" +
		"  tb uninstall\n" +
		"    Confirm and remove the toolbox wrapper and installed versions.\n" +
		"  tb version\n" +
		"    Print the installed toolbox version.\n" +
		"  tb help\n" +
		"    Show this usage and the tools supported in the current environment.\n\n" +
		"AVAILABLE TOOLS\n" +
		"  Tools\n" +
		"    native-tool\n" +
		"      A native tool with a description that wraps onto another line at\n" +
		"      this width.\n\n" +
		"  Direct Tools\n" +
		"    direct-tool\n" +
		"      A direct-only tool.\n"
	got := renderHelp(commands, "1.2.3", 72, false)
	if got != want {
		t.Fatalf("plain help mismatch\n--- got ---\n%s--- want ---\n%s", got, want)
	}
	if strings.Contains(got, "\x1b[") {
		t.Fatalf("plain help contains ANSI: %q", got)
	}
}

func TestTTYHelpStylesHeadingsWithoutBackgrounds(t *testing.T) {
	commands := []Command{{Name: "tool", Category: "Tools", Description: "Description."}}
	help := renderHelp(commands, "1.2.3", 72, true)
	for _, heading := range []string{"TOOLBOX 1.2.3", "USAGE", "AVAILABLE TOOLS", "Tools"} {
		if !strings.Contains(help, "\x1b[1;37m"+heading) {
			t.Fatalf("TTY help is missing styled heading %q: %q", heading, help)
		}
	}
	if strings.Contains(help, "\x1b[4") || strings.ContainsAny(help, "│┌┐└┘") {
		t.Fatalf("TTY help contains background or borders: %q", help)
	}
}

func TestHelpRendersRequirementsBrightRedOrPlainAndWrapsThem(t *testing.T) {
	catalog := Catalog{Commands: []Command{{
		Name: "setup", Category: "Tools", Description: "Description.", Visibility: "list",
		Protocol: "interactive-script", Environments: []string{"linux-native"},
		Requirements: map[string][]string{"linux-native": {"apt-get", "debian-ubuntu"}},
	}}}
	commands := filteredCommands(catalog, "linux-native", true)
	plain := renderHelp(commands, "1.2.3", 38, false)
	want := "      Requires: Bash; apt-get; Debian\n      or Ubuntu\n"
	if !strings.Contains(plain, want) {
		t.Fatalf("plain help = %q, want containing %q", plain, want)
	}
	if strings.Contains(plain, "\x1b[") {
		t.Fatalf("plain help contains ANSI: %q", plain)
	}
	styled := renderHelp(commands, "1.2.3", 72, true)
	if !strings.Contains(styled, "\x1b[91mRequires: Bash; apt-get; Debian or Ubuntu") {
		t.Fatalf("styled help lacks bright-red requirement: %q", styled)
	}
}

func TestHelpOmitsRequiresLineForCommandWithoutRequirements(t *testing.T) {
	catalog := Catalog{Commands: []Command{{Name: "install-gh", Category: "Tools", Description: "Description.", Visibility: "list", Protocol: "builtin", Environments: []string{"linux-native"}}}}
	help := renderHelp(filteredCommands(catalog, "linux-native", true), "1.2.3", 72, false)
	if strings.Contains(help, "Requires:") {
		t.Fatalf("help = %q", help)
	}
}

func TestHelpCapsWideTerminalsAndUsesNarrowLiveWidth(t *testing.T) {
	command := Command{Name: "tool", Category: "Tools", Description: "A description with enough words to wrap across multiple lines in a narrow terminal."}
	for _, test := range []struct {
		terminalWidth int
		maxWidth      int
	}{{terminalWidth: 120, maxWidth: 72}, {terminalWidth: 32, maxWidth: 32}} {
		for _, line := range strings.Split(renderHelp([]Command{command}, "1.2.3", test.terminalWidth, false), "\n") {
			if lipgloss.Width(line) > test.maxWidth {
				t.Fatalf("terminal width %d produced line width %d: %q", test.terminalWidth, lipgloss.Width(line), line)
			}
		}
	}
}
