package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var rememberedTools = map[string]bool{
	"setup-agents-codex":       true,
	"setup-agents-claude":      true,
	"setup-agents-antigravity": true,
}

func toolMarkerPath(platform, name string) (string, error) {
	root, err := toolboxDataRoot(platform)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "used-tools", name), nil
}

func rememberTool(platform, name string) error {
	if !rememberedTools[name] {
		return nil
	}
	path, err := toolMarkerPath(platform, name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create remembered tools directory: %w", err)
	}
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		return fmt.Errorf("remember %s: %w", name, err)
	}
	return nil
}

func rememberedCatalogTools(platform string, catalog Catalog, environment string) ([]string, error) {
	names := []string{}
	for _, command := range catalog.Commands {
		if !rememberedTools[command.Name] || !command.SupportsEnvironment(environment) {
			continue
		}
		path, err := toolMarkerPath(platform, command.Name)
		if err != nil {
			return nil, err
		}
		if _, err := os.Stat(path); err == nil {
			names = append(names, command.Name)
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("inspect remembered tool %s: %w", command.Name, err)
		}
	}
	return names, nil
}

func activeToolboxRoot(platform string) (string, error) {
	root, err := toolboxDataRoot(platform)
	if err != nil {
		return "", err
	}
	versionBytes, err := os.ReadFile(filepath.Join(root, "current.txt"))
	if err != nil {
		return "", fmt.Errorf("read active toolbox version: %w", err)
	}
	activeVersion := strings.TrimSpace(string(versionBytes))
	if _, err := parseVersion(activeVersion); err != nil {
		return "", fmt.Errorf("invalid active toolbox version %q: %w", activeVersion, err)
	}
	return filepath.Join(root, "versions", activeVersion), nil
}

func runActiveTool(platform, name string, output, errorOutput io.Writer) error {
	root, err := activeToolboxRoot(platform)
	if err != nil {
		return err
	}
	filename := "tb"
	if platform == "windows-amd64" {
		filename = "tb.exe"
	}
	command := exec.Command(filepath.Join(root, filename), name)
	command.Stdin = os.Stdin
	command.Stdout = output
	command.Stderr = errorOutput
	if err := command.Run(); err != nil {
		return fmt.Errorf("refresh %s: %w", name, err)
	}
	return nil
}
