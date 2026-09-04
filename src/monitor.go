//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

type monitorInterpreter struct {
	Path        string
	Environment string
}

type monitorEmailSettings struct {
	Provider string
	Host     string
	Port     string
	Security string
	Sender   string
	Password string
}

func monitorEmailCredentials(provider, sender, password string, existing monitorEmailSettings) monitorEmailSettings {
	credentials := existing
	credentials.Provider = provider
	credentials.Sender = strings.TrimSpace(sender)
	if password != "" {
		credentials.Password = password
	}
	if provider == "gmail" {
		if password != "" {
			credentials.Password = strings.ReplaceAll(password, " ", "")
		}
		credentials.Host = "smtp.gmail.com"
		credentials.Port = "587"
		credentials.Security = "starttls"
	}
	return credentials
}

func monitorConfigurationSummary(credentials monitorEmailSettings, recipients []string, quick bool) string {
	provider := "Custom email server"
	if credentials.Provider == "gmail" {
		provider = "Gmail"
	}
	settings := "custom monitoring settings"
	if quick {
		settings = "default monitoring settings"
	}
	return fmt.Sprintf("Email provider: %s\nSender: %s\nNotifications go to: %s\nMonitoring: %s", provider, credentials.Sender, strings.Join(recipients, ", "), settings)
}

func applyMonitorRestartMode(restart map[string]any, enabled bool, mode string, value float64) error {
	if !enabled {
		restart["memory_aware"] = false
		restart["scheduled_interval_minutes"] = 0
		return nil
	}
	if value <= 0 {
		return fmt.Errorf("automatic restart value must be greater than zero")
	}
	switch mode {
	case "memory":
		restart["memory_aware"] = true
		restart["memory_limit_gb"] = value
		delete(restart, "memory_limit_mib")
		restart["scheduled_interval_minutes"] = 0
	case "time":
		restart["memory_aware"] = false
		restart["scheduled_interval_minutes"] = int(value)
	default:
		return fmt.Errorf("unknown automatic restart mode: %s", mode)
	}
	return nil
}

func normalizeMonitorRestart(restart map[string]any) {
	if _, present := restart["rapid_crash_seconds"]; !present {
		restart["rapid_crash_seconds"] = 60
	}
	if _, present := restart["memory_limit_gb"]; !present {
		if legacy, legacyPresent := restart["memory_limit_mib"]; legacyPresent {
			restart["memory_limit_gb"] = numberValue(legacy, 1024) * 1048576 / 1000000000
		} else {
			restart["memory_limit_gb"] = 1.0
		}
	}
	delete(restart, "memory_limit_mib")
}

func applyMonitorLeakWarmup(leak map[string]any, enabled bool, minutes int) error {
	leak["enabled"] = enabled
	if !enabled {
		return nil
	}
	if minutes <= 0 {
		return fmt.Errorf("memory-leak warm-up must be greater than zero")
	}
	leak["warmup_seconds"] = minutes * 60
	return nil
}

func resolveMonitorInterpreter(requested string) (monitorInterpreter, error) {
	type candidate struct {
		path        string
		environment string
	}
	candidates := []candidate{}
	if requested != "" {
		candidates = append(candidates, candidate{path: requested, environment: filepath.Base(filepath.Dir(filepath.Dir(requested)))})
	} else {
		for _, variable := range []string{"VIRTUAL_ENV", "CONDA_PREFIX"} {
			if prefix := os.Getenv(variable); prefix != "" {
				candidates = append(candidates, candidate{path: filepath.Join(prefix, "bin", "python3"), environment: filepath.Base(prefix)})
			}
		}
		if path, err := exec.LookPath("python3"); err == nil {
			candidates = append(candidates, candidate{path: path, environment: "system"})
		}
	}
	for _, candidate := range candidates {
		absolute, err := filepath.Abs(candidate.path)
		if err != nil || !regularExecutable(absolute) || !supportsPythonVersion(absolute, nil, "sys.version_info[0] == 3") {
			continue
		}
		environment := candidate.environment
		if environment == "." || environment == string(filepath.Separator) {
			environment = "system"
		}
		return monitorInterpreter{Path: absolute, Environment: environment}, nil
	}
	return monitorInterpreter{}, fmt.Errorf("no Python 3 interpreter is available")
}

func regularExecutable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0
}

func validateMonitorTargets(arguments []string) ([]string, error) {
	if len(arguments) == 0 {
		return nil, fmt.Errorf("at least one Python script is required")
	}
	targets := make([]string, 0, len(arguments))
	for _, argument := range arguments {
		absolute, err := filepath.Abs(argument)
		if err != nil {
			return nil, fmt.Errorf("resolve target %s: %w", argument, err)
		}
		info, err := os.Stat(absolute)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o444 == 0 || strings.ToLower(filepath.Ext(absolute)) != ".py" {
			return nil, fmt.Errorf("target must be a readable regular .py file: %s", argument)
		}
		targets = append(targets, absolute)
	}
	return targets, nil
}

var monitorControlSequence = regexp.MustCompile(`(?:\x1b\[[0-?]*[ -/]*[@-~])|[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]`)

func sanitizeMonitorLines(value string, limit int) []string {
	clean := monitorControlSequence.ReplaceAllString(value, "")
	lines := strings.Split(strings.ReplaceAll(clean, "\r\n", "\n"), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if limit >= 0 && len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}
	return lines
}
