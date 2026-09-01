package main

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ToolboxBuiltins implements official user-owned installers and self-update.
type ToolboxBuiltins struct {
	root     string
	platform string
	version  string
	output   io.Writer
	client   *http.Client
}

// NewToolboxBuiltins constructs the production builtin executor.
//
// Args:
//   - root: Installed version payload root.
//   - platform: Validated catalog platform key.
//   - version: Current toolbox version.
//   - output: Status destination.
//
// Returns:
//   - *ToolboxBuiltins: Ready builtin executor.
func NewToolboxBuiltins(root, platform, version string, output io.Writer) *ToolboxBuiltins {
	return &ToolboxBuiltins{
		root:     root,
		platform: platform,
		version:  version,
		output:   output,
		client:   &http.Client{Timeout: 2 * time.Minute},
	}
}

// SkipReason reports installed tools or plugins without changing state.
//
// Args:
//   - name: Builtin catalog command name.
//
// Returns:
//   - string: User-facing skip reason, or empty when execution is required.
//   - error: Plugin-management detection failure.
func (builtins *ToolboxBuiltins) SkipReason(name string) (string, error) {
	binaries := map[string]string{
		"install-codex":       "codex",
		"install-claude":      "claude",
		"install-antigravity": "agy",
		"install-uv":          "uv",
		"install-gh":          "gh",
	}
	if binary, exists := binaries[name]; exists {
		if path, err := resolveUserCommand(binary, builtins.platform); err == nil {
			return fmt.Sprintf("%s is already installed at %s", binary, path), nil
		}
		return "", nil
	}
	agents := map[string]string{
		"install-superpowers-codex":       "codex",
		"install-superpowers-claude":      "claude",
		"install-superpowers-antigravity": "agy",
	}
	agent, exists := agents[name]
	if !exists {
		return "", nil
	}
	path, err := resolveUserCommand(agent, builtins.platform)
	if err != nil {
		// A preceding selected installer may create the agent before execution.
		// Direct plugin commands still fail explicitly at run time if it remains absent.
		return "", nil
	}
	output, err := exec.Command(path, "plugin", "list").CombinedOutput()
	if err != nil {
		// Requirement validation belongs to the just-in-time execution preflight.
		// Configuration must remain valid when an earlier selected installer can
		// replace or update the agent before this tool runs.
		return "", nil
	}
	if strings.Contains(strings.ToLower(string(output)), "superpowers") {
		return fmt.Sprintf("Superpowers is already installed for %s", agent), nil
	}
	return "", nil
}

// Run executes one builtin command with no unmanaged terminal input.
//
// Args:
//   - name: Builtin command name, including update.
//   - arguments: Direct arguments; builtin commands currently accept none.
//
// Returns:
//   - error: Unsupported arguments, download, verification, or process failure.
func (builtins *ToolboxBuiltins) Run(name string, arguments []string) error {
	if len(arguments) != 0 {
		return fmt.Errorf("%s does not accept arguments", name)
	}
	switch name {
	case "install-codex":
		return builtins.runOfficialInstaller("https://chatgpt.com/codex/install.sh", "https://chatgpt.com/codex/install.ps1", nil)
	case "install-claude":
		return builtins.runOfficialInstaller("https://claude.ai/install.sh", "https://claude.ai/install.ps1", nil)
	case "install-antigravity":
		return builtins.runOfficialInstaller("https://antigravity.google/cli/install.sh", "https://antigravity.google/cli/install.ps1", nil)
	case "install-uv":
		return builtins.runOfficialInstaller("https://astral.sh/uv/install.sh", "https://astral.sh/uv/install.ps1", []string{"UV_NO_MODIFY_PATH=1"})
	case "install-gh":
		return builtins.installGH()
	case "install-superpowers-codex":
		return runAgentPlugin("codex", []string{"plugin", "add", "superpowers@openai-curated"}, builtins.platform, builtins.output)
	case "install-superpowers-claude":
		return runAgentPlugin("claude", []string{"plugin", "install", "superpowers@claude-plugins-official", "--scope", "user"}, builtins.platform, builtins.output)
	case "install-superpowers-antigravity":
		return runAgentPlugin("agy", []string{"plugin", "install", "https://github.com/obra/superpowers"}, builtins.platform, builtins.output)
	case "update":
		return builtins.update()
	case "uninstall":
		return builtins.uninstall()
	default:
		return fmt.Errorf("unsupported builtin command %q", name)
	}
}

func (builtins *ToolboxBuiltins) runOfficialInstaller(linuxURL, windowsURL string, environment []string) error {
	url := linuxURL
	extension := ".sh"
	if builtins.platform == "windows-amd64" {
		url = windowsURL
		extension = ".ps1"
	}
	script, err := builtins.download(url)
	if err != nil {
		return err
	}
	path, err := writeTemporaryInstaller(script, extension)
	if err != nil {
		return err
	}
	defer os.Remove(path)
	if builtins.platform == "windows-amd64" {
		powershell, exists := supportedPowerShellPath()
		if !exists {
			return fmt.Errorf("required Windows PowerShell 5.1 or PowerShell 7 is unavailable")
		}
		return runClosedPath(powershell, "PowerShell", []string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", path}, environment, builtins.output)
	}
	return runClosedInput("bash", []string{path}, environment, builtins.output)
}

// writeTemporaryInstaller writes a downloaded installer outside the managed toolbox root.
//
// Args:
//   - script: Complete installer bytes downloaded before any installed files are removed.
//   - extension: Platform script extension, either .sh or .ps1.
//
// Returns:
//   - string: Temporary installer path. The caller owns its removal.
//   - error: Temporary-file creation, write, or close failure.
func writeTemporaryInstaller(script []byte, extension string) (string, error) {
	temporaryFile, err := os.CreateTemp("", "toolbox-installer-*"+extension)
	if err != nil {
		return "", fmt.Errorf("create installer file: %w", err)
	}
	path := temporaryFile.Name()
	if _, err := temporaryFile.Write(script); err != nil {
		temporaryFile.Close()
		_ = os.Remove(path)
		return "", fmt.Errorf("write installer file: %w", err)
	}
	if err := temporaryFile.Close(); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("close installer file: %w", err)
	}
	return path, nil
}

func runClosedInput(name string, arguments, environment []string, output io.Writer) error {
	path, err := exec.LookPath(name)
	if err != nil {
		return fmt.Errorf("required command %s is unavailable", name)
	}
	return runClosedPath(path, name, arguments, environment, output)
}

func runAgentPlugin(name string, arguments []string, platform string, output io.Writer) error {
	path, err := resolveUserCommand(name, platform)
	if err != nil {
		return fmt.Errorf("required command %s is unavailable", name)
	}
	return runClosedPath(path, name, arguments, nil, output)
}

func runClosedPath(path, displayName string, arguments, environment []string, output io.Writer) error {
	command := exec.Command(path, arguments...)
	configureNonInteractive(command)
	command.Env = append(os.Environ(), environment...)
	command.Stdin = strings.NewReader("")
	command.Stdout = output
	command.Stderr = output
	if err := command.Run(); err != nil {
		return fmt.Errorf("%s failed: %w", displayName, err)
	}
	return nil
}

func resolveUserCommand(name, platform string) (string, error) {
	if path, err := exec.LookPath(name); err == nil {
		return path, nil
	}
	candidates := []string{}
	if platform == "windows-amd64" {
		localAppData := os.Getenv("LOCALAPPDATA")
		userProfile := os.Getenv("USERPROFILE")
		switch name {
		case "codex":
			if localAppData != "" {
				candidates = append(candidates, filepath.Join(localAppData, "Programs", "OpenAI", "Codex", "bin", "codex.exe"))
			}
		case "agy":
			if localAppData != "" {
				candidates = append(candidates, filepath.Join(localAppData, "agy", "bin", "agy.exe"))
			}
		case "gh":
			if localAppData != "" {
				candidates = append(candidates, filepath.Join(localAppData, "my-toolbox", "bin", "gh.exe"))
			}
		default:
			if userProfile != "" {
				candidates = append(candidates, filepath.Join(userProfile, ".local", "bin", name+".exe"))
			}
		}
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		candidates = append(candidates, filepath.Join(home, ".local", "bin", name))
	}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		info, err := os.Stat(candidate)
		if err == nil && info.Mode().IsRegular() && (platform == "windows-amd64" || info.Mode().Perm()&0o111 != 0) {
			return candidate, nil
		}
	}
	return "", exec.ErrNotFound
}

func (builtins *ToolboxBuiltins) download(url string) ([]byte, error) {
	response, err := builtins.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", url, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: HTTP %s", url, response.Status)
	}
	content, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", url, err)
	}
	return content, nil
}

func (builtins *ToolboxBuiltins) installGH() error {
	var release struct {
		TagName string `json:"tag_name"`
	}
	metadata, err := builtins.download("https://api.github.com/repos/cli/cli/releases/latest")
	if err != nil {
		return err
	}
	if err := json.Unmarshal(metadata, &release); err != nil {
		return fmt.Errorf("decode latest GitHub CLI release: %w", err)
	}
	if release.TagName == "" {
		return fmt.Errorf("latest GitHub CLI release is missing tag_name")
	}
	version := strings.TrimPrefix(release.TagName, "v")
	var archive string
	switch builtins.platform {
	case "linux-amd64":
		archive = fmt.Sprintf("gh_%s_linux_amd64.tar.gz", version)
	case "linux-arm64":
		archive = fmt.Sprintf("gh_%s_linux_arm64.tar.gz", version)
	case "windows-amd64":
		archive = fmt.Sprintf("gh_%s_windows_amd64.zip", version)
	default:
		return fmt.Errorf("unsupported GitHub CLI platform %s", builtins.platform)
	}
	baseURL := fmt.Sprintf("https://github.com/cli/cli/releases/download/%s", release.TagName)
	archiveBytes, err := builtins.download(baseURL + "/" + archive)
	if err != nil {
		return err
	}
	checksums, err := builtins.download(baseURL + "/gh_" + version + "_checksums.txt")
	if err != nil {
		return err
	}
	expected, err := checksumFor(checksums, archive)
	if err != nil {
		return err
	}
	if err := verifyChecksum(archiveBytes, expected); err != nil {
		return fmt.Errorf("verify %s: %w", archive, err)
	}
	destination, err := ghDestination(builtins.platform)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return fmt.Errorf("create GitHub CLI directory: %w", err)
	}
	if strings.HasSuffix(archive, ".zip") {
		err = extractZipBinary(archiveBytes, "bin/gh.exe", destination)
	} else {
		err = extractTarBinary(archiveBytes, "bin/gh", destination)
	}
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(builtins.output, "Installed gh to %s\n", destination)
	if err == nil && !directoryOnPath(filepath.Dir(destination), builtins.platform == "windows-amd64") {
		_, err = fmt.Fprintf(builtins.output, "Add %s to PATH to run gh.\n", filepath.Dir(destination))
	}
	return err
}

func directoryOnPath(directory string, caseInsensitive bool) bool {
	for _, candidate := range filepath.SplitList(os.Getenv("PATH")) {
		if caseInsensitive {
			if strings.EqualFold(filepath.Clean(candidate), filepath.Clean(directory)) {
				return true
			}
		} else if filepath.Clean(candidate) == filepath.Clean(directory) {
			return true
		}
	}
	return false
}

func checksumFor(content []byte, filename string) (string, error) {
	scanner := bufio.NewScanner(bytes.NewReader(content))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == filename {
			if _, err := hex.DecodeString(fields[0]); err != nil || len(fields[0]) != sha256.Size*2 {
				return "", fmt.Errorf("invalid SHA-256 for %s", filename)
			}
			return strings.ToLower(fields[0]), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read checksums: %w", err)
	}
	return "", fmt.Errorf("checksum for %s is missing", filename)
}

func verifyChecksum(content []byte, expected string) error {
	actual := sha256.Sum256(content)
	if hex.EncodeToString(actual[:]) != strings.ToLower(expected) {
		return fmt.Errorf("SHA-256 mismatch")
	}
	return nil
}

func ghDestination(platform string) (string, error) {
	if platform == "windows-amd64" {
		root := os.Getenv("LOCALAPPDATA")
		if root == "" {
			return "", fmt.Errorf("LOCALAPPDATA is not set")
		}
		return filepath.Join(root, "my-toolbox", "bin", "gh.exe"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home: %w", err)
	}
	return filepath.Join(home, ".local", "bin", "gh"), nil
}

func extractTarBinary(content []byte, suffix, destination string) error {
	gzipReader, err := gzip.NewReader(bytes.NewReader(content))
	if err != nil {
		return fmt.Errorf("open tar archive: %w", err)
	}
	defer gzipReader.Close()
	reader := tar.NewReader(gzipReader)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar archive: %w", err)
		}
		if header.Typeflag == tar.TypeReg && strings.HasSuffix(filepath.ToSlash(header.Name), suffix) {
			return writeExecutable(destination, reader)
		}
	}
	return fmt.Errorf("archive does not contain %s", suffix)
}

func extractZipBinary(content []byte, suffix, destination string) error {
	reader, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return fmt.Errorf("open zip archive: %w", err)
	}
	for _, file := range reader.File {
		if !strings.HasSuffix(filepath.ToSlash(file.Name), suffix) {
			continue
		}
		source, err := file.Open()
		if err != nil {
			return fmt.Errorf("open %s: %w", file.Name, err)
		}
		err = writeExecutable(destination, source)
		source.Close()
		return err
	}
	return fmt.Errorf("archive does not contain %s", suffix)
}

func writeExecutable(destination string, source io.Reader) error {
	temporary, err := os.CreateTemp(filepath.Dir(destination), "gh-*")
	if err != nil {
		return fmt.Errorf("create temporary executable: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := io.Copy(temporary, source); err != nil {
		temporary.Close()
		return fmt.Errorf("write executable: %w", err)
	}
	if err := temporary.Chmod(0o755); err != nil {
		temporary.Close()
		return fmt.Errorf("mark executable: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close executable: %w", err)
	}
	if err := os.Rename(temporaryPath, destination); err != nil {
		return fmt.Errorf("install executable: %w", err)
	}
	return nil
}
