package execution

import (
	"os"
	"os/exec"
	"path/filepath"
)

func ResolveUserCommand(name, platform string) (string, error) {
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
