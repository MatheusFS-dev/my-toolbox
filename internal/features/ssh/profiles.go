package ssh

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const SSHIncludeBlock = "# BEGIN ssh-tools\nInclude ~/.ssh/ssh-tools/config.d/*.conf\n# END ssh-tools\n"

var sshAliasPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

// SSHProfile describes a toolbox-managed OpenSSH connection.
type SSHProfile struct {
	Alias        string
	Hostname     string
	User         string
	Port         int
	IdentityPath string
	Health       string
}

func ValidSSHAlias(alias string) bool { return sshAliasPattern.MatchString(alias) }

func HasSSHHostCollision(config, alias string) bool {
	for _, line := range strings.Split(config, "\n") {
		fields := strings.Fields(line)
		if len(fields) > 1 && strings.EqualFold(fields[0], "Host") {
			for _, host := range fields[1:] {
				if strings.EqualFold(host, alias) {
					return true
				}
			}
		}
	}
	return false
}

func SecureSSHPath(path string, directory bool) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing symlink %s", path)
	}
	if directory && !info.IsDir() {
		return fmt.Errorf("%s is not a directory", path)
	}
	if !directory && !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", path)
	}
	if info.Mode().Perm()&0022 != 0 {
		return fmt.Errorf("%s is writable by group or others", path)
	}
	return nil
}

func SecureSSHPrivateKey(path string) error {
	if err := SecureSSHPath(path, false); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("private key %s is accessible by group or others", path)
	}
	return nil
}

func SSHPaths(home string) (string, string, string, string) {
	ssh := filepath.Join(home, ".ssh")
	base := filepath.Join(ssh, "ssh-tools")
	return ssh, base, filepath.Join(base, "config.d"), filepath.Join(base, "keys")
}

func EnsureSSHDirectories(home string) error {
	ssh, base, configs, keys := SSHPaths(home)
	for _, path := range []string{ssh, base, configs, keys} {
		if err := SecureSSHPath(path, true); err != nil {
			return err
		}
		if err := os.Mkdir(path, 0700); err != nil && !os.IsExist(err) {
			return err
		}
		if err := os.Chmod(path, 0700); err != nil {
			return err
		}
	}
	return nil
}

func AtomicSSHWrite(path string, data []byte, backup bool) error {
	if err := SecureSSHPath(path, false); err != nil {
		return err
	}
	if backup {
		if current, err := os.ReadFile(path); err == nil {
			name := path + ".bak-" + time.Now().Format("20060102-150405.000000000")
			file, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
			if err != nil {
				return err
			}
			if _, err := file.Write(current); err != nil {
				file.Close()
				return err
			}
			if err := file.Close(); err != nil {
				return err
			}
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".ssh-tools-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err := file.Chmod(0600); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

func ValidateSSHProfile(profile SSHProfile) error {
	if !ValidSSHAlias(profile.Alias) {
		return fmt.Errorf("invalid SSH alias %q", profile.Alias)
	}
	for _, value := range []string{profile.Hostname, profile.User} {
		if value == "" || strings.ContainsAny(value, " \t\r\n#\\\"'") || strings.HasPrefix(value, "-") {
			return fmt.Errorf("invalid hostname or user")
		}
	}
	if profile.Port < 1 || profile.Port > 65535 {
		return fmt.Errorf("SSH port must be 1 through 65535")
	}
	return nil
}

func SaveSSHProfile(home string, profile SSHProfile) error {
	if err := ValidateSSHProfile(profile); err != nil {
		return err
	}
	if err := CheckSSHProfileDestination(home, profile); err != nil {
		return err
	}
	if err := EnsureSSHDirectories(home); err != nil {
		return err
	}
	ssh, _, configs, keys := SSHPaths(home)
	rootConfig := filepath.Join(ssh, "config")
	if err := SecureSSHPath(rootConfig, false); err != nil {
		return err
	}
	config, err := os.ReadFile(rootConfig)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	text := string(config)
	if strings.Contains(text, "# BEGIN ssh-tools") && !strings.HasPrefix(text, SSHIncludeBlock) {
		return fmt.Errorf("damaged ssh-tools include block")
	}
	if !strings.HasPrefix(text, SSHIncludeBlock) {
		if HasSSHHostCollision(text, profile.Alias) {
			return fmt.Errorf("unmanaged Host %q already exists", profile.Alias)
		}
		text = SSHIncludeBlock + text
	} else if HasSSHHostCollision(strings.TrimPrefix(text, SSHIncludeBlock), profile.Alias) {
		return fmt.Errorf("unmanaged Host %q already exists", profile.Alias)
	}
	key := filepath.Join(keys, profile.Alias)
	if err := SecureSSHPrivateKey(key); err != nil {
		return err
	}
	if err := SecureSSHPath(filepath.Join(configs, profile.Alias+".conf"), false); err != nil {
		return err
	}
	if err := AtomicSSHWrite(rootConfig, []byte(text), true); err != nil {
		return err
	}
	body := fmt.Sprintf("Host %s\n    HostName %s\n    User %s\n    Port %d\n    IdentityFile %s\n    IdentitiesOnly yes\n", profile.Alias, profile.Hostname, profile.User, profile.Port, strconv.Quote(key))
	return AtomicSSHWrite(filepath.Join(configs, profile.Alias+".conf"), []byte(body), true)
}

func CheckSSHProfileDestination(home string, profile SSHProfile) error {
	ssh, base, configs, keys := SSHPaths(home)
	for _, path := range []string{ssh, base, configs, keys} {
		if err := SecureSSHPath(path, true); err != nil {
			return err
		}
	}
	rootConfig := filepath.Join(ssh, "config")
	if err := SecureSSHPath(rootConfig, false); err != nil {
		return err
	}
	data, err := os.ReadFile(rootConfig)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	config := string(data)
	if strings.Contains(config, "# BEGIN ssh-tools") && !strings.HasPrefix(config, SSHIncludeBlock) {
		return fmt.Errorf("damaged ssh-tools include block")
	}
	if HasSSHHostCollision(strings.TrimPrefix(config, SSHIncludeBlock), profile.Alias) {
		return fmt.Errorf("unmanaged Host %q already exists", profile.Alias)
	}
	if err := SecureSSHPrivateKey(filepath.Join(keys, profile.Alias)); err != nil {
		return err
	}
	if err := SecureSSHPath(filepath.Join(keys, profile.Alias+".pub"), false); err != nil {
		return err
	}
	return SecureSSHPath(filepath.Join(configs, profile.Alias+".conf"), false)
}

func LoadSSHProfiles(home string) ([]SSHProfile, error) {
	ssh, base, configs, keys := SSHPaths(home)
	for _, path := range []string{ssh, base, configs, keys} {
		if err := SecureSSHPath(path, true); err != nil {
			return nil, err
		}
	}
	if err := SecureSSHPath(filepath.Join(ssh, "config"), false); err != nil {
		return nil, err
	}
	if err := SecureSSHPath(configs, true); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(configs)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	profiles := []SSHProfile{}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".conf") {
			continue
		}
		alias := strings.TrimSuffix(entry.Name(), ".conf")
		if !ValidSSHAlias(alias) {
			return nil, fmt.Errorf("invalid profile filename %q", entry.Name())
		}
		path := filepath.Join(configs, entry.Name())
		if err := SecureSSHPath(path, false); err != nil {
			return nil, err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		profile, err := ParseManagedSSHProfile(alias, string(data), filepath.Join(keys, alias))
		if err != nil {
			return nil, err
		}
		if err := ValidateSSHProfile(profile); err != nil {
			return nil, err
		}
		if profile.IdentityPath != filepath.Join(keys, alias) {
			return nil, fmt.Errorf("unexpected key path in profile %q", alias)
		}
		if err := SecureSSHPrivateKey(profile.IdentityPath); err != nil {
			return nil, err
		}
		if _, err := os.Stat(profile.IdentityPath); os.IsNotExist(err) {
			profile.Health = "missing key"
		} else if err != nil {
			return nil, err
		} else {
			publicKey := profile.IdentityPath + ".pub"
			if err := SecureSSHPath(publicKey, false); err != nil {
				return nil, err
			}
			if _, err := os.Stat(publicKey); os.IsNotExist(err) {
				profile.Health = "missing public key"
			} else if err != nil {
				return nil, err
			} else {
				command := exec.Command("ssh-keygen", "-y", "-P", "", "-f", profile.IdentityPath)
				command.Stdout, command.Stderr = io.Discard, io.Discard
				if command.Run() != nil {
					profile.Health = "damaged key"
				} else {
					profile.Health = "saved"
				}
			}
		}
		profiles = append(profiles, profile)
	}
	sort.Slice(profiles, func(i, j int) bool { return profiles[i].Alias < profiles[j].Alias })
	return profiles, nil
}

func ParseManagedSSHProfile(alias, content, expectedKey string) (SSHProfile, error) {
	profile := SSHProfile{Alias: alias}
	seen := map[string]bool{}
	for _, line := range strings.Split(content, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		key := strings.ToLower(fields[0])
		if seen[key] {
			return SSHProfile{}, fmt.Errorf("duplicate %s in profile %q", key, alias)
		}
		seen[key] = true
		value := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), fields[0]))
		switch key {
		case "host":
			if value != alias {
				return SSHProfile{}, fmt.Errorf("Host mismatch in profile %q", alias)
			}
		case "hostname":
			profile.Hostname = value
		case "user":
			profile.User = value
		case "port":
			parsed, err := strconv.Atoi(value)
			if err != nil {
				return SSHProfile{}, fmt.Errorf("invalid port in profile %q", alias)
			}
			profile.Port = parsed
		case "identityfile":
			if strings.HasPrefix(value, "\"") {
				parsed, err := strconv.Unquote(value)
				if err != nil {
					return SSHProfile{}, fmt.Errorf("invalid identity path in profile %q: %w", alias, err)
				}
				value = parsed
			}
			profile.IdentityPath = value
		case "identitiesonly":
			if !strings.EqualFold(value, "yes") {
				return SSHProfile{}, fmt.Errorf("IdentitiesOnly must be yes in profile %q", alias)
			}
		default:
			return SSHProfile{}, fmt.Errorf("unsupported directive %q in profile %q", fields[0], alias)
		}
	}
	for _, key := range []string{"host", "hostname", "user", "port", "identityfile", "identitiesonly"} {
		if !seen[key] {
			return SSHProfile{}, fmt.Errorf("missing %s in profile %q", key, alias)
		}
	}
	if err := ValidateSSHProfile(profile); err != nil {
		return SSHProfile{}, err
	}
	if profile.IdentityPath != expectedKey {
		return SSHProfile{}, fmt.Errorf("unexpected key path in profile %q", alias)
	}
	return profile, nil
}

func RemoveSSHProfile(home, alias string) error {
	if !ValidSSHAlias(alias) {
		return fmt.Errorf("invalid SSH alias")
	}
	_, _, configs, keys := SSHPaths(home)
	for _, path := range []string{filepath.Join(configs, alias+".conf"), filepath.Join(keys, alias), filepath.Join(keys, alias+".pub")} {
		if err := SecureSSHPath(path, false); err != nil {
			return err
		}
	}
	for _, path := range []string{filepath.Join(configs, alias+".conf"), filepath.Join(keys, alias), filepath.Join(keys, alias+".pub")} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
