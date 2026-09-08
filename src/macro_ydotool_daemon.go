package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func (workflow DefaultMacroWorkflow) ensureYdotoolDaemon(client, home string) ([]string, error) {
	environment := os.Environ()
	if probeYdotoolEnvironment(client, environment) == nil {
		return environment, nil
	}
	socket := os.Getenv("YDOTOOL_SOCKET")
	if socket == "" {
		socket = filepath.Join(home, ".ydotool_socket")
	}
	socket, err := filepath.Abs(socket)
	if err != nil {
		return nil, err
	}
	// Linux sockaddr_un reserves 108 bytes, including the terminating NUL.
	if len(socket) > 107 {
		return nil, fmt.Errorf("ydotool socket path is too long: %s", socket)
	}
	environment = append(environment, "YDOTOOL_SOCKET="+socket)
	if probeYdotoolEnvironment(client, environment) == nil {
		return environment, nil
	}
	// ydotoold removes stale sockets itself; never let it remove an unrelated file.
	if info, err := os.Lstat(socket); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return nil, fmt.Errorf("ydotool socket path is not a socket: %s", socket)
		}
		managedSocket, err := filepath.Abs(filepath.Join(home, ".ydotool_socket"))
		if err != nil {
			return nil, err
		}
		if socket != managedSocket {
			return nil, fmt.Errorf("configured ydotool socket is unavailable; refusing to replace an existing custom socket: %s", socket)
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(socket), 0o700); err != nil {
		return nil, err
	}
	daemon, err := exec.LookPath(filepath.Join(filepath.Dir(client), "ydotoold"))
	if err != nil {
		daemon, err = exec.LookPath("ydotoold")
	}
	if err != nil {
		return nil, fmt.Errorf("ydotoold is missing; install the ydotool daemon alongside %s: %w", client, err)
	}
	daemon, err = filepath.Abs(daemon)
	if err != nil {
		return nil, err
	}
	needsSudo := false
	if os.Geteuid() != 0 {
		device, err := os.OpenFile("/dev/uinput", os.O_WRONLY, 0)
		if err != nil {
			needsSudo = true
		} else {
			device.Close()
		}
	}
	_, _, _, _, output := workflow.settings()
	log, err := startYdotoolDaemon(daemon, socket, home, needsSudo, output)
	if err != nil {
		return nil, err
	}
	// v1.0.4 binds its socket before creating the input device, then waits one
	// second for device discovery. A successful socket connection alone is early.
	time.Sleep(1500 * time.Millisecond)
	deadline := time.Now().Add(5 * time.Second)
	for {
		err = probeYdotoolEnvironment(client, environment)
		if err == nil {
			return environment, nil
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil, fmt.Errorf("ydotoold did not become ready: %w; daemon log: %s", err, log)
}

func startYdotoolDaemon(daemon, socket, home string, needsSudo bool, output io.Writer) (string, error) {
	directory := filepath.Join(home, ".local", "state", "my-toolbox", "macros")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", err
	}
	log, err := os.CreateTemp(directory, "ydotoold-*.log")
	if err != nil {
		return "", err
	}
	defer log.Close()
	arguments := []string{"--socket-path=" + socket, "--socket-own=" + fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()), "--socket-perm=0600"}
	command := exec.Command(daemon, arguments...)
	if needsSudo {
		command = exec.Command("sudo", append([]string{"-n", "-b", "--", daemon}, arguments...)...)
	}
	command.Stdout, command.Stderr = log, log
	if needsSudo {
		// Try the command first: sudo -v can require a password even with NOPASSWD.
		err = command.Run()
		if err != nil {
			fmt.Fprintln(output, "Starting ydotoold requires access to /dev/uinput. sudo may ask for your password.")
			authenticate := exec.Command("sudo", "-v")
			authenticate.Stdin, authenticate.Stdout, authenticate.Stderr = os.Stdin, output, output
			if authErr := authenticate.Run(); authErr != nil {
				return "", fmt.Errorf("authorize ydotoold startup: %w", authErr)
			}
			command = exec.Command("sudo", append([]string{"-n", "-b", "--", daemon}, arguments...)...)
			command.Stdout, command.Stderr = log, log
			err = command.Run()
		}
	} else {
		configureNonInteractive(command)
		err = command.Start()
		if err == nil {
			go command.Wait()
		}
	}
	if err != nil {
		details, _ := os.ReadFile(log.Name())
		return "", fmt.Errorf("start ydotoold: %w: %s; daemon log: %s", err, strings.TrimSpace(string(details)), log.Name())
	}
	fmt.Fprintf(output, "Started ydotoold. Daemon log: %s\n", log.Name())
	return log.Name(), nil
}
