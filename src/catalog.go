package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

var commandNamePattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)

var reservedCommands = map[string]bool{
	"help":      true,
	"list":      true,
	"uninstall": true,
	"update":    true,
	"version":   true,
}

var supportedPlatforms = []string{"linux-amd64", "linux-arm64", "windows-amd64"}

// Command describes one editable catalog tool and its platform entrypoints.
type Command struct {
	Name                    string              `json:"name"`
	Category                string              `json:"category"`
	Description             string              `json:"description"`
	Package                 string              `json:"package"`
	Visibility              string              `json:"visibility"`
	Protocol                string              `json:"protocol"`
	Environments            []string            `json:"environments"`
	Elevation               string              `json:"elevation,omitempty"`
	DefaultArguments        []string            `json:"default_arguments,omitempty"`
	Requirements            map[string][]string `json:"requirements,omitempty"`
	Entrypoints             map[string][]string `json:"entrypoints"`
	presentationEnvironment string
}

func (command Command) requirementText() string {
	if command.presentationEnvironment == "" {
		return ""
	}
	capabilities, err := ResolveRequirements(command, command.presentationEnvironment)
	if err != nil || len(capabilities) == 0 {
		return ""
	}
	labels := make([]string, len(capabilities))
	for index, capability := range capabilities {
		labels[index] = capability.Label
	}
	return "Requires: " + strings.Join(labels, "; ")
}

// Catalog preserves the user-visible command order from commands.json.
type Catalog struct {
	Commands []Command `json:"commands"`
}

// LoadCatalog decodes and validates a command catalog.
//
// Args:
//   - reader: JSON source containing the complete command catalog.
//
// Returns:
//   - Catalog: Validated catalog preserving source order.
//   - error: Decode or validation failure.
func LoadCatalog(reader io.Reader) (Catalog, error) {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	var catalog Catalog
	if err := decoder.Decode(&catalog); err != nil {
		return Catalog{}, fmt.Errorf("decode command catalog: %w", err)
	}
	if len(catalog.Commands) == 0 {
		return Catalog{}, fmt.Errorf("command catalog must contain at least one command")
	}
	seen := map[string]bool{}
	for index, command := range catalog.Commands {
		if !commandNamePattern.MatchString(command.Name) {
			return Catalog{}, fmt.Errorf("command %d has invalid name %q", index, command.Name)
		}
		if reservedCommands[command.Name] {
			return Catalog{}, fmt.Errorf("command name %q is reserved", command.Name)
		}
		if seen[command.Name] {
			return Catalog{}, fmt.Errorf("duplicate command name %q", command.Name)
		}
		seen[command.Name] = true
		if command.Description == "" {
			return Catalog{}, fmt.Errorf("command %q has an empty description", command.Name)
		}
		if command.Package == "" {
			return Catalog{}, fmt.Errorf("command %q has an empty package", command.Name)
		}
		if command.Visibility != "list" && command.Visibility != "direct" {
			return Catalog{}, fmt.Errorf("command %q has unsupported visibility %q", command.Name, command.Visibility)
		}
		if command.Protocol != "builtin" && command.Protocol != "questionnaire" && command.Protocol != "interactive-python" && command.Protocol != "interactive-script" {
			return Catalog{}, fmt.Errorf("command %q has unsupported protocol %q", command.Name, command.Protocol)
		}
		if len(command.Environments) == 0 {
			return Catalog{}, fmt.Errorf("command %q must declare at least one environment", command.Name)
		}
		environments := map[string]bool{}
		for _, environment := range command.Environments {
			if environment != "linux-native" && environment != "linux-wsl" && environment != "windows" {
				return Catalog{}, fmt.Errorf("command %q has unsupported environment %q", command.Name, environment)
			}
			if environments[environment] {
				return Catalog{}, fmt.Errorf("command %q repeats environment %q", command.Name, environment)
			}
			environments[environment] = true
		}
		if command.Elevation != "" && command.Elevation != "sudo" {
			return Catalog{}, fmt.Errorf("command %q has unsupported elevation %q", command.Name, command.Elevation)
		}
		if command.Elevation == "sudo" && (command.Protocol != "interactive-script" || environments["windows"]) {
			return Catalog{}, fmt.Errorf("command %q has invalid sudo elevation", command.Name)
		}
		if err := validateRequirements(command, environments); err != nil {
			return Catalog{}, err
		}
		for _, platform := range supportedPlatforms {
			required := environments["windows"]
			if strings.HasPrefix(platform, "linux-") {
				required = environments["linux-native"] || environments["linux-wsl"]
			}
			entrypoint, exists := command.Entrypoints[platform]
			if !required {
				if exists {
					return Catalog{}, fmt.Errorf("command %q has unexpected %s entrypoint", command.Name, platform)
				}
				continue
			}
			entrypointType := map[string]string{
				"builtin":            "builtin",
				"questionnaire":      "python-adapter",
				"interactive-python": "python-script",
			}[command.Protocol]
			if command.Protocol == "interactive-script" {
				entrypointType = "bash-script"
				if platform == "windows-amd64" {
					entrypointType = "powershell-script"
				}
			}
			if len(entrypoint) < 2 || entrypoint[0] != entrypointType || entrypoint[1] == "" {
				return Catalog{}, fmt.Errorf("command %q has invalid %s entrypoint", command.Name, platform)
			}
			if command.Protocol == "interactive-python" && len(entrypoint) > 2 && entrypoint[2] == "" {
				return Catalog{}, fmt.Errorf("command %q has invalid %s Python 2.7 fallback entrypoint", command.Name, platform)
			}
		}
		if command.Category == "" {
			return Catalog{}, fmt.Errorf("command %q has an empty category", command.Name)
		}
	}
	return catalog, nil
}

// SupportsEnvironment reports whether a command is available in one detected environment.
//
// Args:
//   - environment: One validated environment name: linux-native, linux-wsl, or windows.
//
// Returns:
//   - bool: True when the command explicitly declares the environment.
func (command Command) SupportsEnvironment(environment string) bool {
	for _, supported := range command.Environments {
		if supported == environment {
			return true
		}
	}
	return false
}

// LoadCatalogFile opens and validates a catalog from disk.
//
// Args:
//   - path: Filesystem path to commands.json.
//
// Returns:
//   - Catalog: Validated catalog.
//   - error: Open, decode, or validation failure.
func LoadCatalogFile(path string) (Catalog, error) {
	file, err := os.Open(path)
	if err != nil {
		return Catalog{}, fmt.Errorf("open command catalog: %w", err)
	}
	defer file.Close()
	return LoadCatalog(file)
}

// Find returns a catalog command by exact public name.
//
// Args:
//   - name: Public command name.
//
// Returns:
//   - Command: Matching command when found.
//   - bool: True when the command exists.
func (catalog Catalog) Find(name string) (Command, bool) {
	for _, command := range catalog.Commands {
		if command.Name == name {
			return command, true
		}
	}
	return Command{}, false
}
