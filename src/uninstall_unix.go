//go:build !windows

package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// uninstallPlatform synchronously removes Linux toolbox-managed paths.
//
// Args:
//   - dataRoot: Validated toolbox data directory containing every version.
//   - wrapper: Stable user-owned tb wrapper path.
//   - installerPath: Empty for uninstall only, or a downloaded shell installer
//     to execute after removal.
//   - output: Installer output destination.
//
// Returns:
//   - string: Empty because synchronous Linux cleanup needs no status file.
//   - error: Filesystem removal failure.
func uninstallPlatform(dataRoot, wrapper, installerPath string, output io.Writer) (string, error) {
	ownedWrapper, err := isOwnedToolboxWrapper(wrapper, linuxToolboxWrapper)
	if err != nil {
		return "", err
	}
	if !ownedWrapper && installerPath != "" {
		if _, err := os.Stat(wrapper); err == nil {
			return "", fmt.Errorf("refusing to update with unrecognized wrapper %s", wrapper)
		} else if !os.IsNotExist(err) {
			return "", fmt.Errorf("inspect toolbox wrapper: %w", err)
		}
	}
	if err := removeUnixCompletionProfiles(dataRoot); err != nil {
		return "", err
	}
	if err := os.RemoveAll(dataRoot); err != nil {
		return "", fmt.Errorf("remove toolbox data directory: %w", err)
	}
	if ownedWrapper {
		if err := os.Remove(wrapper); err != nil && !os.IsNotExist(err) {
			return "", fmt.Errorf("remove toolbox wrapper: %w", err)
		}
	}
	if installerPath != "" {
		if err := runClosedInput("bash", []string{installerPath}, nil, output); err != nil {
			return "", fmt.Errorf("reinstall toolbox: %w", err)
		}
	}
	return "", nil
}

// removeUnixCompletionProfiles removes exact Bash and Zsh activation blocks.
//
// Args:
//   - dataRoot: Stable toolbox data root referenced by both managed source lines.
//
// Returns:
//   - error: Home resolution, malformed-marker, profile write, or rollback failure.
func removeUnixCompletionProfiles(dataRoot string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve user home for completion cleanup: %w", err)
	}
	zshRoot := os.Getenv("ZDOTDIR")
	if zshRoot == "" {
		zshRoot = home
	}
	quotedCompletionRoot := strings.ReplaceAll(filepath.Join(dataRoot, "completions"), "'", "'\\''")
	return removeCompletionProfileBlocks([]completionProfileRequest{
		{
			path:       filepath.Join(home, ".bashrc"),
			sourceLine: ". '" + filepath.ToSlash(filepath.Join(quotedCompletionRoot, "tb.bash")) + "'",
			newline:    "\n",
		},
		{
			path:       filepath.Join(zshRoot, ".zshrc"),
			sourceLine: "source '" + filepath.ToSlash(filepath.Join(quotedCompletionRoot, "_tb")) + "'",
			newline:    "\n",
		},
	})
}

// runPlatformCleanup reports that Linux has no detached cleanup mode.
//
// Args: None.
//
// Returns:
//   - bool: Always false so normal command parsing continues.
//   - error: Always nil.
func runPlatformCleanup() (bool, error) {
	return false, nil
}
