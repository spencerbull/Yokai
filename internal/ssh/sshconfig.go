package ssh

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// SSHHost represents a single Host entry parsed from ~/.ssh/config.
type SSHHost struct {
	Alias        string // the Host alias (e.g. "myserver")
	HostName     string // resolved hostname or IP
	Port         string // SSH port (empty means default 22)
	User         string // SSH user
	IdentityFile string // path to private key
}

// ParseSSHConfig reads an SSH config file and returns all non-wildcard Host entries.
func ParseSSHConfig(path string) ([]SSHHost, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var hosts []SSHHost
	var current *SSHHost

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Split into keyword and argument(s)
		key, val := splitDirective(line)
		if key == "" {
			continue
		}

		switch strings.ToLower(key) {
		case "host":
			// Flush previous host
			if current != nil && !isWildcard(current.Alias) {
				hosts = append(hosts, *current)
			}
			current = &SSHHost{Alias: val}
		case "hostname":
			if current != nil {
				current.HostName = val
			}
		case "port":
			if current != nil {
				current.Port = val
			}
		case "user":
			if current != nil {
				current.User = val
			}
		case "identityfile":
			if current != nil {
				current.IdentityFile = expandTilde(val)
			}
		}
	}

	// Flush last entry
	if current != nil && !isWildcard(current.Alias) {
		hosts = append(hosts, *current)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return hosts, nil
}

// DiscoverSSHHosts tries to parse the default ~/.ssh/config and returns
// the discovered hosts. Returns nil (no error) if the file doesn't exist.
func DiscoverSSHHosts() []SSHHost {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	hosts, err := ParseSSHConfig(filepath.Join(home, ".ssh", "config"))
	if err != nil {
		return nil
	}
	return hosts
}

// splitDirective splits "Key Value" or "Key=Value" into (key, value).
func splitDirective(line string) (string, string) {
	// Handle "Key=Value"
	if idx := strings.IndexByte(line, '='); idx > 0 {
		before := strings.TrimSpace(line[:idx])
		after := strings.TrimSpace(line[idx+1:])
		if !strings.ContainsAny(before, " \t") {
			return before, after
		}
	}
	// Handle "Key Value"
	fields := strings.SplitN(line, " ", 2)
	if len(fields) < 2 {
		fields = strings.SplitN(line, "\t", 2)
	}
	if len(fields) < 2 {
		return fields[0], ""
	}
	return strings.TrimSpace(fields[0]), strings.TrimSpace(fields[1])
}

func isWildcard(alias string) bool {
	return strings.Contains(alias, "*") || strings.Contains(alias, "?")
}

func expandTilde(path string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}
