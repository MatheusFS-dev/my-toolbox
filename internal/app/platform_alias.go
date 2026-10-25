package app

import "github.com/MatheusFS-dev/my-toolbox/internal/platform"

func CurrentPlatform() (string, error)    { return platform.CurrentPlatform() }
func CurrentEnvironment() (string, error) { return platform.CurrentEnvironment() }
func detectEnvironment(goos, osreleasePath string) (string, error) {
	return platform.DetectEnvironment(goos, osreleasePath)
}
func executableRoot() (string, error) { return platform.ExecutableRoot() }
