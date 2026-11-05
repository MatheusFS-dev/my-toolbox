//go:build !windows

package builtins

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/MatheusFS-dev/my-toolbox/internal/output"
)

func TestMonitorDetectsGPUAssignmentInLocalPackageWithoutImporting(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 unavailable")
	}
	root := t.TempDir()
	for path, content := range map[string]string{
		"pyproject.toml":         "",
		"scripts/train.py":       "from sample.training import run\n",
		"src/sample/__init__.py": "import os\nos.environ['CUDA_VISIBLE_DEVICES'] = '0'\nraise RuntimeError('must not execute')\n",
		"src/sample/training.py": "def run(): pass\n",
	} {
		destination := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(destination, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	settings, err := detectMonitorGPUSettings([]string{filepath.Join(root, "scripts/train.py")}, python)
	if err != nil {
		t.Fatal(err)
	}
	if len(settings) != 1 || settings[0].Value != "0" || settings[0].Line != 2 || !strings.HasSuffix(settings[0].Path, "sample/__init__.py") {
		t.Fatalf("settings = %#v", settings)
	}
}

func TestMonitorGPUDetectionIgnoresCommentsStringsAndDefaults(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 unavailable")
	}
	path := filepath.Join(t.TempDir(), "job.py")
	source := "import os\n# os.environ['CUDA_VISIBLE_DEVICES'] = '1'\ntext = \"os.environ['CUDA_VISIBLE_DEVICES'] = '1'\"\nos.environ.setdefault('CUDA_VISIBLE_DEVICES', '0')\n"
	if err := os.WriteFile(path, []byte(source), 0644); err != nil {
		t.Fatal(err)
	}
	settings, err := detectMonitorGPUSettings([]string{path}, python)
	if err != nil || len(settings) != 0 {
		t.Fatalf("settings = %#v, error = %v", settings, err)
	}
}

func TestMonitorGPUConflictRequiresExplicitConfirmationAndRedWarning(t *testing.T) {
	settings := []monitorGPUSetting{{Path: "job.py", Line: 2, Value: "0"}}
	devices := []monitorLaunchGPU{{Index: 0, UUID: "GPU-first", Name: "RTX 3060"}, {Index: 1, UUID: "GPU-second", Name: "RTX 4070"}}
	buffer := &bytes.Buffer{}
	messages := newToolOutput(buffer, "monitor", nil, output.ColorModeBasic)
	calls := 0
	accepted, err := confirmMonitorGPUSelection(settings, "GPU-second", devices, messages, func(field huh.Field) error {
		calls++
		if field.GetKey() != "gpu-conflict" {
			t.Fatalf("field = %s", field.GetKey())
		}
		return nil // Keep the default No.
	})
	if err != nil || accepted || calls != 1 {
		t.Fatalf("accepted=%v calls=%d err=%v", accepted, calls, err)
	}
	if !strings.Contains(buffer.String(), "\x1b[31m") || !strings.Contains(buffer.String(), "job.py:2") || !strings.Contains(buffer.String(), "override") {
		t.Fatalf("warning = %q", buffer.String())
	}
	for _, selected := range []string{"", "GPU-first"} {
		accepted, err = confirmMonitorGPUSelection(settings, selected, devices, messages, func(huh.Field) error { t.Fatal("unexpected confirmation"); return nil })
		if err != nil || !accepted {
			t.Fatalf("selection %q rejected: %v", selected, err)
		}
	}
	accepted, err = confirmMonitorGPUSelection(settings, "GPU-second", devices, messages, func(field huh.Field) error {
		if field.GetValue() != false {
			t.Fatal("confirmation must default to No")
		}
		field.(*huh.Confirm).WithKeyMap(huh.NewDefaultKeyMap())
		_, _ = field.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
		return nil
	})
	if err != nil || !accepted {
		t.Fatalf("explicit Yes rejected: %v %v", accepted, err)
	}
}

func TestMonitorGPUDetectionHandlesDynamicAndUpdateSettings(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 unavailable")
	}
	path := filepath.Join(t.TempDir(), "job.py")
	source := "import os\nGPU = '1'\nos.environ.update(CUDA_VISIBLE_DEVICES=GPU)\nos.putenv('CUDA_VISIBLE_DEVICES', choose_gpu())\n"
	if err := os.WriteFile(path, []byte(source), 0644); err != nil {
		t.Fatal(err)
	}
	settings, err := detectMonitorGPUSettings([]string{path}, python)
	if err != nil || len(settings) != 2 || settings[0].Value != "1" || settings[1].Value != "<dynamic>" {
		t.Fatalf("settings = %#v, error = %v", settings, err)
	}
	calls := 0
	accepted, err := confirmMonitorGPUSelection(settings, "GPU-any", nil, newToolOutput(&bytes.Buffer{}, "monitor", nil, output.ColorModeNever), func(huh.Field) error { calls++; return nil })
	if err != nil || accepted || calls != 1 {
		t.Fatalf("dynamic choice not confirmed: %v %v %d", accepted, err, calls)
	}
}
