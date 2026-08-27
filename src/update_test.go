package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
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

func TestArchiveExtractorsRejectPathTraversal(t *testing.T) {
	tarBuffer := &bytes.Buffer{}
	gzipWriter := gzip.NewWriter(tarBuffer)
	tarWriter := tar.NewWriter(gzipWriter)
	content := []byte("outside")
	if err := tarWriter.WriteHeader(&tar.Header{Name: "../outside", Mode: 0o644, Size: int64(len(content))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write(content); err != nil {
		t.Fatal(err)
	}
	tarWriter.Close()
	gzipWriter.Close()
	if err := extractTarArchive(tarBuffer.Bytes(), t.TempDir()); err == nil {
		t.Fatal("extractTarArchive() accepted path traversal")
	}

	zipBuffer := &bytes.Buffer{}
	zipWriter := zip.NewWriter(zipBuffer)
	file, err := zipWriter.Create("../outside")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(content); err != nil {
		t.Fatal(err)
	}
	zipWriter.Close()
	if err := extractZipArchive(zipBuffer.Bytes(), t.TempDir()); err == nil {
		t.Fatal("extractZipArchive() accepted path traversal")
	}
}

func TestArchiveExtractorsPreserveSafeSymlinksAndRejectEscapes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symbolic links requires optional Windows privileges")
	}
	tests := []struct {
		name    string
		extract func([]byte, string) error
		build   func(string) []byte
	}{
		{
			name:    "tar",
			extract: extractTarArchive,
			build: func(target string) []byte {
				buffer := &bytes.Buffer{}
				gzipWriter := gzip.NewWriter(buffer)
				tarWriter := tar.NewWriter(gzipWriter)
				if err := tarWriter.WriteHeader(&tar.Header{Name: "template/link", Linkname: target, Typeflag: tar.TypeSymlink, Mode: 0o777}); err != nil {
					t.Fatal(err)
				}
				if err := tarWriter.Close(); err != nil {
					t.Fatal(err)
				}
				if err := gzipWriter.Close(); err != nil {
					t.Fatal(err)
				}
				return buffer.Bytes()
			},
		},
		{
			name:    "zip",
			extract: extractZipArchive,
			build: func(target string) []byte {
				buffer := &bytes.Buffer{}
				zipWriter := zip.NewWriter(buffer)
				header := &zip.FileHeader{Name: "template/link", Method: zip.Store}
				header.SetMode(os.ModeSymlink | 0o777)
				writer, err := zipWriter.CreateHeader(header)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := io.WriteString(writer, target); err != nil {
					t.Fatal(err)
				}
				if err := zipWriter.Close(); err != nil {
					t.Fatal(err)
				}
				return buffer.Bytes()
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name+" safe", func(t *testing.T) {
			root := t.TempDir()
			if err := test.extract(test.build("target.txt"), root); err != nil {
				t.Fatal(err)
			}
			target, err := os.Readlink(filepath.Join(root, "template", "link"))
			if err != nil || target != "target.txt" {
				t.Fatalf("Readlink() = %q, %v", target, err)
			}
		})
		t.Run(test.name+" escape", func(t *testing.T) {
			if err := test.extract(test.build("../../outside"), t.TempDir()); err == nil {
				t.Fatal("extractor accepted escaping symbolic link")
			}
		})
	}
}

func TestValidatePayloadRejectsMalformedArchive(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "commands.json"), []byte(`{"commands":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validatePayload(root, "linux-amd64", "0.1.2"); err == nil {
		t.Fatal("validatePayload() accepted malformed payload")
	}
}

func TestValidatePayloadRequiresEveryDeclaredNonBuiltinEntrypoint(t *testing.T) {
	root := t.TempDir()
	catalog := `{"commands":[{"name":"builtin-tool","description":"Builtin","package":"p","visibility":"list","protocol":"builtin","environments":["linux-native"],"entrypoints":{"linux-amd64":["builtin","builtin-tool"],"linux-arm64":["builtin","builtin-tool"]}},{"name":"script-tool","description":"Script","package":"p","visibility":"list","protocol":"interactive-script","environments":["linux-native"],"entrypoints":{"linux-amd64":["bash-script","packages/p/script.sh"],"linux-arm64":["bash-script","packages/p/script.sh"]}}]}`
	files := map[string][]byte{
		"tb":            []byte("binary"),
		"commands.json": []byte(catalog),
		"version.txt":   []byte("0.1.2\n"),
		"packages/agent-workspace-template/source/scripts/linux/python2/requirements.txt": []byte("toml==0.10.2\n"),
	}
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	err := validatePayload(root, "linux-amd64", "0.1.2")
	if err == nil || !strings.Contains(err.Error(), "packages/p/script.sh") {
		t.Fatalf("validatePayload() error = %v, want missing script", err)
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

func TestValidatePayloadRequiresEveryLinuxInteractiveInstallerAndMatchingVersion(t *testing.T) {
	root := t.TempDir()
	catalog, err := os.ReadFile("../commands.json")
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{
		"tb":            []byte("binary"),
		"commands.json": catalog,
		"version.txt":   []byte("0.1.2\n"),
		"packages/agent-workspace-template/source/scripts/linux/python3/install_codex.py":       []byte("script"),
		"packages/agent-workspace-template/source/scripts/linux/python3/install_claude.py":      []byte("script"),
		"packages/agent-workspace-template/source/scripts/linux/python3/install_antigravity.py": []byte("script"),
		"packages/agent-workspace-template/source/scripts/linux/python3/install_project.py":     []byte("script"),
		"packages/agent-workspace-template/source/scripts/linux/python2/install_codex.py":       []byte("script"),
		"packages/agent-workspace-template/source/scripts/linux/python2/install_claude.py":      []byte("script"),
		"packages/agent-workspace-template/source/scripts/linux/python2/install_antigravity.py": []byte("script"),
		"packages/agent-workspace-template/source/scripts/linux/python2/install_project.py":     []byte("script"),
		"packages/agent-workspace-template/source/scripts/linux/python2/requirements.txt":       []byte("toml==0.10.2\n"),
		"packages/scripts/terminal/alacritty/setup_alacritty.sh":                                []byte("script"),
		"packages/scripts/terminal/kitty/setup_kitty.sh":                                        []byte("script"),
		"packages/scripts/terminal/wsl/setup_wsl.sh":                                            []byte("script"),
		"packages/scripts/terminal/wsl/set_default_cwd.sh":                                      []byte("script"),
		"packages/scripts/utils/change_grub_order.sh":                                           []byte("script"),
		"packages/scripts/utils/setup_venv.sh":                                                  []byte("script"),
		"packages/scripts/utils/toggle_nopasswd_sudo.sh":                                        []byte("script"),
		"packages/others/create_env_alias.py":                                                   []byte("script"),
		"packages/others/bootstrap_python_from_venv.py":                                         []byte("script"),
		"packages/others/create_project_template.py":                                            []byte("script"),
	}
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := validatePayload(root, "linux-amd64", "0.1.2"); err != nil {
		t.Fatal(err)
	}
	for _, relativePath := range []string{
		"packages/agent-workspace-template/source/scripts/linux/python3/install_codex.py",
		"packages/agent-workspace-template/source/scripts/linux/python3/install_claude.py",
		"packages/agent-workspace-template/source/scripts/linux/python3/install_antigravity.py",
		"packages/agent-workspace-template/source/scripts/linux/python3/install_project.py",
		"packages/agent-workspace-template/source/scripts/linux/python2/install_codex.py",
		"packages/agent-workspace-template/source/scripts/linux/python2/install_claude.py",
		"packages/agent-workspace-template/source/scripts/linux/python2/install_antigravity.py",
		"packages/agent-workspace-template/source/scripts/linux/python2/install_project.py",
		"packages/agent-workspace-template/source/scripts/linux/python2/requirements.txt",
		"packages/scripts/terminal/alacritty/setup_alacritty.sh",
		"packages/scripts/terminal/kitty/setup_kitty.sh",
		"packages/scripts/terminal/wsl/setup_wsl.sh",
		"packages/scripts/terminal/wsl/set_default_cwd.sh",
		"packages/scripts/utils/change_grub_order.sh",
		"packages/scripts/utils/setup_venv.sh",
		"packages/scripts/utils/toggle_nopasswd_sudo.sh",
		"packages/others/create_env_alias.py",
		"packages/others/bootstrap_python_from_venv.py",
		"packages/others/create_project_template.py",
	} {
		missingPath := filepath.Join(root, filepath.FromSlash(relativePath))
		content, err := os.ReadFile(missingPath)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(missingPath); err != nil {
			t.Fatal(err)
		}
		if err := validatePayload(root, "linux-amd64", "0.1.2"); err == nil {
			t.Fatalf("validatePayload() accepted a payload without %s", relativePath)
		}
		if err := os.WriteFile(missingPath, content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "version.txt"), []byte("0.1.3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validatePayload(root, "linux-amd64", "0.1.2"); err == nil {
		t.Fatal("validatePayload() accepted mismatched version.txt")
	}
}

func TestValidatePayloadRequiresEveryWindowsInteractiveInstaller(t *testing.T) {
	root := t.TempDir()
	catalog, err := os.ReadFile("../commands.json")
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{
		"tb.exe":        []byte("binary"),
		"commands.json": catalog,
		"version.txt":   []byte("0.1.2\n"),
		"packages/agent-workspace-template/source/scripts/windows/install_codex.py":       []byte("script"),
		"packages/agent-workspace-template/source/scripts/windows/install_claude.py":      []byte("script"),
		"packages/agent-workspace-template/source/scripts/windows/install_antigravity.py": []byte("script"),
		"packages/agent-workspace-template/source/scripts/windows/install_project.py":     []byte("script"),
		"packages/scripts/terminal/windows/setup_windows.ps1":                             []byte("script"),
		"packages/scripts/terminal/windows/set_vscode_wsl_cwd.ps1":                        []byte("script"),
		"packages/others/create_project_template.py":                                      []byte("script"),
	}
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := validatePayload(root, "windows-amd64", "0.1.2"); err != nil {
		t.Fatal(err)
	}
	for _, relativePath := range []string{
		"packages/agent-workspace-template/source/scripts/windows/install_codex.py",
		"packages/agent-workspace-template/source/scripts/windows/install_claude.py",
		"packages/agent-workspace-template/source/scripts/windows/install_antigravity.py",
		"packages/agent-workspace-template/source/scripts/windows/install_project.py",
		"packages/scripts/terminal/windows/setup_windows.ps1",
		"packages/scripts/terminal/windows/set_vscode_wsl_cwd.ps1",
		"packages/others/create_project_template.py",
	} {
		missingPath := filepath.Join(root, filepath.FromSlash(relativePath))
		if err := os.Remove(missingPath); err != nil {
			t.Fatal(err)
		}
		if err := validatePayload(root, "windows-amd64", "0.1.2"); err == nil {
			t.Fatalf("validatePayload() accepted a payload without %s", relativePath)
		}
		if err := os.WriteFile(missingPath, []byte("script"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestUpdateInstallsAndAtomicallySwitchesValidatedVersion(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	archive := validToolboxTar(t)
	digest := sha256.Sum256(archive)
	builtins := updateBuiltins(t, "0.1.1", archive, fmt.Sprintf("%x  toolbox-linux-amd64.tar.gz\n", digest))

	if err := builtins.update(); err != nil {
		t.Fatal(err)
	}
	dataRoot := filepath.Join(home, ".local", "share", "my-toolbox")
	current, err := os.ReadFile(filepath.Join(dataRoot, "current.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(current) != "0.1.2\n" {
		t.Fatalf("current.txt = %q", current)
	}
	if _, err := os.Stat(filepath.Join(dataRoot, "versions", "0.1.2", "tb")); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateSkipsSameVersionWithoutDownloadingArchive(t *testing.T) {
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

func TestUpdateRejectsInvalidChecksumBeforeCreatingVersion(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	archive := validToolboxTar(t)
	builtins := updateBuiltins(t, "0.1.1", archive, strings.Repeat("0", 64)+"  toolbox-linux-amd64.tar.gz\n")

	if err := builtins.update(); err == nil || !strings.Contains(err.Error(), "SHA-256 mismatch") {
		t.Fatalf("update() error = %v", err)
	}
	versionRoot := filepath.Join(home, ".local", "share", "my-toolbox", "versions", "0.1.2")
	if _, err := os.Stat(versionRoot); !os.IsNotExist(err) {
		t.Fatalf("invalid archive created version directory: %v", err)
	}
}

func TestUpdateRejectsMalformedArchiveAfterChecksum(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	archive := []byte("not an archive")
	digest := sha256.Sum256(archive)
	builtins := updateBuiltins(t, "0.1.1", archive, fmt.Sprintf("%x  toolbox-linux-amd64.tar.gz\n", digest))
	if err := builtins.update(); err == nil || !strings.Contains(err.Error(), "archive") {
		t.Fatalf("update() error = %v", err)
	}
}

func updateBuiltins(t *testing.T, currentVersion string, archive []byte, checksum string) *ToolboxBuiltins {
	t.Helper()
	builtins := NewToolboxBuiltins("", "linux-amd64", currentVersion, io.Discard)
	builtins.client = &http.Client{Transport: roundTripFunction(func(request *http.Request) (*http.Response, error) {
		switch {
		case strings.HasSuffix(request.URL.Path, "/releases/latest"):
			return responseWithBody(`{"tag_name":"v0.1.2"}`), nil
		case strings.HasSuffix(request.URL.Path, "/toolbox-linux-amd64.tar.gz.sha256"):
			return responseWithBody(checksum), nil
		case strings.HasSuffix(request.URL.Path, "/toolbox-linux-amd64.tar.gz"):
			return responseWithBytes(archive), nil
		default:
			t.Fatalf("unexpected update URL: %s", request.URL)
			return nil, nil
		}
	})}
	return builtins
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

func validToolboxTar(t *testing.T) []byte {
	t.Helper()
	catalog, err := os.ReadFile("../commands.json")
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{
		"tb":            []byte("binary"),
		"commands.json": catalog,
		"version.txt":   []byte("0.1.2\n"),
		"packages/agent-workspace-template/source/scripts/linux/python3/install_codex.py":       []byte("script"),
		"packages/agent-workspace-template/source/scripts/linux/python3/install_claude.py":      []byte("script"),
		"packages/agent-workspace-template/source/scripts/linux/python3/install_antigravity.py": []byte("script"),
		"packages/agent-workspace-template/source/scripts/linux/python3/install_project.py":     []byte("script"),
		"packages/agent-workspace-template/source/scripts/linux/python2/install_codex.py":       []byte("script"),
		"packages/agent-workspace-template/source/scripts/linux/python2/install_claude.py":      []byte("script"),
		"packages/agent-workspace-template/source/scripts/linux/python2/install_antigravity.py": []byte("script"),
		"packages/agent-workspace-template/source/scripts/linux/python2/install_project.py":     []byte("script"),
		"packages/agent-workspace-template/source/scripts/linux/python2/requirements.txt":       []byte("toml==0.10.2\n"),
		"packages/scripts/terminal/alacritty/setup_alacritty.sh":                                []byte("script"),
		"packages/scripts/terminal/kitty/setup_kitty.sh":                                        []byte("script"),
		"packages/scripts/terminal/wsl/setup_wsl.sh":                                            []byte("script"),
		"packages/scripts/terminal/wsl/set_default_cwd.sh":                                      []byte("script"),
		"packages/scripts/utils/change_grub_order.sh":                                           []byte("script"),
		"packages/scripts/utils/setup_venv.sh":                                                  []byte("script"),
		"packages/scripts/utils/toggle_nopasswd_sudo.sh":                                        []byte("script"),
		"packages/others/create_env_alias.py":                                                   []byte("script"),
		"packages/others/bootstrap_python_from_venv.py":                                         []byte("script"),
		"packages/others/create_project_template.py":                                            []byte("script"),
	}
	archive := &bytes.Buffer{}
	gzipWriter := gzip.NewWriter(archive)
	tarWriter := tar.NewWriter(gzipWriter)
	for name, content := range files {
		mode := int64(0o644)
		if name == "tb" {
			mode = 0o755
		}
		if err := tarWriter.WriteHeader(&tar.Header{Name: name, Mode: mode, Size: int64(len(content))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tarWriter.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return archive.Bytes()
}
