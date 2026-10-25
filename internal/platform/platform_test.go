package platform

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectEnvironmentDistinguishesNativeLinuxWSLAndWindows(t *testing.T) {
	root := t.TempDir()
	osrelease := filepath.Join(root, "osrelease")
	if err := os.WriteFile(osrelease, []byte("6.8.0-generic\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	environment, err := DetectEnvironment("linux", osrelease)
	if err != nil || environment != "linux-native" {
		t.Fatalf("DetectEnvironment(native) = %q, %v", environment, err)
	}
	if err := os.WriteFile(osrelease, []byte("5.15.153.1-microsoft-standard-WSL2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	environment, err = DetectEnvironment("linux", osrelease)
	if err != nil || environment != "linux-wsl" {
		t.Fatalf("DetectEnvironment(WSL) = %q, %v", environment, err)
	}
	environment, err = DetectEnvironment("windows", osrelease)
	if err != nil || environment != "windows" {
		t.Fatalf("DetectEnvironment(Windows) = %q, %v", environment, err)
	}
	if _, err := DetectEnvironment("linux", filepath.Join(root, "missing")); err == nil || !strings.Contains(err.Error(), "detect Linux environment") {
		t.Fatalf("DetectEnvironment(missing) error = %v", err)
	}
}
