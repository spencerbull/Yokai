package ssh

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseSSHConfig(t *testing.T) {
	content := `# Global defaults
Host *
  ServerAliveInterval 60

Host devbox
  HostName 192.168.1.100
  User alice
  Port 2222
  IdentityFile ~/.ssh/id_ed25519

Host gpu-server
  HostName gpu.example.com
  User root

# Wildcard entry should be skipped
Host *.internal
  User deploy
`

	path := writeTempFile(t, content)
	hosts, err := ParseSSHConfig(path)
	if err != nil {
		t.Fatalf("ParseSSHConfig: %v", err)
	}

	if len(hosts) != 2 {
		t.Fatalf("expected 2 hosts, got %d", len(hosts))
	}

	// First host: devbox
	h := hosts[0]
	if h.Alias != "devbox" {
		t.Errorf("alias: got %q, want %q", h.Alias, "devbox")
	}
	if h.HostName != "192.168.1.100" {
		t.Errorf("hostname: got %q, want %q", h.HostName, "192.168.1.100")
	}
	if h.User != "alice" {
		t.Errorf("user: got %q, want %q", h.User, "alice")
	}
	if h.Port != "2222" {
		t.Errorf("port: got %q, want %q", h.Port, "2222")
	}
	if h.IdentityFile == "" {
		t.Error("identity file should not be empty")
	}

	// Second host: gpu-server
	h = hosts[1]
	if h.Alias != "gpu-server" {
		t.Errorf("alias: got %q, want %q", h.Alias, "gpu-server")
	}
	if h.HostName != "gpu.example.com" {
		t.Errorf("hostname: got %q, want %q", h.HostName, "gpu.example.com")
	}
	if h.User != "root" {
		t.Errorf("user: got %q, want %q", h.User, "root")
	}
	if h.Port != "" {
		t.Errorf("port: got %q, want empty", h.Port)
	}
}

func TestParseSSHConfig_Empty(t *testing.T) {
	path := writeTempFile(t, "")
	hosts, err := ParseSSHConfig(path)
	if err != nil {
		t.Fatalf("ParseSSHConfig: %v", err)
	}
	if len(hosts) != 0 {
		t.Fatalf("expected 0 hosts, got %d", len(hosts))
	}
}

func TestParseSSHConfig_OnlyWildcards(t *testing.T) {
	content := `Host *
  User root

Host *.example.com
  Port 22
`
	path := writeTempFile(t, content)
	hosts, err := ParseSSHConfig(path)
	if err != nil {
		t.Fatalf("ParseSSHConfig: %v", err)
	}
	if len(hosts) != 0 {
		t.Fatalf("expected 0 hosts, got %d", len(hosts))
	}
}

func TestParseSSHConfig_EqualsSign(t *testing.T) {
	content := `Host myhost
  HostName=10.0.0.1
  User=deploy
  Port=3333
`
	path := writeTempFile(t, content)
	hosts, err := ParseSSHConfig(path)
	if err != nil {
		t.Fatalf("ParseSSHConfig: %v", err)
	}
	if len(hosts) != 1 {
		t.Fatalf("expected 1 host, got %d", len(hosts))
	}
	h := hosts[0]
	if h.HostName != "10.0.0.1" {
		t.Errorf("hostname: got %q, want %q", h.HostName, "10.0.0.1")
	}
	if h.User != "deploy" {
		t.Errorf("user: got %q, want %q", h.User, "deploy")
	}
	if h.Port != "3333" {
		t.Errorf("port: got %q, want %q", h.Port, "3333")
	}
}

func TestParseSSHConfig_MissingFile(t *testing.T) {
	_, err := ParseSSHConfig("/nonexistent/ssh/config")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func writeTempFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}
