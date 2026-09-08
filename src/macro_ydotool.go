package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// The pinned source build avoids older distro packages with incompatible key syntax.
const ydotoolSourceURL = "https://codeload.github.com/ReimuNotMoe/ydotool/tar.gz/refs/tags/v1.0.4"
const ydotoolSourceDigest = "sha256:ba075a43aa6ead51940e892ecffa4d0b8b40c241e4e2bc4bd9bd26b61fde23bd"

func findYdotool(home string) (string, error) {
	candidates := []string{filepath.Join(home, ".local", "bin", "ydotool")}
	if path, err := exec.LookPath("ydotool"); err == nil {
		candidates = append(candidates, path)
	}
	for _, path := range candidates {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		output, err := exec.CommandContext(ctx, path, "--help").CombinedOutput()
		cancel()
		// 1.x exposes YDOTOOL_SOCKET in its top-level help; 0.1.x uses another protocol.
		if err == nil && strings.Contains(string(output), "YDOTOOL_SOCKET") {
			return path, nil
		}
	}
	return "", fmt.Errorf("ydotool 1.x is required (older 0.1.x packages are incompatible)")
}

func probeYdotool(path string) error {
	return probeYdotoolEnvironment(path, os.Environ())
}

func probeYdotoolEnvironment(path string, environment []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// With no keycodes, this connects to the daemon and prints help without sending input.
	command := exec.CommandContext(ctx, path, "key")
	command.Env = environment
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ydotool daemon is unavailable: %w: %s", err, strings.TrimSpace(string(output)))
	}
	if !strings.Contains(string(output), "keycode") {
		return fmt.Errorf("ydotool does not support the required keycode interface")
	}
	return nil
}

func (workflow DefaultMacroWorkflow) runYdotool(macro Macro) error {
	goos, goarch, home, _, output := workflow.settings()
	findRuntime := workflow.FindRuntime
	if findRuntime == nil {
		findRuntime = func(_ string, home string) (string, error) { return findYdotool(home) }
	}
	path, err := findRuntime(goos, home)
	if err != nil {
		answer, askErr := workflow.UI.Ask(Question{ID: "install-ydotool", Type: "confirm", Title: "ydotool 1.x is required. Build and install v1.0.4 in ~/.local/bin (requires CMake, Make, and a C compiler)?"})
		if askErr != nil {
			return askErr
		}
		confirmed, ok := answer.(bool)
		if !ok || !confirmed {
			return err
		}
		installRuntime := workflow.InstallRuntime
		if installRuntime == nil {
			installRuntime = func(_, _, home string) error { return workflow.installYdotool(home) }
		}
		if err := installRuntime(goos, goarch, home); err != nil {
			return err
		}
		path, err = findRuntime(goos, home)
		if err != nil {
			return fmt.Errorf("validate ydotool installation: %w", err)
		}
		fmt.Fprintln(output, "ydotool installed.")
	}
	arguments, err := workflow.macroArguments(macro)
	if err != nil {
		return err
	}
	shell, err := exec.LookPath("sh")
	if err != nil {
		return err
	}
	// ydotool macros implement --check so invalid input is rejected before detaching.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	check := exec.CommandContext(ctx, shell, append([]string{macro.SourcePath, "--check"}, arguments...)...)
	if details, err := check.CombinedOutput(); err != nil {
		return fmt.Errorf("validate macro: %w: %s", err, strings.TrimSpace(string(details)))
	}
	environment, err := workflow.ensureYdotoolDaemon(path, home)
	if err != nil {
		return err
	}
	directory := filepath.Join(home, ".local", "state", "my-toolbox", "macros")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	log, err := os.CreateTemp(directory, "ydotool-*.log")
	if err != nil {
		return err
	}
	defer log.Close()
	command := exec.Command(shell, append([]string{macro.SourcePath}, arguments...)...)
	command.Env = append(environment, "PATH="+filepath.Dir(path)+string(os.PathListSeparator)+os.Getenv("PATH"))
	command.Stdout, command.Stderr = log, log
	configureNonInteractive(command)
	if err := command.Start(); err != nil {
		os.Remove(log.Name())
		return fmt.Errorf("start macro: %w", err)
	}
	pid := command.Process.Pid
	go command.Wait()
	_, err = fmt.Fprintf(output, "Scheduled %s in the background (PID %d). Errors: %s\n", macro.Title, pid, log.Name())
	return err
}

func (workflow DefaultMacroWorkflow) installYdotool(home string) error {
	for _, name := range []string{"cmake", "make", "cc"} {
		if _, err := exec.LookPath(name); err != nil {
			return fmt.Errorf("building ydotool requires %s; install CMake, Make, and a C compiler using your package manager, then retry", name)
		}
	}
	_, _, _, client, output := workflow.settings()
	archive, cleanup, err := downloadTemporary(client, ydotoolSourceURL, ".tar.gz")
	if err != nil {
		return err
	}
	defer cleanup()
	if err := verifyDigest(archive, ydotoolSourceDigest, "ydotool v1.0.4"); err != nil {
		return err
	}
	directory, err := os.MkdirTemp("", "tb-ydotool-build-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(directory)
	if err := extractTarGzip(archive, directory); err != nil {
		return err
	}
	build := filepath.Join(directory, "build")
	for _, arguments := range [][]string{
		{"-S", filepath.Join(directory, "ydotool-1.0.4"), "-B", build, "-DCMAKE_BUILD_TYPE=Release"},
		{"--build", build, "--target", "ydotool", "ydotoold"},
	} {
		command := exec.Command("cmake", arguments...)
		command.Stdout, command.Stderr = output, output
		if err := command.Run(); err != nil {
			return fmt.Errorf("build ydotool: %w", err)
		}
	}
	bin := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		return err
	}
	for _, name := range []string{"ydotool", "ydotoold"} {
		content, err := os.ReadFile(filepath.Join(build, name))
		if err != nil {
			return err
		}
		file, err := os.CreateTemp(bin, ".ydotool-install-*")
		if err != nil {
			return err
		}
		temporary := file.Name()
		defer os.Remove(temporary)
		_, writeErr := file.Write(content)
		closeErr := file.Close()
		if writeErr != nil {
			return writeErr
		}
		if closeErr != nil {
			return closeErr
		}
		if err := os.Chmod(temporary, 0o755); err != nil {
			return err
		}
		if err := os.Rename(temporary, filepath.Join(bin, name)); err != nil {
			return err
		}
	}
	return nil
}
