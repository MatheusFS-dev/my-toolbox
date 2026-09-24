//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMonitorUpdateNeededOnlyWhenInstalledVersionIsOlder(t *testing.T) {
	for _, tc := range []struct {
		name      string
		installed string
		bundled   string
		want      bool
		wantError bool
	}{
		{"older", "1.0.0", "1.1.0", true, false},
		{"equal", "1.1.0", "1.1.0", false, false},
		{"newer", "1.2.0", "1.1.0", false, false},
		{"absent", "", "1.1.0", false, false},
		{"invalid installed", "unknown", "1.1.0", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			activeRoot := t.TempDir()
			bundled := filepath.Join(activeRoot, "packages", "monitor_runtime", "monitor_runtime", "__init__.py")
			if err := os.MkdirAll(filepath.Dir(bundled), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(bundled, []byte("__version__ = \""+tc.bundled+"\"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if tc.installed != "" {
				runtimeRoot := filepath.Join(home, ".monitor", "runtime")
				installed := filepath.Join(runtimeRoot, "app", "monitor_runtime", "__init__.py")
				if err := os.MkdirAll(filepath.Dir(installed), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(runtimeRoot, "owned.json"), []byte(monitorRuntimeMarker), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(installed, []byte("__version__ = \""+tc.installed+"\"\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			got, err := monitorUpdateNeeded(activeRoot)
			if (err != nil) != tc.wantError || got != tc.want {
				t.Fatalf("monitorUpdateNeeded() = %t, %v; want %t, error=%t", got, err, tc.want, tc.wantError)
			}
		})
	}
}
