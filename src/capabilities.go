package main

import (
	"fmt"
	"strings"
)

// Capability describes one hard pre-start requirement.
type Capability struct {
	ID           string
	Label        string
	Remediation  string
	Environments map[string]bool
}

func environments(names ...string) map[string]bool {
	result := make(map[string]bool, len(names))
	for _, name := range names {
		result[name] = true
	}
	return result
}

var capabilityRegistry = map[string]Capability{
	"bash":                          {ID: "bash", Label: "Bash", Remediation: "Install Bash and ensure 'bash' is on PATH.", Environments: environments("linux-native", "linux-wsl")},
	"powershell":                    {ID: "powershell", Label: "Windows PowerShell 5.1 or PowerShell 7", Remediation: "Install Windows PowerShell 5.1 or PowerShell 7.", Environments: environments("windows")},
	"python-workspace-linux":        {ID: "python-workspace-linux", Label: "Python 3.9+, or Python 2.7 with toml==0.10.2", Remediation: "Install Python 3.9 or newer, or install Python 2.7 and run: python2.7 -m pip install --user toml==0.10.2", Environments: environments("linux-native", "linux-wsl")},
	"python39":                      {ID: "python39", Label: "Python 3.9+", Remediation: "Install Python 3.9 or newer.", Environments: environments("linux-native", "linux-wsl", "windows")},
	"python3":                       {ID: "python3", Label: "Python 3", Remediation: "Install Python 3 and ensure 'python3' is on PATH.", Environments: environments("linux-native", "linux-wsl")},
	"sudo":                          {ID: "sudo", Label: "sudo", Remediation: "Install sudo and ensure 'sudo' is on PATH.", Environments: environments("linux-native", "linux-wsl")},
	"codex-plugin-management":       {ID: "codex-plugin-management", Label: "Codex with plugin management", Remediation: "Install or update Codex to a version that supports 'codex plugin'.", Environments: environments("linux-native", "linux-wsl", "windows")},
	"claude-plugin-management":      {ID: "claude-plugin-management", Label: "Claude Code with plugin management", Remediation: "Install or update Claude Code to a version that supports 'claude plugin'.", Environments: environments("linux-native", "linux-wsl", "windows")},
	"antigravity-plugin-management": {ID: "antigravity-plugin-management", Label: "Antigravity with plugin management", Remediation: "Install or update Antigravity to a version that supports 'agy plugin'.", Environments: environments("linux-native", "linux-wsl", "windows")},
	"apt-get":                       {ID: "apt-get", Label: "apt-get", Remediation: "Install apt and ensure 'apt-get' is on PATH.", Environments: environments("linux-native", "linux-wsl")},
	"debian-ubuntu":                 {ID: "debian-ubuntu", Label: "Debian or Ubuntu", Remediation: "Run this tool on a supported Debian or Ubuntu installation.", Environments: environments("linux-native")},
	"wsl-ubuntu-supported":          {ID: "wsl-ubuntu-supported", Label: "WSL Ubuntu 22.04 or 24.04", Remediation: "Run this tool in WSL on Ubuntu 22.04 or 24.04.", Environments: environments("linux-wsl")},
	"windows-build-supported":       {ID: "windows-build-supported", Label: "Windows 10 build 17763+ or Windows 11", Remediation: "Update Windows to Windows 10 build 17763 or newer, or Windows 11.", Environments: environments("windows")},
	"winget":                        {ID: "winget", Label: "WinGet", Remediation: "Install or update App Installer from Microsoft Store to provide WinGet.", Environments: environments("windows")},
	"wsl":                           {ID: "wsl", Label: "WSL", Remediation: "Install WSL and at least one Linux distribution.", Environments: environments("windows")},
	"vscode-wsl":                    {ID: "vscode-wsl", Label: "VS Code with WSL support", Remediation: "Install VS Code and its WSL extension, and ensure 'code' is on PATH.", Environments: environments("windows")},
	"grub-files":                    {ID: "grub-files", Label: "GRUB configuration files", Remediation: "Install and configure GRUB so /etc/default/grub and /boot/grub/grub.cfg exist.", Environments: environments("linux-native")},
	"grub-utilities":                {ID: "grub-utilities", Label: "GRUB utilities", Remediation: "Install the GRUB utilities package that provides 'update-grub'.", Environments: environments("linux-native")},
	"visudo":                        {ID: "visudo", Label: "visudo", Remediation: "Install sudo utilities so 'visudo' is on PATH.", Environments: environments("linux-native", "linux-wsl")},
}

func init() {
	for _, name := range []string{"awk", "cat", "chmod", "chown", "cmp", "cp", "cut", "date", "dirname", "env", "getent", "grep", "id", "install", "mktemp", "mv", "od", "rm", "sort", "tail", "tr"} {
		capabilityRegistry[name] = Capability{ID: name, Label: name, Remediation: fmt.Sprintf("Install the utility that provides '%s' and ensure it is on PATH.", name), Environments: environments("linux-native", "linux-wsl")}
	}
}

func validateRequirements(command Command, declaredEnvironments map[string]bool) error {
	for environment, ids := range command.Requirements {
		if !declaredEnvironments[environment] {
			return fmt.Errorf("command %q declares requirements for undeclared environment %q", command.Name, environment)
		}
		derived, err := derivedRequirementIDs(command, environment)
		if err != nil {
			return err
		}
		derivedSet := map[string]bool{}
		for _, id := range derived {
			derivedSet[id] = true
		}
		seen := map[string]bool{}
		for _, id := range ids {
			capability, exists := capabilityRegistry[id]
			if !exists {
				return fmt.Errorf("command %q declares unknown capability %q", command.Name, id)
			}
			if seen[id] {
				return fmt.Errorf("command %q repeats capability %q for %s", command.Name, id, environment)
			}
			if derivedSet[id] {
				return fmt.Errorf("command %q explicitly repeats derived capability %q for %s", command.Name, id, environment)
			}
			if !capability.Environments[environment] {
				return fmt.Errorf("capability %q is incompatible with environment %q", id, environment)
			}
			seen[id] = true
		}
	}
	return nil
}

// ResolveRequirements returns derived and explicit requirements in stable order.
func ResolveRequirements(command Command, environment string) ([]Capability, error) {
	ids, err := derivedRequirementIDs(command, environment)
	if err != nil {
		return nil, err
	}
	ids = append(ids, command.Requirements[environment]...)
	result := make([]Capability, 0, len(ids))
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			return nil, fmt.Errorf("command %q repeats resolved capability %q", command.Name, id)
		}
		capability, exists := capabilityRegistry[id]
		if !exists {
			return nil, fmt.Errorf("command %q has unknown capability %q", command.Name, id)
		}
		if !capability.Environments[environment] {
			return nil, fmt.Errorf("capability %q is incompatible with environment %q", id, environment)
		}
		seen[id] = true
		result = append(result, capability)
	}
	return result, nil
}

func derivedRequirementIDs(command Command, environment string) ([]string, error) {
	ids := []string{}
	if command.Protocol == "interactive-script" {
		if environment == "windows" {
			ids = append(ids, "powershell")
		} else {
			ids = append(ids, "bash")
		}
	}
	if command.Protocol == "interactive-python" || command.Protocol == "questionnaire" {
		if environment == "windows" {
			ids = append(ids, "python39")
		} else if hasPythonFallback(command) {
			ids = append(ids, "python-workspace-linux")
		} else {
			ids = append(ids, "python39")
		}
	}
	if command.Elevation == "sudo" {
		ids = append(ids, "sudo")
	}
	if isOfficialScriptInstaller(command.Name) {
		if environment == "windows" {
			ids = append(ids, "powershell")
		} else {
			ids = append(ids, "bash")
		}
	}
	return ids, nil
}

func hasPythonFallback(command Command) bool {
	for platform, entrypoint := range command.Entrypoints {
		if strings.HasPrefix(platform, "linux-") && len(entrypoint) > 2 {
			return true
		}
	}
	return false
}

func isOfficialScriptInstaller(name string) bool {
	switch name {
	case "install-codex", "install-claude", "install-antigravity", "install-uv":
		return true
	default:
		return false
	}
}
