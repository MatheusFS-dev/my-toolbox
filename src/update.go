package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const toolboxRepository = "MatheusFS-dev/my-toolbox"

const (
	toolboxLinuxInstallerURL   = "https://matheusfs-dev.github.io/my-toolbox/install.sh"
	toolboxWindowsInstallerURL = "https://matheusfs-dev.github.io/my-toolbox/install.ps1"
)

// update checks for a newer release and replaces the installation through the bootstrap installer.
//
// The installer is downloaded before managed files are removed. Unix performs
// the removal and reinstall synchronously. Windows schedules both operations
// through the detached cleanup helper because the running executable is locked.
//
// Args: None.
//
// Returns:
//   - error: Release lookup, version validation, installer download, managed
//     removal, installer execution, or status-output failure.
func (builtins *ToolboxBuiltins) update() error {
	var release struct {
		TagName string `json:"tag_name"`
	}
	metadata, err := builtins.download("https://api.github.com/repos/" + toolboxRepository + "/releases/latest")
	if err != nil {
		return err
	}
	if err := json.Unmarshal(metadata, &release); err != nil {
		return fmt.Errorf("decode latest toolbox release: %w", err)
	}
	if release.TagName == "" {
		return fmt.Errorf("latest toolbox release is missing tag_name")
	}
	latest := strings.TrimPrefix(release.TagName, "v")
	comparison, err := compareVersions(latest, builtins.version)
	if err != nil {
		return err
	}
	if comparison == 0 {
		_, err := fmt.Fprintf(builtins.output, "Toolbox %s is already current.\n", latest)
		return err
	}
	if comparison < 0 {
		return fmt.Errorf("latest release %s is older than installed version %s", latest, builtins.version)
	}

	installerURL := toolboxLinuxInstallerURL
	extension := ".sh"
	if builtins.platform == "windows-amd64" {
		installerURL = toolboxWindowsInstallerURL
		extension = ".ps1"
	} else if builtins.platform != "linux-amd64" && builtins.platform != "linux-arm64" {
		return fmt.Errorf("unsupported update platform %s", builtins.platform)
	}
	installer, err := builtins.download(installerURL)
	if err != nil {
		return err
	}
	installerPath, err := writeTemporaryInstaller(installer, extension)
	if err != nil {
		return err
	}
	if builtins.platform != "windows-amd64" {
		defer os.Remove(installerPath)
	}
	statusPath, err := builtins.remove(installerPath)
	if err != nil {
		if builtins.platform == "windows-amd64" {
			_ = os.Remove(installerPath)
		}
		return err
	}
	if builtins.platform == "windows-amd64" {
		_, err = fmt.Fprintf(builtins.output, "Update scheduled; status will be written to %s after tb exits.\n", statusPath)
	}
	return err
}

// toolboxDataRoot resolves the managed data directory for one platform.
//
// Args:
//   - platform: Validated platform key. windows-amd64 uses LOCALAPPDATA;
//     Linux platforms use XDG_DATA_HOME when set or ~/.local/share otherwise.
//
// Returns:
//   - string: Absolute or home-derived my-toolbox data directory.
//   - error: Missing LOCALAPPDATA or user-home lookup failure.
func toolboxDataRoot(platform string) (string, error) {
	if platform == "windows-amd64" {
		root := os.Getenv("LOCALAPPDATA")
		if root == "" {
			return "", fmt.Errorf("LOCALAPPDATA is not set")
		}
		return filepath.Join(root, "my-toolbox"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home: %w", err)
	}
	if root := os.Getenv("XDG_DATA_HOME"); root != "" {
		return filepath.Join(root, "my-toolbox"), nil
	}
	return filepath.Join(home, ".local", "share", "my-toolbox"), nil
}

// compareVersions compares two canonical three-part numeric versions.
//
// Args:
//   - candidate: Candidate release version without a leading v.
//   - current: Installed version without a leading v.
//
// Returns:
//   - int: -1 when candidate is older, 0 when equal, or 1 when newer.
//   - error: Invalid candidate or installed version syntax.
func compareVersions(candidate, current string) (int, error) {
	candidateParts, err := parseVersion(candidate)
	if err != nil {
		return 0, fmt.Errorf("invalid release version %q: %w", candidate, err)
	}
	currentParts, err := parseVersion(current)
	if err != nil {
		return 0, fmt.Errorf("invalid installed version %q: %w", current, err)
	}
	for index := range candidateParts {
		if candidateParts[index] < currentParts[index] {
			return -1, nil
		}
		if candidateParts[index] > currentParts[index] {
			return 1, nil
		}
	}
	return 0, nil
}

// parseVersion validates and parses a canonical three-part numeric version.
//
// Args:
//   - version: Version in major.minor.patch form with no leading zeroes.
//
// Returns:
//   - [3]int: Parsed major, minor, and patch components.
//   - error: Wrong component count, non-numeric content, or non-canonical zero padding.
func parseVersion(version string) ([3]int, error) {
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return [3]int{}, fmt.Errorf("expected three numeric components")
	}
	var values [3]int
	for index, part := range parts {
		if part == "" || (len(part) > 1 && part[0] == '0') {
			return [3]int{}, fmt.Errorf("component %q is not canonical", part)
		}
		value, err := strconv.Atoi(part)
		if err != nil || value < 0 {
			return [3]int{}, fmt.Errorf("component %q is not numeric", part)
		}
		values[index] = value
	}
	return values, nil
}
