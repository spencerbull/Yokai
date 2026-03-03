package ssh

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

// ClientConfig holds SSH connection parameters.
type ClientConfig struct {
	Host     string
	Port     string
	User     string
	KeyPath  string // path to private key, empty to try defaults
	Password string // fallback password auth
}

// Client wraps an SSH connection.
type Client struct {
	conn   *ssh.Client
	config ClientConfig
}

// Connect establishes an SSH connection using the provided config.
// Auth resolution order: explicit key → SSH agent → default keys → password.
func Connect(cfg ClientConfig) (*Client, error) {
	if cfg.Port == "" {
		cfg.Port = "22"
	}
	if cfg.User == "" {
		cfg.User = currentUser()
	}

	authMethods, err := resolveAuth(cfg)
	if err != nil {
		return nil, fmt.Errorf("resolving SSH auth: %w", err)
	}

	// Build known_hosts callback (permissive if file doesn't exist)
	hostKeyCallback := ssh.InsecureIgnoreHostKey()
	knownHostsPath := expandPath("~/.ssh/known_hosts")
	if _, err := os.Stat(knownHostsPath); err == nil {
		cb, err := knownhosts.New(knownHostsPath)
		if err == nil {
			hostKeyCallback = cb
		}
	}

	sshConfig := &ssh.ClientConfig{
		User:            cfg.User,
		Auth:            authMethods,
		HostKeyCallback: hostKeyCallback,
		Timeout:         10 * time.Second,
	}

	addr := net.JoinHostPort(cfg.Host, cfg.Port)
	conn, err := ssh.Dial("tcp", addr, sshConfig)
	if err != nil {
		return nil, fmt.Errorf("SSH dial %s: %w", addr, err)
	}

	return &Client{conn: conn, config: cfg}, nil
}

// Close closes the SSH connection.
func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// Exec runs a command on the remote host and returns combined output.
func (c *Client) Exec(cmd string) (string, error) {
	session, err := c.conn.NewSession()
	if err != nil {
		return "", fmt.Errorf("creating session: %w", err)
	}
	defer session.Close()

	out, err := session.CombinedOutput(cmd)
	return string(out), err
}

// Upload copies a local file to a remote path via SCP.
func (c *Client) Upload(localPath, remotePath string) error {
	f, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("opening local file: %w", err)
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat local file: %w", err)
	}

	session, err := c.conn.NewSession()
	if err != nil {
		return fmt.Errorf("creating session: %w", err)
	}
	defer session.Close()

	go func() {
		w, _ := session.StdinPipe()
		defer w.Close()
		fmt.Fprintf(w, "C0755 %d %s\n", stat.Size(), filepath.Base(remotePath))
		io.Copy(w, f)
		fmt.Fprint(w, "\x00")
	}()

	dir := filepath.Dir(remotePath)
	err = session.Run(fmt.Sprintf("scp -t %s", dir))
	if err != nil {
		return fmt.Errorf("SCP upload: %w", err)
	}

	return nil
}

// Underlying returns the raw ssh.Client for advanced use (e.g. tunneling).
func (c *Client) Underlying() *ssh.Client {
	return c.conn
}

// resolveAuth builds auth methods in priority order.
func resolveAuth(cfg ClientConfig) ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod

	// 1. Explicit key path
	if cfg.KeyPath != "" {
		if m, err := keyAuth(expandPath(cfg.KeyPath)); err == nil {
			methods = append(methods, m)
		}
	}

	// 2. SSH agent
	if m, err := agentAuth(); err == nil {
		methods = append(methods, m)
	}

	// 3. Default key paths
	defaultKeys := []string{
		"~/.ssh/id_ed25519",
		"~/.ssh/id_rsa",
	}
	for _, k := range defaultKeys {
		p := expandPath(k)
		if p == expandPath(cfg.KeyPath) {
			continue // already tried
		}
		if m, err := keyAuth(p); err == nil {
			methods = append(methods, m)
		}
	}

	// 4. Password
	if cfg.Password != "" {
		methods = append(methods, ssh.Password(cfg.Password))
	}

	if len(methods) == 0 {
		return nil, fmt.Errorf("no SSH auth methods available")
	}

	return methods, nil
}

func keyAuth(path string) (ssh.AuthMethod, error) {
	key, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, err
	}
	return ssh.PublicKeys(signer), nil
}

func agentAuth() (ssh.AuthMethod, error) {
	sock := os.Getenv("SSH_AUTH_SOCK")
	if sock == "" {
		return nil, fmt.Errorf("SSH_AUTH_SOCK not set")
	}
	conn, err := net.Dial("unix", sock)
	if err != nil {
		return nil, err
	}
	agentClient := agent.NewClient(conn)
	return ssh.PublicKeysCallback(agentClient.Signers), nil
}

func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

func currentUser() string {
	if u := os.Getenv("USER"); u != "" {
		return u
	}
	if u := os.Getenv("LOGNAME"); u != "" {
		return u
	}
	return "root"
}
