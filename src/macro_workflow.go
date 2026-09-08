package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type macroHTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

// DefaultMacroWorkflow runs, installs dependencies for, or downloads macros.
type DefaultMacroWorkflow struct {
	UI             UI
	Output         io.Writer
	GOOS           string
	GOARCH         string
	Home           string
	HTTP           macroHTTPClient
	FindRuntime    func(goos string, home string) (string, error)
	InstallRuntime func(goos string, goarch string, home string) error
	StartDetached  func(runtimePath string, scriptPath string, arguments []string) (int, error)
}

// Execute performs the selected macro action.
func (workflow DefaultMacroWorkflow) Execute(selection MacroSelection) error {
	if selection.Action == MacroActionDownload {
		return workflow.download(selection.Macro)
	}
	return workflow.run(selection.Macro)
}

func (workflow DefaultMacroWorkflow) settings() (string, string, string, macroHTTPClient, io.Writer) {
	goos, goarch, home, client, output := workflow.GOOS, workflow.GOARCH, workflow.Home, workflow.HTTP, workflow.Output
	if goos == "" {
		goos = runtime.GOOS
	}
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	if client == nil {
		client = http.DefaultClient
	}
	if output == nil {
		output = io.Discard
	}
	return goos, goarch, home, client, output
}

func (workflow DefaultMacroWorkflow) run(macro Macro) error {
	goos, goarch, home, _, output := workflow.settings()
	if !macroSupportsOS(macro, goos) {
		return fmt.Errorf("%s macros cannot run on %s", macro.Subpackage, goos)
	}
	if macro.Subpackage == "ydotool" {
		return workflow.runYdotool(macro)
	}
	findRuntime := workflow.FindRuntime
	if findRuntime == nil {
		findRuntime = findAutoHotkeyV2
	}
	runtimePath, err := findRuntime(goos, home)
	if err != nil {
		answer, askErr := workflow.UI.Ask(Question{ID: "install-autohotkey", Type: "confirm", Title: "AutoHotkey v2 is required. Install it now?"})
		if askErr != nil {
			return askErr
		}
		confirmed, ok := answer.(bool)
		if !ok {
			return fmt.Errorf("AutoHotkey installation confirmation returned an invalid answer")
		}
		if !confirmed {
			return fmt.Errorf("AutoHotkey v2 is required")
		}
		installRuntime := workflow.InstallRuntime
		if installRuntime == nil {
			installRuntime = workflow.install
		}
		if err := installRuntime(goos, goarch, home); err != nil {
			return err
		}
		if _, err := findRuntime(goos, home); err != nil {
			return fmt.Errorf("AutoHotkey v2 installation completed but validation failed: %w", err)
		}
		_, err = fmt.Fprintln(output, "AutoHotkey v2 installed successfully. Run 'tb macros' again to start the macro.")
		return err
	}
	arguments, err := workflow.macroArguments(macro)
	if err != nil {
		return err
	}
	startDetached := workflow.StartDetached
	if startDetached == nil {
		startDetached = startMacroDetached
	}
	pid, err := startDetached(runtimePath, macro.SourcePath, arguments)
	if err != nil {
		return fmt.Errorf("start macro: %w", err)
	}
	_, err = fmt.Fprintf(output, "Started %s in the background (PID %d).\n", macro.Title, pid)
	return err
}

func (workflow DefaultMacroWorkflow) macroArguments(macro Macro) ([]string, error) {
	arguments := []string{}
	for _, argument := range macro.Arguments {
		answer, askErr := workflow.UI.Ask(Question{ID: "macro-" + argument.ID, Type: "text", Title: argument.Prompt + " [default: " + argument.Default + "]"})
		if askErr != nil {
			return nil, askErr
		}
		value, ok := answer.(string)
		if !ok {
			return nil, fmt.Errorf("macro argument %q returned an invalid answer", argument.ID)
		}
		if strings.TrimSpace(value) == "" {
			value = argument.Default
		}
		if err := validateMacroArgument(argument, value); err != nil {
			return nil, fmt.Errorf("invalid %s: %w", argument.Prompt, err)
		}
		arguments = append(arguments, value)
	}
	return arguments, nil
}

func startMacroDetached(runtimePath string, scriptPath string, arguments []string) (int, error) {
	command := exec.Command(runtimePath, append([]string{scriptPath}, arguments...)...)
	command.Stdin = nil
	command.Stdout = nil
	command.Stderr = nil
	configureNonInteractive(command)
	if err := command.Start(); err != nil {
		return 0, err
	}
	return command.Process.Pid, nil
}

func autoHotkeyCandidates(goos, home string) []string {
	if goos != "windows" {
		return nil
	}
	names := []string{"AutoHotkey.exe", "AutoHotkey64.exe"}
	candidates := []string{}
	for _, name := range names {
		if path, err := exec.LookPath(name); err == nil {
			candidates = append(candidates, path)
		}
	}
	for _, base := range []string{os.Getenv("LOCALAPPDATA"), os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)")} {
		if base != "" {
			candidates = append(candidates, filepath.Join(base, "Programs", "AutoHotkey", "v2", "AutoHotkey64.exe"), filepath.Join(base, "AutoHotkey", "v2", "AutoHotkey64.exe"))
		}
	}
	return candidates
}

func findAutoHotkeyV2(goos, home string) (string, error) {
	for _, candidate := range autoHotkeyCandidates(goos, home) {
		if probeAutoHotkeyV2(candidate) == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("no working AutoHotkey v2 runtime found")
}

func probeAutoHotkeyV2(candidate string) error {
	if info, err := os.Stat(candidate); err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("runtime unavailable")
	}
	directory, err := os.MkdirTemp("", "tb-ahk-probe-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(directory)
	script := filepath.Join(directory, "probe.ahk")
	if err := os.WriteFile(script, []byte("#Requires AutoHotkey v2.0\nFileAppend A_AhkVersion, \"*\"\n"), 0o600); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, candidate, "/ErrorStdOut", script).CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("runtime probe timed out")
		}
		return err
	}
	if !strings.HasPrefix(strings.TrimSpace(string(output)), "2.") {
		return fmt.Errorf("runtime is not AutoHotkey v2")
	}
	return nil
}

type githubRelease struct {
	Assets []struct {
		Name   string `json:"name"`
		URL    string `json:"browser_download_url"`
		Digest string `json:"digest"`
	} `json:"assets"`
}

func (workflow DefaultMacroWorkflow) install(goos, goarch, home string) error {
	_, _, _, client, _ := workflow.settings()
	if goos != "windows" {
		return fmt.Errorf("AutoHotkey installation is supported only on Windows")
	}
	repository := "AutoHotkey/AutoHotkey"
	var release githubRelease
	if err := downloadJSON(client, "https://api.github.com/repos/"+repository+"/releases/latest", &release); err != nil {
		return fmt.Errorf("resolve latest AutoHotkey release: %w", err)
	}
	asset, ok := findReleaseAsset(release, "_setup.exe")
	if !ok {
		return fmt.Errorf("latest AutoHotkey release has no v2 setup")
	}
	path, cleanup, err := downloadTemporary(client, asset.URL, ".exe")
	if err != nil {
		return err
	}
	defer cleanup()
	if err := verifyDigest(path, asset.Digest, asset.Name); err != nil {
		return err
	}
	command := exec.Command(path)
	command.Stdout = workflow.Output
	command.Stderr = workflow.Output
	command.Stdin = os.Stdin
	if err := command.Run(); err != nil {
		return fmt.Errorf("run AutoHotkey v2 setup: %w", err)
	}
	return nil
}

func downloadJSON(client macroHTTPClient, url string, target any) error {
	response, err := client.Do(mustRequest(url))
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %s", response.Status)
	}
	return json.NewDecoder(response.Body).Decode(target)
}
func mustRequest(url string) *http.Request {
	request, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	request.Header.Set("User-Agent", "my-toolbox")
	return request
}
func findReleaseAsset(release githubRelease, suffix string) (struct {
	Name   string `json:"name"`
	URL    string `json:"browser_download_url"`
	Digest string `json:"digest"`
}, bool) {
	for _, asset := range release.Assets {
		if strings.HasSuffix(asset.Name, suffix) {
			return asset, true
		}
	}
	return struct {
		Name   string `json:"name"`
		URL    string `json:"browser_download_url"`
		Digest string `json:"digest"`
	}{}, false
}
func downloadTemporary(client macroHTTPClient, url, suffix string) (string, func(), error) {
	file, err := os.CreateTemp("", "tb-ahk-*"+suffix)
	if err != nil {
		return "", func() {}, err
	}
	cleanup := func() { os.Remove(file.Name()) }
	response, err := client.Do(mustRequest(url))
	if err != nil {
		file.Close()
		cleanup()
		return "", func() {}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		file.Close()
		cleanup()
		return "", func() {}, fmt.Errorf("download %s: HTTP %s", url, response.Status)
	}
	_, err = io.Copy(file, response.Body)
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		cleanup()
		return "", func() {}, err
	}
	return file.Name(), cleanup, nil
}
func verifyDigest(path, digest, name string) error {
	expected, found := strings.CutPrefix(digest, "sha256:")
	if !found || expected == "" {
		return fmt.Errorf("SHA-256 digest for %s is missing", name)
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return err
	}
	if !strings.EqualFold(expected, hex.EncodeToString(hash.Sum(nil))) {
		return fmt.Errorf("checksum mismatch for %s", name)
	}
	return nil
}
func extractTarGzip(archive, destination string) error {
	file, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer file.Close()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzipReader.Close()
	reader := tar.NewReader(gzipReader)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		clean := filepath.Clean(header.Name)
		if filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return fmt.Errorf("unsafe archive path %q", header.Name)
		}
		target := filepath.Join(destination, clean)
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, os.FileMode(header.Mode)&0o777)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(output, reader)
			closeErr := output.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		}
	}
}

func (workflow DefaultMacroWorkflow) download(macro Macro) error {
	_, _, home, _, output := workflow.settings()
	answer, err := workflow.UI.Ask(Question{ID: "macro-download-directory", Type: "text", Title: "Existing directory to download into"})
	if err != nil {
		return err
	}
	directory, ok := answer.(string)
	if !ok {
		return fmt.Errorf("download directory returned an invalid answer")
	}
	directory = strings.TrimSpace(directory)
	if directory == "" {
		return fmt.Errorf("download directory is required")
	}
	if directory == "~" {
		directory = home
	} else if strings.HasPrefix(directory, "~/") || strings.HasPrefix(directory, `~\`) {
		directory = filepath.Join(home, directory[2:])
	}
	if !filepath.IsAbs(directory) {
		directory, err = filepath.Abs(directory)
		if err != nil {
			return err
		}
	}
	info, err := os.Stat(directory)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("download directory does not exist: %s", directory)
	}
	destination := filepath.Join(directory, filepath.Base(macro.SourcePath))
	destinationExists := false
	if _, err := os.Lstat(destination); err == nil {
		destinationExists = true
		answer, askErr := workflow.UI.Ask(Question{ID: "macro-overwrite", Type: "confirm", Title: "Overwrite " + destination + "?"})
		if askErr != nil {
			return askErr
		}
		confirmed, ok := answer.(bool)
		if !ok || !confirmed {
			return ErrCancelled
		}
	}
	temporary, err := os.CreateTemp(directory, ".tb-macro-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	complete := false
	defer func() {
		if !complete {
			os.Remove(temporaryPath)
		}
	}()
	source, err := os.Open(macro.SourcePath)
	if err != nil {
		temporary.Close()
		return err
	}
	_, copyErr := io.Copy(temporary, source)
	source.Close()
	closeErr := temporary.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if err := os.Chmod(temporaryPath, 0o644); err != nil {
		return err
	}
	if err := publishMacroDownload(temporaryPath, destination, destinationExists); err != nil {
		return err
	}
	complete = true
	_, err = fmt.Fprintf(output, "Downloaded %s to %s.\n", macro.Title, destination)
	return err
}
