package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

type roundTripFunction func(*http.Request) (*http.Response, error)

func (function roundTripFunction) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestChecksumVerificationRejectsWrongContent(t *testing.T) {
	content := []byte("verified payload")
	digest := sha256.Sum256(content)
	if err := verifyChecksum(content, fmt.Sprintf("%x", digest)); err != nil {
		t.Fatalf("verifyChecksum(valid) error = %v", err)
	}
	if err := verifyChecksum([]byte("tampered payload"), fmt.Sprintf("%x", digest)); err == nil {
		t.Fatal("verifyChecksum(tampered) succeeded")
	}
}

func TestChecksumLookupRequiresNamedSHA256(t *testing.T) {
	digest := strings.Repeat("a", 64)
	got, err := checksumFor([]byte(digest+"  archive.tar.gz\n"), "archive.tar.gz")
	if err != nil || got != digest {
		t.Fatalf("checksumFor() = %q, %v", got, err)
	}
	if _, err := checksumFor([]byte(digest+"  other.tar.gz\n"), "archive.tar.gz"); err == nil {
		t.Fatal("checksumFor() accepted checksum for a different archive")
	}
}

func TestCompareVersionsRejectsUnsafeAndOlderReleases(t *testing.T) {
	for _, version := range []string{"../1", "1.2", "1.02.3", "1.2.x", "/1.2.3"} {
		if _, err := compareVersions(version, "1.2.3"); err == nil {
			t.Fatalf("compareVersions(%q) accepted unsafe version", version)
		}
	}
	comparison, err := compareVersions("1.2.2", "1.2.3")
	if err != nil || comparison != -1 {
		t.Fatalf("compareVersions(older) = %d, %v", comparison, err)
	}
	comparison, err = compareVersions("1.3.0", "1.2.9")
	if err != nil || comparison != 1 {
		t.Fatalf("compareVersions(newer) = %d, %v", comparison, err)
	}
}

func TestUpdateRunsInstallerWithActiveInstallationAndClosedInput(t *testing.T) {
	home := t.TempDir()
	dataBase := filepath.Join(home, "data")
	dataRoot := filepath.Join(dataBase, "my-toolbox")
	versionRoot := filepath.Join(dataRoot, "versions", "0.1.1")
	wrapper := filepath.Join(home, ".local", "bin", "tb")
	platform := "linux-amd64"
	expectedWrapper := linuxToolboxWrapper
	if runtime.GOOS == "windows" {
		platform = "windows-amd64"
		wrapper = filepath.Join(dataRoot, "bin", "tb.cmd")
		expectedWrapper = windowsToolboxWrapper
		t.Setenv("LOCALAPPDATA", dataBase)
	}
	marker := filepath.Join(home, "installed")
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", dataBase)
	t.Setenv("TOOLBOX_UPDATE", "0")
	t.Setenv("TOOLBOX_EXPECTED_VERSION", "stale")
	if err := os.MkdirAll(versionRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(wrapper), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(wrapper, []byte(expectedWrapper), 0o755); err != nil {
		t.Fatal(err)
	}
	installer := fmt.Sprintf("#!/bin/sh\nset -eu\n[ -d %q ]\n[ -f %q ]\n[ \"$TOOLBOX_UPDATE\" = 1 ]\n[ \"$TOOLBOX_EXPECTED_VERSION\" = 0.1.2 ]\nif read -r input; then exit 91; fi\nprintf installed > %q\n", versionRoot, wrapper, marker)
	installerURL := toolboxLinuxInstallerURL
	if runtime.GOOS == "windows" {
		installerURL = toolboxWindowsInstallerURL
		installer = fmt.Sprintf("$ErrorActionPreference = 'Stop'\nif (-not (Test-Path -LiteralPath '%s') -or -not (Test-Path -LiteralPath '%s')) { exit 92 }\nif ($env:TOOLBOX_UPDATE -cne '1' -or $env:TOOLBOX_EXPECTED_VERSION -cne '0.1.2') { exit 93 }\nif ([Console]::In.ReadToEnd() -cne '') { exit 94 }\n[IO.File]::WriteAllText('%s', 'installed')\n", strings.ReplaceAll(versionRoot, "'", "''"), strings.ReplaceAll(wrapper, "'", "''"), strings.ReplaceAll(marker, "'", "''"))
	}
	requests := []string{}
	builtins := NewToolboxBuiltins(versionRoot, platform, "0.1.1", io.Discard)
	builtins.client = &http.Client{Transport: roundTripFunction(func(request *http.Request) (*http.Response, error) {
		requests = append(requests, request.URL.String())
		switch request.URL.String() {
		case "https://api.github.com/repos/MatheusFS-dev/my-toolbox/releases/latest":
			return responseWithBody(`{"tag_name":"v0.1.2"}`), nil
		case installerURL:
			return responseWithBody(installer), nil
		default:
			t.Fatalf("unexpected update URL: %s", request.URL)
			return nil, nil
		}
	})}

	if err := builtins.update(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(requests, []string{
		"https://api.github.com/repos/MatheusFS-dev/my-toolbox/releases/latest",
		installerURL,
	}) {
		t.Fatalf("update requests = %v", requests)
	}
	content, err := os.ReadFile(marker)
	if err != nil || string(content) != "installed" {
		t.Fatalf("installer marker = %q, %v", content, err)
	}
	if _, err := os.Stat(versionRoot); err != nil {
		t.Fatalf("update removed the active version: %v", err)
	}
	if content, err := os.ReadFile(wrapper); err != nil || string(content) != expectedWrapper {
		t.Fatalf("update changed the wrapper: %q, %v", content, err)
	}
}

func TestUpdatePreservesCurrentInstallationWhenInstallerFails(t *testing.T) {
	home := t.TempDir()
	dataBase := filepath.Join(home, "data")
	dataRoot := filepath.Join(dataBase, "my-toolbox")
	versionRoot := filepath.Join(dataRoot, "versions", "0.1.1")
	wrapper := filepath.Join(home, ".local", "bin", "tb")
	platform := "linux-amd64"
	expectedWrapper := linuxToolboxWrapper
	if runtime.GOOS == "windows" {
		platform = "windows-amd64"
		wrapper = filepath.Join(dataRoot, "bin", "tb.cmd")
		expectedWrapper = windowsToolboxWrapper
		t.Setenv("LOCALAPPDATA", dataBase)
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", dataBase)
	files := map[string]string{
		filepath.Join(versionRoot, "tb"):          "active executable",
		filepath.Join(versionRoot, "version.txt"): "0.1.1\n",
		filepath.Join(dataRoot, "current.txt"):    "0.1.1\n",
		wrapper:                                   expectedWrapper,
	}
	for path, content := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	builtins := NewToolboxBuiltins(versionRoot, platform, "0.1.1", io.Discard)
	builtins.client = &http.Client{Transport: roundTripFunction(func(request *http.Request) (*http.Response, error) {
		if strings.HasSuffix(request.URL.Path, "/releases/latest") {
			return responseWithBody(`{"tag_name":"v0.1.2"}`), nil
		}
		return responseWithBody("exit 23\n"), nil
	})}
	if err := builtins.update(); err == nil || !strings.Contains(err.Error(), "exit status 23") {
		t.Fatalf("update() error = %v", err)
	}
	for path, want := range files {
		if content, err := os.ReadFile(path); err != nil || string(content) != want {
			t.Fatalf("failed update changed %s: %q, %v", path, content, err)
		}
	}
}

func TestUpdatePreservesCurrentInstallationWhenInstallerDownloadFails(t *testing.T) {
	home := t.TempDir()
	dataBase := filepath.Join(home, "data")
	versionRoot := filepath.Join(dataBase, "my-toolbox", "versions", "0.1.1")
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", dataBase)
	if err := os.MkdirAll(versionRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	builtins := NewToolboxBuiltins(versionRoot, "linux-amd64", "0.1.1", io.Discard)
	builtins.client = &http.Client{Transport: roundTripFunction(func(request *http.Request) (*http.Response, error) {
		if strings.HasSuffix(request.URL.Path, "/releases/latest") {
			return responseWithBody(`{"tag_name":"v0.1.2"}`), nil
		}
		return &http.Response{StatusCode: http.StatusServiceUnavailable, Status: "503 Service Unavailable", Body: io.NopCloser(strings.NewReader("unavailable"))}, nil
	})}

	if err := builtins.update(); err == nil || !strings.Contains(err.Error(), "HTTP 503 Service Unavailable") {
		t.Fatalf("update() error = %v", err)
	}
	if _, err := os.Stat(versionRoot); err != nil {
		t.Fatalf("failed installer download changed current installation: %v", err)
	}
}

func TestUpdateRefusesUnrecognizedWrapperWithoutChangingCurrentInstallation(t *testing.T) {
	home := t.TempDir()
	dataBase := filepath.Join(home, "data")
	versionRoot := filepath.Join(dataBase, "my-toolbox", "versions", "0.1.1")
	wrapper := filepath.Join(home, ".local", "bin", "tb")
	platform := "linux-amd64"
	if runtime.GOOS == "windows" {
		platform = "windows-amd64"
		wrapper = filepath.Join(dataBase, "my-toolbox", "bin", "tb.cmd")
		t.Setenv("LOCALAPPDATA", dataBase)
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", dataBase)
	if err := os.MkdirAll(versionRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(wrapper), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(wrapper, []byte("user-owned executable"), 0o755); err != nil {
		t.Fatal(err)
	}
	builtins := NewToolboxBuiltins(versionRoot, platform, "0.1.1", io.Discard)
	builtins.client = &http.Client{Transport: roundTripFunction(func(request *http.Request) (*http.Response, error) {
		if strings.HasSuffix(request.URL.Path, "/releases/latest") {
			return responseWithBody(`{"tag_name":"v0.1.2"}`), nil
		}
		return responseWithBody("#!/bin/sh\nexit 0\n"), nil
	})}

	err := builtins.update()
	if err == nil || !strings.Contains(err.Error(), "refusing to update with unrecognized wrapper") {
		t.Fatalf("update() error = %v", err)
	}
	if _, err := os.Stat(versionRoot); err != nil {
		t.Fatalf("update changed current installation: %v", err)
	}
	content, err := os.ReadFile(wrapper)
	if err != nil || string(content) != "user-owned executable" {
		t.Fatalf("update changed unrecognized wrapper: %q, %v", content, err)
	}
}

func TestUpdateSkipsSameVersionWithoutDownloadingInstaller(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	requests := 0
	builtins := NewToolboxBuiltins("", "linux-amd64", "0.1.2", io.Discard)
	builtins.client = &http.Client{Transport: roundTripFunction(func(request *http.Request) (*http.Response, error) {
		requests++
		if !strings.HasSuffix(request.URL.Path, "/releases/latest") {
			t.Fatalf("unexpected download for same version: %s", request.URL)
		}
		return responseWithBody(`{"tag_name":"v0.1.2"}`), nil
	})}
	if err := builtins.update(); err != nil {
		t.Fatal(err)
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want metadata only", requests)
	}
}

func responseWithBody(body string) *http.Response {
	return responseWithBytes([]byte(body))
}

func responseWithBytes(body []byte) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
}
