//go:build !windows

package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestStressGPUInstallerInterruptHelper runs the production installer in a child.
// Args: t (*testing.T): Test context.
// Returns: None.
// Raises: None. Failures use t.Fatal.
func TestStressGPUInstallerInterruptHelper(t *testing.T) {
	if os.Getenv("STRESS_GPU_TEST_HELPER") != "1" {
		return
	}
	root := os.Getenv("STRESS_GPU_TEST_ROOT")
	builtin := NewToolboxBuiltins(root, "linux-amd64", "1.0.0", io.Discard)
	err := builtin.installStressGPU()
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("interrupted install: %v", err)
	}
	state, _, _ := stressGPUPaths("linux-amd64")
	if _, err := os.Stat(filepath.Join(state, ".install-lock")); !os.IsNotExist(err) {
		t.Fatal("installation lock survived cancellation")
	}
	generations, _ := filepath.Glob(filepath.Join(state, "runtime-*"))
	if len(generations) != 0 {
		t.Fatal("cancelled runtime stage survived")
	}
	os.Setenv("STRESS_GPU_FAKE_WAIT", "0")
	if err := builtin.installStressGPU(); err != nil {
		t.Fatalf("repair after cancellation: %v", err)
	}
}

// TestStressGPUInstallerInterruptCancelsDetachedChildAndPermitsRetry verifies Ctrl+C.
// Args: t (*testing.T): Test context.
// Returns: None.
// Raises: None. Failures use t.Fatal.
func TestStressGPUInstallerInterruptCancelsDetachedChildAndPermitsRetry(t *testing.T) {
	home, root := t.TempDir(), t.TempDir()
	bin := filepath.Join(root, "bin")
	source := filepath.Join(root, "packages", "stress_gpu_runtime", "stress_gpu_runtime")
	for _, folder := range []string{bin, source} {
		if err := os.MkdirAll(folder, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(source, "__init__.py"), []byte("VERSION = 1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(source), "requirements.txt"), []byte("fake dependency\n"), 0600); err != nil {
		t.Fatal(err)
	}
	fake := "#!/bin/sh\nif [ \"$1\" = -c ] || [ \"$1\" = -s ]; then exit 0; fi\nif [ \"$1 $2\" = '-m venv' ]; then mkdir -p \"$3/bin\"; cp \"$0\" \"$3/bin/python\"; exit 0; fi\nif [ \"$1 $2 $3\" = '-m pip install' ]; then\n if [ \"$STRESS_GPU_FAKE_WAIT\" = 1 ]; then echo \"$$\" > \"$STRESS_GPU_TEST_READY\"; exec sleep 30; fi\n exit 0\nfi\nexit 1\n"
	if err := os.WriteFile(filepath.Join(bin, "python3"), []byte(fake), 0700); err != nil {
		t.Fatal(err)
	}
	ready := filepath.Join(root, "ready")
	command := exec.Command(os.Args[0], "-test.run=^TestStressGPUInstallerInterruptHelper$", "-test.v")
	command.Env = append(os.Environ(), "HOME="+home, "PATH="+bin+":/usr/bin:/bin", "STRESS_GPU_TEST_HELPER=1", "STRESS_GPU_TEST_ROOT="+root, "STRESS_GPU_TEST_READY="+ready, "STRESS_GPU_FAKE_WAIT=1")
	var output bytes.Buffer
	command.Stdout, command.Stderr = &output, &output
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer command.Process.Kill()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("fake pip did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	pidText, err := os.ReadFile(ready)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(pidText)))
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.Kill(pid, syscall.SIGKILL)
	if err := command.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("helper failed: %v\n%s", err, output.String())
		}
	case <-time.After(8 * time.Second):
		t.Fatal("interrupted installer did not exit")
	}
	if err := syscall.Kill(pid, 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("detached dependency process is still alive: %d, %v", pid, err)
	}
}
