package builtins

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const openCodeSuperpowersPackage = "superpowers@git+https://github.com/obra/superpowers.git"

var openCodeVersionPattern = regexp.MustCompile(`(?:^|\s)v?(\d+)\.(\d+)\.(\d+)(?:\s|$)`)

func openCodePluginKey(version string) string {
	match := openCodeVersionPattern.FindStringSubmatch(strings.TrimSpace(version))
	if match == nil {
		return ""
	}
	major, _ := strconv.Atoi(match[1])
	minor, _ := strconv.Atoi(match[2])
	patch, _ := strconv.Atoi(match[3])
	if major == 1 {
		return "plugin"
	}
	if major >= 2 && (major > 2 || minor > 0 || patch >= 4) {
		return "plugins"
	}
	return ""
}

func openCodeConfigPath() (string, error) {
	if directory := os.Getenv("OPENCODE_CONFIG_DIR"); directory != "" {
		return filepath.Join(directory, "opencode.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "opencode", "opencode.json"), nil
}

func openCodeConfig(path string) (map[string]json.RawMessage, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]json.RawMessage{}, nil
	}
	if err != nil {
		return nil, err
	}
	var config map[string]json.RawMessage
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("read OpenCode config: %w", err)
	}
	if config == nil {
		return nil, fmt.Errorf("OpenCode config must be a JSON object")
	}
	return config, nil
}

func openCodePluginEntries(config map[string]json.RawMessage, key string) ([]string, error) {
	raw, exists := config[key]
	if !exists {
		return nil, nil
	}
	var entries []string
	if err := json.Unmarshal(raw, &entries); err != nil || entries == nil {
		return nil, fmt.Errorf("OpenCode %s must be an array of package strings", key)
	}
	return entries, nil
}

func hasOpenCodeSuperpowers(config map[string]json.RawMessage) (bool, error) {
	for _, key := range []string{"plugin", "plugins"} {
		entries, err := openCodePluginEntries(config, key)
		if err != nil {
			return false, err
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry, "superpowers@") || strings.Contains(entry, "/superpowers") {
				return true, nil
			}
		}
	}
	return false, nil
}

func addOpenCodeSuperpowers(path, key string) (bool, error) {
	config, err := openCodeConfig(path)
	if err != nil {
		return false, err
	}
	exists, err := hasOpenCodeSuperpowers(config)
	if err != nil || exists {
		return false, err
	}
	entries, err := openCodePluginEntries(config, key)
	if err != nil {
		return false, err
	}
	entries = append(entries, openCodeSuperpowersPackage)
	encodedEntries, err := json.Marshal(entries)
	if err != nil {
		return false, err
	}
	config[key] = encodedEntries
	encoded, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return false, err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".opencode-config-*")
	if err != nil {
		return false, err
	}
	defer os.Remove(temporary.Name())
	if _, err := temporary.Write(append(encoded, '\n')); err != nil {
		temporary.Close()
		return false, err
	}
	if err := temporary.Close(); err != nil {
		return false, err
	}
	if err := os.Rename(temporary.Name(), path); err != nil {
		return false, err
	}
	return true, nil
}

func (builtins *ToolboxBuiltins) installOpenCodeSuperpowers() error {
	path, err := resolveUserCommand("opencode", builtins.platform)
	if err != nil {
		return fmt.Errorf("OpenCode is required: %w", err)
	}
	versionOutput, err := exec.Command(path, "--version").Output()
	if err != nil {
		return fmt.Errorf("check OpenCode version: %w", err)
	}
	key := openCodePluginKey(string(bytes.TrimSpace(versionOutput)))
	if key == "" {
		return fmt.Errorf("unsupported OpenCode version %q: use V1 or V2 2.0.4 or newer", strings.TrimSpace(string(versionOutput)))
	}
	configPath, err := openCodeConfigPath()
	if err != nil {
		return err
	}
	installed, err := addOpenCodeSuperpowers(configPath, key)
	if err != nil {
		return err
	}
	if !installed {
		return builtins.info("Superpowers is already configured for OpenCode.")
	}
	return builtins.info("Added Superpowers to OpenCode configuration. Restart OpenCode to load it.")
}
