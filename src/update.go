package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

const toolboxRepository = "MatheusFS-dev/my-toolbox"

func (builtins *ToolboxBuiltins) update() error {
	var release struct {
		TagName string `json:"tag_name"`
	}
	metadata, err := builtins.download("https://api.github.com/repos/" + toolboxRepository + "/releases/latest")
	if err != nil {
		return err
	}
	if err := json.Unmarshal(metadata, &release); err != nil {
		return fmt.Errorf("decode latest toolbox release: %w", err)
	}
	if release.TagName == "" {
		return fmt.Errorf("latest toolbox release is missing tag_name")
	}
	latest := strings.TrimPrefix(release.TagName, "v")
	comparison, err := compareVersions(latest, builtins.version)
	if err != nil {
		return err
	}
	if comparison == 0 {
		_, err := fmt.Fprintf(builtins.output, "Toolbox %s is already current.\n", latest)
		return err
	}
	if comparison < 0 {
		return fmt.Errorf("latest release %s is older than installed version %s", latest, builtins.version)
	}
	archiveName := map[string]string{
		"linux-amd64":   "toolbox-linux-amd64.tar.gz",
		"linux-arm64":   "toolbox-linux-arm64.tar.gz",
		"windows-amd64": "toolbox-windows-amd64.zip",
	}[builtins.platform]
	if archiveName == "" {
		return fmt.Errorf("unsupported update platform %s", builtins.platform)
	}
	baseURL := fmt.Sprintf("https://github.com/%s/releases/download/%s", toolboxRepository, release.TagName)
	archive, err := builtins.download(baseURL + "/" + archiveName)
	if err != nil {
		return err
	}
	checksumFile, err := builtins.download(baseURL + "/" + archiveName + ".sha256")
	if err != nil {
		return err
	}
	expected, err := checksumFor(checksumFile, archiveName)
	if err != nil {
		return err
	}
	if err := verifyChecksum(archive, expected); err != nil {
		return fmt.Errorf("verify %s: %w", archiveName, err)
	}
	base, err := toolboxDataRoot(builtins.platform)
	if err != nil {
		return err
	}
	versions := filepath.Join(base, "versions")
	if err := os.MkdirAll(versions, 0o755); err != nil {
		return fmt.Errorf("create versions directory: %w", err)
	}
	destination := filepath.Join(versions, latest)
	if _, err := os.Stat(destination); err == nil {
		return fmt.Errorf("version directory already exists: %s", destination)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect version directory: %w", err)
	}
	staging, err := os.MkdirTemp(versions, ".update-*")
	if err != nil {
		return fmt.Errorf("create update staging directory: %w", err)
	}
	defer os.RemoveAll(staging)
	if strings.HasSuffix(archiveName, ".zip") {
		err = extractZipArchive(archive, staging)
	} else {
		err = extractTarArchive(archive, staging)
	}
	if err != nil {
		return err
	}
	if err := validatePayload(staging, builtins.platform, latest); err != nil {
		return err
	}
	if err := os.Rename(staging, destination); err != nil {
		return fmt.Errorf("install version %s: %w", latest, err)
	}
	if err := replaceCurrentFile(base, latest); err != nil {
		return err
	}
	_, err = fmt.Fprintf(builtins.output, "Updated toolbox to %s.\n", latest)
	return err
}

func toolboxDataRoot(platform string) (string, error) {
	if platform == "windows-amd64" {
		root := os.Getenv("LOCALAPPDATA")
		if root == "" {
			return "", fmt.Errorf("LOCALAPPDATA is not set")
		}
		return filepath.Join(root, "my-toolbox"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home: %w", err)
	}
	if root := os.Getenv("XDG_DATA_HOME"); root != "" {
		return filepath.Join(root, "my-toolbox"), nil
	}
	return filepath.Join(home, ".local", "share", "my-toolbox"), nil
}

func validatePayload(root, platform, expectedVersion string) error {
	catalog, err := LoadCatalogFile(filepath.Join(root, "commands.json"))
	if err != nil {
		return fmt.Errorf("invalid toolbox payload catalog: %w", err)
	}
	binary := "tb"
	required := []string{}
	if platform == "windows-amd64" {
		binary = "tb.exe"
	} else {
		required = append(required, filepath.Join(root, "packages", "agent-workspace-template", "source", "scripts", "linux", "python2", "requirements.txt"))
	}
	for _, command := range catalog.Commands {
		if command.Protocol == "builtin" {
			continue
		}
		entrypoint, exists := command.Entrypoints[platform]
		if !exists {
			continue
		}
		for _, script := range entrypoint[1:] {
			required = append(required, filepath.Join(root, filepath.FromSlash(script)))
		}
	}
	required = append(required, filepath.Join(root, binary), filepath.Join(root, "version.txt"))
	for _, path := range required {
		if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("toolbox payload is missing required file %s", path)
		}
	}
	versionContent, err := os.ReadFile(filepath.Join(root, "version.txt"))
	if err != nil {
		return fmt.Errorf("read payload version: %w", err)
	}
	if strings.TrimSpace(string(versionContent)) != expectedVersion {
		return fmt.Errorf("payload version %q does not match release %q", strings.TrimSpace(string(versionContent)), expectedVersion)
	}
	return nil
}

func compareVersions(candidate, current string) (int, error) {
	candidateParts, err := parseVersion(candidate)
	if err != nil {
		return 0, fmt.Errorf("invalid release version %q: %w", candidate, err)
	}
	currentParts, err := parseVersion(current)
	if err != nil {
		return 0, fmt.Errorf("invalid installed version %q: %w", current, err)
	}
	for index := range candidateParts {
		if candidateParts[index] < currentParts[index] {
			return -1, nil
		}
		if candidateParts[index] > currentParts[index] {
			return 1, nil
		}
	}
	return 0, nil
}

func parseVersion(version string) ([3]int, error) {
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return [3]int{}, fmt.Errorf("expected three numeric components")
	}
	var values [3]int
	for index, part := range parts {
		if part == "" || (len(part) > 1 && part[0] == '0') {
			return [3]int{}, fmt.Errorf("component %q is not canonical", part)
		}
		value, err := strconv.Atoi(part)
		if err != nil || value < 0 {
			return [3]int{}, fmt.Errorf("component %q is not numeric", part)
		}
		values[index] = value
	}
	return values, nil
}

func extractTarArchive(content []byte, destination string) error {
	gzipReader, err := gzip.NewReader(bytes.NewReader(content))
	if err != nil {
		return fmt.Errorf("open toolbox tar archive: %w", err)
	}
	defer gzipReader.Close()
	reader := tar.NewReader(gzipReader)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read toolbox tar archive: %w", err)
		}
		path, err := archiveDestination(destination, header.Name)
		if err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := createArchiveDirectory(destination, path); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := writeArchiveFile(destination, path, io.LimitReader(reader, header.Size), os.FileMode(header.Mode)); err != nil {
				return err
			}
		case tar.TypeSymlink:
			if err := writeArchiveSymlink(destination, path, header.Name, header.Linkname); err != nil {
				return err
			}
		default:
			return fmt.Errorf("archive contains unsupported entry %s", header.Name)
		}
	}
}

func extractZipArchive(content []byte, destination string) error {
	reader, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return fmt.Errorf("open toolbox zip archive: %w", err)
	}
	for _, file := range reader.File {
		path, err := archiveDestination(destination, file.Name)
		if err != nil {
			return err
		}
		if file.FileInfo().IsDir() {
			if err := createArchiveDirectory(destination, path); err != nil {
				return err
			}
			continue
		}
		source, err := file.Open()
		if err != nil {
			return fmt.Errorf("open archive file %s: %w", file.Name, err)
		}
		if file.Mode()&os.ModeSymlink != 0 {
			linkTarget, readErr := io.ReadAll(io.LimitReader(source, 4097))
			if readErr != nil {
				source.Close()
				return fmt.Errorf("read archive symlink %s: %w", file.Name, readErr)
			}
			if len(linkTarget) > 4096 {
				source.Close()
				return fmt.Errorf("archive symlink target is too long: %s", file.Name)
			}
			err = writeArchiveSymlink(destination, path, file.Name, string(linkTarget))
		} else {
			err = writeArchiveFile(destination, path, source, file.Mode())
		}
		source.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func archiveDestination(root, name string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(name))
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("archive contains unsafe path %q", name)
	}
	return filepath.Join(root, clean), nil
}

func writeArchiveFile(root, path string, source io.Reader, mode os.FileMode) error {
	if err := ensureArchiveParent(root, path); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode.Perm())
	if err != nil {
		return fmt.Errorf("create archive file %s: %w", path, err)
	}
	if _, err := io.Copy(file, source); err != nil {
		file.Close()
		return fmt.Errorf("write archive file %s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close archive file %s: %w", path, err)
	}
	return nil
}

func createArchiveDirectory(root, directory string) error {
	if err := ensureArchiveParent(root, directory); err != nil {
		return err
	}
	info, err := os.Lstat(directory)
	if err == nil {
		if info.IsDir() {
			return nil
		}
		return fmt.Errorf("archive directory conflicts with existing path: %s", directory)
	}
	if !os.IsNotExist(err) {
		return fmt.Errorf("inspect archive directory %s: %w", directory, err)
	}
	if err := os.Mkdir(directory, 0o755); err != nil {
		return fmt.Errorf("create archive directory %s: %w", directory, err)
	}
	return nil
}

func ensureArchiveParent(root, destination string) error {
	relative, err := filepath.Rel(root, filepath.Dir(destination))
	if err != nil {
		return fmt.Errorf("resolve archive parent: %w", err)
	}
	current := root
	for _, component := range strings.Split(relative, string(filepath.Separator)) {
		if component == "." || component == "" {
			continue
		}
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			if err := os.Mkdir(current, 0o755); err != nil {
				return fmt.Errorf("create archive parent %s: %w", current, err)
			}
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect archive parent %s: %w", current, err)
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("archive entry traverses non-directory parent %s", current)
		}
	}
	return nil
}

func writeArchiveSymlink(root, destination, entryName, target string) error {
	if target == "" || strings.ContainsRune(target, '\x00') || strings.Contains(target, "\\") {
		return fmt.Errorf("archive contains unsafe symlink target %q", target)
	}
	cleanTarget := path.Clean(target)
	resolved := path.Clean(path.Join(path.Dir(entryName), cleanTarget))
	localTarget := filepath.FromSlash(cleanTarget)
	if path.IsAbs(cleanTarget) || filepath.IsAbs(localTarget) || filepath.VolumeName(localTarget) != "" || resolved == ".." || strings.HasPrefix(resolved, "../") {
		return fmt.Errorf("archive symlink %s escapes extraction root", entryName)
	}
	if err := ensureArchiveParent(root, destination); err != nil {
		return err
	}
	if _, err := os.Lstat(destination); err == nil {
		return fmt.Errorf("archive symlink conflicts with existing path: %s", destination)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect archive symlink %s: %w", destination, err)
	}
	if err := os.Symlink(localTarget, destination); err != nil {
		return fmt.Errorf("create archive symlink %s: %w", destination, err)
	}
	return nil
}

func replaceCurrentFile(base, version string) error {
	temporary, err := os.CreateTemp(base, "current-*.txt")
	if err != nil {
		return fmt.Errorf("create current version file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := fmt.Fprintln(temporary, version); err != nil {
		temporary.Close()
		return fmt.Errorf("write current version file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close current version file: %w", err)
	}
	if err := atomicReplace(temporaryPath, filepath.Join(base, "current.txt")); err != nil {
		return fmt.Errorf("activate version %s: %w", version, err)
	}
	return nil
}
