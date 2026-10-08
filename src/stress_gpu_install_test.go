package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestStressGPUCatalogChecksVisibility verifies discovery through tb list.
// Args: t (*testing.T): Test context.
// Returns: None.
// Raises: None. Failures are reported through t.
func TestStressGPUCatalogChecksVisibility(t *testing.T) {
	catalog, err := LoadCatalogFile("../commands.json")
	if err != nil {
		t.Fatal(err)
	}
	command, exists := catalog.Find("install-stress-gpu")
	if !exists || command.Visibility != "list" || command.Category != "System Utilities" || command.Protocol != "builtin" {
		t.Fatalf("missing listed installer: %#v", command)
	}
	for _, environment := range []string{"linux-native", "linux-wsl", "windows"} {
		requirements, requirementErr := ResolveRequirements(command, environment)
		if requirementErr != nil || len(requirements) != 1 || requirements[0].ID != "python-stress-gpu" {
			t.Fatalf("missing Python requirement for %s: %v", environment, requirements)
		}
		if !command.SupportsEnvironment(environment) {
			t.Fatalf("unsupported environment %s", environment)
		}
	}
}

// stressGPUFixture creates an installer with a fake command runner.
// Args: t (*testing.T): Test context.
// Returns: *ToolboxBuiltins: Installer. string: State directory. string: Wrapper.
// Raises: None. Setup failures use t.Fatal.
func stressGPUFixture(t *testing.T) (*ToolboxBuiltins, string, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "local"))
	root := t.TempDir()
	source := filepath.Join(root, "packages", "stress_gpu_runtime")
	if err := os.MkdirAll(filepath.Join(source, "stress_gpu_runtime"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "requirements.txt"), []byte("nvidia-ml-py==13.590.48\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "stress_gpu_runtime", "__init__.py"), []byte("__version__ = '1.0.0'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	platform := "linux-amd64"
	if runtime.GOOS == "windows" {
		platform = "windows-amd64"
	}
	builtin := NewToolboxBuiltins(root, platform, "1.0.0", io.Discard)
	state, wrapper, err := stressGPUPaths(platform)
	if err != nil {
		t.Fatal(err)
	}
	return builtin, state, wrapper
}

// stressGPUFakeRun emulates venv creation and dependency installation.
// Args: path (string): Interpreter. arguments ([]string): Command arguments. output (io.Writer): Output.
// Returns: error: Filesystem errors.
// Raises: None.
func stressGPUFakeRun(path string, arguments []string, output io.Writer) error {
	for index, argument := range arguments {
		if argument == "venv" && index+1 < len(arguments) {
			root := arguments[index+1]
			sub, name := "bin", "python"
			if runtime.GOOS == "windows" {
				sub, name = "Scripts", "python.exe"
			}
			if err := os.MkdirAll(filepath.Join(root, sub), 0700); err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(root, sub, name), []byte("fake python"), 0700)
		}
	}
	return nil
}

// TestStressGPUInstallRepairAndRollback verifies generation activation.
// Args: t (*testing.T): Test context.
// Returns: None.
// Raises: None. Failures use t.Fatal.
func TestStressGPUInstallRepairAndRollback(t *testing.T) {
	builtin, state, wrapper := stressGPUFixture(t)
	if err := builtin.installStressGPUWith([]string{"fake-python"}, stressGPUFakeRun, os.Rename); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(filepath.Join(state, "current.txt"))
	if err != nil {
		t.Fatal(err)
	}
	wrapperContent, err := os.ReadFile(wrapper)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(wrapperContent), "stress_gpu_runtime") {
		t.Fatal("wrapper does not launch application")
	}
	if err := builtin.installStressGPUWith([]string{"fake-python"}, stressGPUFakeRun, os.Rename); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(filepath.Join(state, "current.txt"))
	if bytes.Equal(first, second) {
		t.Fatal("repair reused the old generation")
	}
	for _, failure := range []string{"dependency", "self-check", "activation"} {
		run := func(path string, arguments []string, output io.Writer) error {
			joined := strings.Join(arguments, " ")
			if failure == "dependency" && strings.Contains(joined, "pip install") {
				return fmt.Errorf("dependency failed")
			}
			if failure == "self-check" && strings.Contains(joined, "stress_gpu_runtime") {
				return fmt.Errorf("self-check failed")
			}
			return stressGPUFakeRun(path, arguments, output)
		}
		rename := func(source, destination string) error {
			if failure == "activation" && filepath.Base(destination) == "current.txt" {
				return fmt.Errorf("activation failed")
			}
			return os.Rename(source, destination)
		}
		if err := builtin.installStressGPUWith([]string{"fake-python"}, run, rename); err == nil {
			t.Fatalf("%s failure hidden", failure)
		}
		current, _ := os.ReadFile(filepath.Join(state, "current.txt"))
		content, _ := os.ReadFile(wrapper)
		if !bytes.Equal(second, current) || !bytes.Equal(wrapperContent, content) {
			t.Fatalf("%s damaged active installation", failure)
		}
	}
}

// TestStressGPUInstallRequiresNumPy verifies the packaged runtime installs
// NumPy before importing PyTorch's tensor conversion support.
// Args: t (*testing.T): Test context.
// Returns: None.
// Raises: None. Failures use t.Fatal.
func TestStressGPUInstallRequiresNumPy(t *testing.T) {
	builtin := NewToolboxBuiltins("..", "linux-amd64", "1.0.0", io.Discard)
	run := func(path string, arguments []string, output io.Writer) error {
		if strings.Contains(strings.Join(arguments, " "), "-m venv") {
			return stressGPUFakeRun(path, arguments, output)
		}
		for index, argument := range arguments {
			if argument == "-r" && index+1 < len(arguments) {
				content, err := os.ReadFile(arguments[index+1])
				if err != nil {
					return err
				}
				if !strings.Contains(string(content), "numpy") {
					return fmt.Errorf("NumPy is missing from packaged runtime requirements")
				}
			}
		}
		return nil
	}
	if err := builtin.installStressGPUWith([]string{"fake-python"}, run, os.Rename); err != nil {
		t.Fatal(err)
	}
}

// TestStressGPURefusesUnrelatedWrapperAndState checks ownership guards.
// Args: t (*testing.T): Test context.
// Returns: None.
// Raises: None. Failures use t.Fatal.
func TestStressGPURefusesUnrelatedWrapperAndState(t *testing.T) {
	for _, target := range []string{"wrapper", "state"} {
		t.Run(target, func(t *testing.T) {
			builtin, state, wrapper := stressGPUFixture(t)
			path := wrapper
			if target == "state" {
				path = filepath.Join(state, "owned.json")
			}
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("unrelated"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := builtin.installStressGPUWith([]string{"fake-python"}, stressGPUFakeRun, os.Rename); err == nil {
				t.Fatal("unrelated content replaced")
			}
			content, _ := os.ReadFile(path)
			if string(content) != "unrelated" {
				t.Fatal("ownership conflict changed content")
			}
		})
	}
}

// TestStressGPUMissingPythonHasActionableError verifies dependency diagnostics.
// Args: t (*testing.T): Test context.
// Returns: None.
// Raises: None. Failures use t.Fatal.
func TestStressGPUMissingPythonHasActionableError(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if _, err := stressGPUPython("linux-amd64"); err == nil || !strings.Contains(err.Error(), "3.10 through 3.14") {
		t.Fatalf("Python error: %v", err)
	}
	builtins := NewToolboxBuiltins(t.TempDir(), "linux-amd64", "1.0.0", io.Discard)
	if err := builtins.Run("install-stress-gpu", []string{"unexpected"}); err == nil || !strings.Contains(err.Error(), "does not accept arguments") {
		t.Fatalf("argument error: %v", err)
	}
}

// TestStressGPUPointerOwnershipAndWrapperPublicationFailure preserves state.
// Args: t (*testing.T): Test context.
// Returns: None.
// Raises: None. Failures use t.Fatal.
func TestStressGPUPointerOwnershipAndWrapperPublicationFailure(t *testing.T) {
	builtin, state, wrapper := stressGPUFixture(t)
	failure := func(source, destination string) error {
		if destination == wrapper {
			return fmt.Errorf("wrapper unavailable")
		}
		return os.Rename(source, destination)
	}
	if err := builtin.installStressGPUWith([]string{"fake-python"}, stressGPUFakeRun, failure); err == nil {
		t.Fatal("publication failure hidden")
	}
	if _, err := os.Stat(filepath.Join(state, "current.txt")); !os.IsNotExist(err) {
		t.Fatal("failed first install activated a runtime")
	}
	if err := builtin.installStressGPUWith([]string{"fake-python"}, stressGPUFakeRun, os.Rename); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(state, "current.txt"), []byte("../../unrelated\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := builtin.installStressGPUWith([]string{"fake-python"}, stressGPUFakeRun, os.Rename); err == nil {
		t.Fatal("unsafe pointer accepted")
	}
}

// TestStressGPURealVirtualEnvironmentRemainsAtCreationPath checks venv semantics.
// Args: t (*testing.T): Test context.
// Returns: None.
// Raises: None. Unsupported system Python is skipped explicitly.
func TestStressGPURealVirtualEnvironmentRemainsAtCreationPath(t *testing.T) {
	builtin, state, _ := stressGPUFixture(t)
	python, err := stressGPUPython(builtin.platform)
	if err != nil {
		t.Skip(err)
	}
	runner := func(path string, arguments []string, output io.Writer) error {
		if strings.Contains(strings.Join(arguments, " "), "-m venv") {
			return runStressGPUCommand(path, arguments, output)
		}
		if strings.Contains(strings.Join(arguments, " "), "stress_gpu_runtime") {
			return runStressGPUCommand(path, []string{"-c", "import sys, pathlib; assert pathlib.Path(sys.prefix, 'pyvenv.cfg').is_file()"}, output)
		}
		return nil
	}
	if err := builtin.installStressGPUWith(python, runner, os.Rename); err != nil {
		t.Fatal(err)
	}
	current, err := os.ReadFile(filepath.Join(state, "current.txt"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(state, strings.TrimSpace(string(current)), "venv", "pyvenv.cfg")
	if content, err := os.ReadFile(path); err != nil || !strings.Contains(string(content), "include-system-site-packages = false") {
		t.Fatalf("venv isolation: %v", err)
	}
}

// TestStressGPUCommandWaitHelper supplies a cancellable native subprocess.
// Args: t (*testing.T): Test context.
// Returns: None.
// Raises: None. Setup failures use t.Fatal.
func TestStressGPUCommandWaitHelper(t *testing.T) {
	if os.Getenv("STRESS_GPU_COMMAND_WAIT") != "1" {
		return
	}
	if err := os.WriteFile(os.Getenv("STRESS_GPU_COMMAND_READY"), []byte("ready"), 0600); err != nil {
		t.Fatal(err)
	}
	time.Sleep(30 * time.Second)
}

// TestStressGPUCommandCancellationWaitsForExit checks cancellation on every OS.
// Args: t (*testing.T): Test context.
// Returns: None.
// Raises: None. Failures use t.Fatal.
func TestStressGPUCommandCancellationWaitsForExit(t *testing.T) {
	ready := filepath.Join(t.TempDir(), "ready")
	t.Setenv("STRESS_GPU_COMMAND_WAIT", "1")
	t.Setenv("STRESS_GPU_COMMAND_READY", ready)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- runStressGPUCommandContext(ctx, os.Args[0], []string{"-test.run=^TestStressGPUCommandWaitHelper$"}, io.Discard)
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("dependency subprocess did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation result: %v", err)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("dependency subprocess survived cancellation")
	}
}

// TestStressGPULockHelper exits without defers to exercise OS lock release.
// Args: t (*testing.T): Test context.
// Returns: None.
// Raises: None. Child failures use t.Fatal.
func TestStressGPULockHelper(t *testing.T) {
	if os.Getenv("STRESS_GPU_LOCK_HELPER") != "1" {
		return
	}
	if _, err := acquireStressGPULock(os.Getenv("STRESS_GPU_LOCK_PATH")); err != nil {
		t.Fatal(err)
	}
	os.Exit(0)
}

// TestStressGPULockExcludesConcurrentInstallersAndSurvivesProcessExit checks locks.
// Args: t (*testing.T): Test context.
// Returns: None.
// Raises: None. Failures use t.Fatal.
func TestStressGPULockExcludesConcurrentInstallersAndSurvivesProcessExit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lock")
	release, err := acquireStressGPULock(path)
	if err != nil {
		t.Fatal(err)
	}
	if other, err := acquireStressGPULock(path); err == nil {
		other()
		t.Fatal("concurrent installation lock accepted")
	}
	release()
	command := exec.Command(os.Args[0], "-test.run=^TestStressGPULockHelper$")
	command.Env = append(os.Environ(), "STRESS_GPU_LOCK_HELPER=1", "STRESS_GPU_LOCK_PATH="+path)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("lock helper failed: %v %s", err, output)
	}
	release, err = acquireStressGPULock(path)
	if err != nil {
		t.Fatalf("dead process left a stale lock: %v", err)
	}
	release()
}
