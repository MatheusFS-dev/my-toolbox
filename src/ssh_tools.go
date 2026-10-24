package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

type sshToolsSession struct {
	root         string
	environment  string
	input        io.Reader
	reader       *bufio.Reader
	execRun      func(string, ...string) error
	execRunInput func(string, string, ...string) error
	runProgram   func(tea.Model) (tea.Model, error)
	output       io.Writer
	errorOutput  io.Writer
	messages     *toolOutput
	home         string
	successes    int
}

func (session *sshToolsSession) readLine() (string, error) {
	if session.reader == nil {
		session.reader = bufio.NewReader(session.input)
	}
	return session.reader.ReadString('\n')
}

// RunSSHTools opens the SSH dashboard and runs explicitly selected operations.
//
// Args:
//   - root: Installed toolbox payload root.
//   - environment: Detected native Linux or WSL environment.
//   - platform: Validated platform name.
//   - input: Interactive standard input.
//   - output: Status and external process standard output.
//   - errorOutput: External process standard error.
//
// Returns:
//   - error: Configuration, terminal, or command failure.
func RunSSHTools(root, environment, platform string, input io.Reader, output, errorOutput io.Writer) error {
	_ = platform
	if input == nil {
		input = os.Stdin
	}
	if output == nil {
		output = os.Stdout
	}
	if errorOutput == nil {
		errorOutput = os.Stderr
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	session := sshToolsSession{root: root, environment: environment, input: input, output: output, errorOutput: errorOutput, home: home, messages: newToolOutput(output, "ssh-tools", time.Now, detectToolColor(output))}
	return session.runSSHTools()
}

func (session *sshToolsSession) runSSHTools() error {
	for {
		availability := ""
		if session.environment == "linux-native" {
			availability = session.directAvailable()
		}
		model := newSSHLauncher(session.environment, availability, 80, 24)
		final, err := session.runUI(model)
		if err != nil {
			return err
		}
		selected, ok := final.(sshLauncher)
		if !ok {
			return fmt.Errorf("SSH launcher returned invalid state")
		}
		if selected.action == "" {
			if session.successes > 0 {
				return session.messages.Success(fmt.Sprintf("Completed %d SSH operation(s).", session.successes))
			}
			return session.messages.Info("No changes made.")
		}
		if selected.action == "profiles" {
			if err := session.runProfileDashboard(); err != nil {
				return err
			}
			continue
		}
		if err := session.performLauncherAction(selected.action); err != nil {
			if errors.Is(err, ErrCancelled) {
				_ = session.messages.Info("Cancelled; nothing changed.")
				continue
			}
			return err
		}
		session.successes++
	}
}

func (session *sshToolsSession) runUI(model tea.Model) (tea.Model, error) {
	if session.runProgram != nil {
		return session.runProgram(model)
	}
	return tea.NewProgram(model, tea.WithInput(session.input), tea.WithOutput(session.errorOutput)).Run()
}

func (session *sshToolsSession) runProfileDashboard() error {
	for {
		profiles, err := loadSSHProfiles(session.home)
		if err != nil {
			return err
		}
		model := newSSHDashboard(profiles, session.environment, 80, 24)
		final, err := session.runUI(model)
		if err != nil {
			return err
		}
		selected, ok := final.(sshDashboard)
		if !ok {
			return fmt.Errorf("SSH dashboard returned invalid state")
		}
		if selected.action == "" {
			return nil
		}
		if selected.action == "refresh" {
			continue
		}
		if err := session.perform(selected, profiles); err != nil {
			if errors.Is(err, ErrCancelled) {
				_ = session.messages.Info("Cancelled; nothing changed.")
				continue
			}
			return err
		}
		session.successes++
	}
}

func (session *sshToolsSession) prompt(label, defaultValue string, required bool) (string, error) {
	suffix := " (required)"
	if defaultValue != "" {
		suffix = " (default: " + defaultValue + ")"
	} else if !required {
		suffix = " (optional)"
	}
	for {
		if err := session.messages.Prompt(label + suffix + ": "); err != nil {
			return "", err
		}
		answer, err := session.readLine()
		if err != nil && err != io.EOF {
			return "", err
		}
		answer = strings.TrimSpace(answer)
		if answer == "" {
			answer = defaultValue
		}
		if answer != "" || !required {
			return answer, nil
		}
		if err := session.messages.Warn("A value is required."); err != nil {
			return "", err
		}
		if err == io.EOF {
			return "", io.EOF
		}
	}
}

func (session *sshToolsSession) confirm(label string, defaultYes bool) (bool, error) {
	prompt := " [y/N] (default: n)"
	if defaultYes {
		prompt = " [Y/n] (default: Y)"
	}
	for {
		if err := session.messages.Prompt(label + prompt); err != nil {
			return false, err
		}
		answer, err := session.readLine()
		if err != nil && err != io.EOF {
			return false, err
		}
		answer = strings.ToLower(strings.TrimSpace(answer))
		switch answer {
		case "":
			return defaultYes, nil
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		}
		if err := session.messages.Warn("Enter y or n."); err != nil {
			return false, err
		}
		if err == io.EOF {
			return false, io.EOF
		}
	}
}

func (session *sshToolsSession) run(command string, arguments ...string) error {
	if session.execRun != nil {
		return session.execRun(command, arguments...)
	}
	process := exec.Command(command, arguments...)
	process.Stdin = session.input
	process.Stdout = session.messages.External()
	process.Stderr = session.errorOutput
	if err := process.Run(); err != nil {
		return fmt.Errorf("%s failed: %w", command, err)
	}
	return nil
}

func (session *sshToolsSession) runInput(input, command string, arguments ...string) error {
	if session.execRunInput != nil {
		return session.execRunInput(input, command, arguments...)
	}
	process := exec.Command(command, arguments...)
	process.Stdin = strings.NewReader(input)
	process.Stdout = session.messages.External()
	process.Stderr = session.errorOutput
	if err := process.Run(); err != nil {
		return fmt.Errorf("%s failed: %w", command, err)
	}
	return nil
}

func (session *sshToolsSession) performLauncherAction(action string) error {
	switch action {
	case "direct-setup":
		return session.directSetup()
	case "direct-undo":
		return session.directUndo()
	case "monitor-install":
		return session.updateDirectMonitor("install-status")
	case "monitor-remove":
		return session.updateDirectMonitor("uninstall-status")
	}
	return fmt.Errorf("unknown SSH launcher action %q", action)
}

func (session *sshToolsSession) perform(selected sshDashboard, profiles []SSHProfile) error {
	switch selected.action {
	case "add", "edit":
		return session.configureProfile(selected, profiles)
	case "test":
		profile, ok := findSSHProfile(profiles, selected.alias)
		if !ok {
			return fmt.Errorf("profile %q not found", selected.alias)
		}
		if err := session.messages.Info("Testing passwordless SSH connection..."); err != nil {
			return err
		}
		return session.verify(profile)
	case "connect":
		if err := session.messages.Info("Opening SSH connection..."); err != nil {
			return err
		}
		return session.run("ssh", selected.alias)
	case "remove":
		confirmed, err := session.confirm("Remove profile "+selected.alias+"? Remote authorized keys will remain", false)
		if err != nil {
			return err
		}
		if !confirmed {
			return ErrCancelled
		}
		if err := session.messages.Info("Removing local SSH profile..."); err != nil {
			return err
		}
		if err := removeSSHProfile(session.home, selected.alias); err != nil {
			return err
		}
		return session.messages.Warn("Remote authorized keys were not revoked.")
	}
	return fmt.Errorf("unknown SSH action %q", selected.action)
}

func findSSHProfile(profiles []SSHProfile, alias string) (SSHProfile, bool) {
	for _, profile := range profiles {
		if profile.Alias == alias {
			return profile, true
		}
	}
	return SSHProfile{}, false
}

func (session *sshToolsSession) configureProfile(selected sshDashboard, profiles []SSHProfile) error {
	old, updating := findSSHProfile(profiles, selected.alias)
	aliasDefault := ""
	if updating {
		aliasDefault = old.Alias
	}
	alias, err := session.prompt("Alias", aliasDefault, true)
	if err != nil {
		return err
	}
	if updating && alias != old.Alias {
		return fmt.Errorf("profile alias cannot change during edit")
	}
	if !validSSHAlias(alias) {
		return fmt.Errorf("alias must use letters, digits, underscores, or hyphens")
	}
	if !updating {
		if _, exists := findSSHProfile(profiles, alias); exists {
			return fmt.Errorf("profile %q already exists", alias)
		}
	}
	hostDefault, userDefault, portDefault := "", "", "22"
	if updating {
		hostDefault, userDefault, portDefault = old.Hostname, old.User, strconv.Itoa(old.Port)
	}
	hostname, err := session.prompt("Hostname or IP", hostDefault, true)
	if err != nil {
		return err
	}
	user, err := session.prompt("SSH user", userDefault, true)
	if err != nil {
		return err
	}
	portText, err := session.prompt("SSH port", portDefault, true)
	if err != nil {
		return err
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return fmt.Errorf("invalid SSH port: %w", err)
	}
	_, _, _, keys := sshPaths(session.home)
	profile := SSHProfile{Alias: alias, Hostname: hostname, User: user, Port: port, IdentityPath: filepath.Join(keys, alias)}
	if err := validateSSHProfile(profile); err != nil {
		return err
	}
	if err := checkSSHProfileDestination(session.home, profile); err != nil {
		return err
	}
	if updating && old.Health != "saved" {
		return fmt.Errorf("key for %q is %s; remove and recreate the profile", alias, old.Health)
	}
	confirmed, err := session.confirm("Copy the public key to "+user+"@"+hostname+"?", false)
	if err != nil {
		return err
	}
	if !confirmed {
		return ErrCancelled
	}
	if err := ensureSSHDirectories(session.home); err != nil {
		return err
	}
	key := profile.IdentityPath
	if err := secureSSHPath(key, false); err != nil {
		return err
	}
	newKey := !updating
	if newKey {
		if _, err := os.Lstat(key); !os.IsNotExist(err) {
			return fmt.Errorf("key path already exists: %s", key)
		}
		if err := session.messages.Info("Generating a dedicated Ed25519 key..."); err != nil {
			return err
		}
		if err := session.run("ssh-keygen", "-t", "ed25519", "-N", "", "-f", key, "-C", "ssh-tools-"+alias); err != nil {
			return err
		}
		defer func() {
			if newKey {
				os.Remove(key)
				os.Remove(key + ".pub")
			}
		}()
	}
	if err := session.messages.Info("Copying the public key to the SSH host..."); err != nil {
		return err
	}
	if err := session.run("ssh-copy-id", "-i", key+".pub", "-p", strconv.Itoa(port), user+"@"+hostname); err != nil {
		return err
	}
	if err := session.messages.Info("Verifying passwordless SSH access..."); err != nil {
		return err
	}
	if err := session.verify(profile); err != nil {
		return err
	}
	if err := saveSSHProfile(session.home, profile); err != nil {
		return err
	}
	newKey = false
	if updating {
		return session.messages.Warn("The old remote authorized key was not revoked.")
	}
	return nil
}

func (session *sshToolsSession) verify(profile SSHProfile) error {
	return session.run("ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=5", "-i", profile.IdentityPath, "-p", strconv.Itoa(profile.Port), profile.User+"@"+profile.Hostname, "true")
}

func (session *sshToolsSession) helperPath() string {
	return filepath.Join(session.root, "packages", "ssh_tools", "direct_link.py")
}

func (session *sshToolsSession) directAvailable() string {
	for _, name := range []string{"python3", "sudo"} {
		if _, err := exec.LookPath(name); err != nil {
			return "Install " + name + " to manage direct Ethernet links."
		}
	}
	if _, err := os.Stat(session.helperPath()); err != nil {
		return "Direct-link helper is missing from this toolbox installation."
	}
	return ""
}

func (session *sshToolsSession) directSetup() error {
	if session.environment != "linux-native" {
		return fmt.Errorf("direct Ethernet links require native Linux")
	}
	if guidance := session.directAvailable(); guidance != "" {
		return fmt.Errorf("%s", guidance)
	}
	iface, err := session.prompt("Ethernet interface", "", true)
	if err != nil {
		return err
	}
	cidr, err := session.prompt("Link subnet", "172.30.77.0/24", true)
	if err != nil {
		return err
	}
	address, err := session.prompt("Local link address", "172.30.77.1", true)
	if err != nil {
		return err
	}
	config, err := json.Marshal(map[string]string{"interface": iface, "subnet": cidr, "address": address})
	if err != nil {
		return err
	}
	confirmed, err := session.confirm("Configure direct Ethernet on "+iface+"?", false)
	if err != nil {
		return err
	}
	if !confirmed {
		return ErrCancelled
	}
	if err := session.messages.Info("Saving rollback state and configuring direct Ethernet..."); err != nil {
		return err
	}
	if err := session.runInput(string(config), "sudo", "python3", session.helperPath(), "setup"); err != nil {
		return fmt.Errorf("direct-link setup failed: %w", err)
	}
	return nil
}

func (session *sshToolsSession) directUndo() error {
	if session.environment != "linux-native" {
		return fmt.Errorf("direct Ethernet links require native Linux")
	}
	if guidance := session.directAvailable(); guidance != "" {
		return fmt.Errorf("%s", guidance)
	}
	confirmed, err := session.confirm("Undo toolbox direct-link setup?", false)
	if err != nil {
		return err
	}
	if !confirmed {
		return ErrCancelled
	}
	if err := session.messages.Info("Restoring direct-link configuration..."); err != nil {
		return err
	}
	return session.run("sudo", "python3", session.helperPath(), "undo")
}

func (session *sshToolsSession) updateDirectMonitor(action string) error {
	if session.environment != "linux-native" {
		return fmt.Errorf("status monitor requires native Linux")
	}
	if guidance := session.directAvailable(); guidance != "" {
		return fmt.Errorf("%s", guidance)
	}
	label := map[string]string{"install-status": "Install", "uninstall-status": "Remove"}[action]
	if label == "" {
		return fmt.Errorf("unknown status monitor action %q", action)
	}
	confirmed, err := session.confirm(label+" desktop-login status monitor?", false)
	if err != nil {
		return err
	}
	if !confirmed {
		return ErrCancelled
	}
	if err := session.messages.Info("Updating direct-link status monitor..."); err != nil {
		return err
	}
	return session.run("sudo", "python3", session.helperPath(), action)
}

func runSSHStatusDisplay(root string, output, errorOutput io.Writer) int {
	command := exec.Command("python3", filepath.Join(root, "packages", "ssh_tools", "direct_link.py"), "status-display")
	command.Stdout, command.Stderr = output, errorOutput
	if command.Run() != nil {
		return 1
	}
	return 0
}
