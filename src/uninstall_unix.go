//go:build !windows

package main

import (
	"fmt"
	"os"
)

// uninstallPlatform synchronously removes Linux toolbox-managed paths.
//
// Args:
//   - dataRoot: Validated toolbox data directory containing every version.
//   - wrapper: Stable user-owned tb wrapper path.
//
// Returns:
//   - string: Empty because synchronous Linux cleanup needs no status file.
//   - error: Filesystem removal failure.
func uninstallPlatform(dataRoot, wrapper string) (string, error) {
	if err := os.RemoveAll(dataRoot); err != nil {
		return "", fmt.Errorf("remove toolbox data directory: %w", err)
	}
	ownedWrapper, err := isOwnedToolboxWrapper(wrapper, linuxToolboxWrapper)
	if err != nil {
		return "", err
	}
	if ownedWrapper {
		if err := os.Remove(wrapper); err != nil && !os.IsNotExist(err) {
			return "", fmt.Errorf("remove toolbox wrapper: %w", err)
		}
	}
	return "", nil
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
