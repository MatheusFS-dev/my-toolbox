package platform

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// CurrentPlatform returns the supported OS and architecture identifier.
func CurrentPlatform() (string, error) {
	key := runtime.GOOS + "-" + runtime.GOARCH
	for _, supported := range []string{"linux-amd64", "linux-arm64", "windows-amd64"} {
		if key == supported {
			return key, nil
		}
	}
	return "", fmt.Errorf("unsupported platform %s", key)
}

// CurrentEnvironment returns the supported operating environment.
func CurrentEnvironment() (string, error) {
	return DetectEnvironment(runtime.GOOS, "/proc/sys/kernel/osrelease")
}

// DetectEnvironment identifies native Linux, WSL, or Windows.
func DetectEnvironment(goos, osreleasePath string) (string, error) {
	switch goos {
	case "windows":
		return "windows", nil
	case "linux":
		content, err := os.ReadFile(osreleasePath)
		if err != nil {
			return "", fmt.Errorf("detect Linux environment from %s: %w", osreleasePath, err)
		}
		if strings.Contains(strings.ToLower(string(content)), "microsoft") {
			return "linux-wsl", nil
		}
		return "linux-native", nil
	default:
		return "", fmt.Errorf("unsupported operating system %s", goos)
	}
}

// ExecutableRoot returns the directory containing the resolved executable.
func ExecutableRoot() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve toolbox executable: %w", err)
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("resolve toolbox executable links: %w", err)
	}
	return filepath.Dir(path), nil
}
