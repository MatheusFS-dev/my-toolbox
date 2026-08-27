package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
)

var commandNamePattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)

var reservedCommands = map[string]bool{
	"help":    true,
	"list":    true,
	"update":  true,
	"version": true,
}

var supportedPlatforms = []string{"linux-amd64", "linux-arm64", "windows-amd64"}

// Command describes one editable catalog tool and its platform entrypoints.
type Command struct {
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Package     string              `json:"package"`
	Protocol    string              `json:"protocol"`
	Entrypoints map[string][]string `json:"entrypoints"`
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
		if command.Protocol != "builtin" && command.Protocol != "questionnaire" {
			return Catalog{}, fmt.Errorf("command %q has unsupported protocol %q", command.Name, command.Protocol)
		}
		for _, platform := range supportedPlatforms {
			entrypoint := command.Entrypoints[platform]
			if len(entrypoint) < 2 || entrypoint[0] == "" || entrypoint[1] == "" {
				return Catalog{}, fmt.Errorf("command %q has invalid %s entrypoint", command.Name, platform)
			}
		}
	}
	return catalog, nil
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
