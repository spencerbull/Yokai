package ssh

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
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
	Host          string
	Port          string
	User          string
	KeyPath       string // path to private key, empty to try defaults
	KeyPassphrase string // passphrase for encrypted private key
	Password      string // fallback password auth
}

// Client wraps an SSH connection.
type Client struct {
	conn   *ssh.Client
	config ClientConfig
}

// Connect establishes an SSH connection using the provided config.
// Auth resolution order: SSH agent → explicit key → default keys → password.
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
			hostKeyCallback = tolerateKnownHostsKeyTypeMismatch(cb)
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

func tolerateKnownHostsKeyTypeMismatch(cb ssh.HostKeyCallback) ssh.HostKeyCallback {
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		err := cb(hostname, remote, key)
		if err == nil {
			return nil
		}

		var keyErr *knownhosts.KeyError
		if !errors.As(err, &keyErr) {
			return err
		}

		if len(keyErr.Want) == 0 {
			return err
		}

		for _, known := range keyErr.Want {
			if known.Key != nil && bytes.Equal(known.Key.Marshal(), key.Marshal()) {
				return nil
			}
		}

		return nil
	}
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
	defer func() {
		_ = session.Close() // Best-effort session close after command.
	}()

	out, err := session.CombinedOutput(cmd)
	return string(out), err
}

// Upload copies a local file to a remote path via SCP.
func (c *Client) Upload(localPath, remotePath string) error {
	f, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("opening local file: %w", err)
	}
	defer func() {
		_ = f.Close() // Best-effort file close after upload.
	}()

	stat, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat local file: %w", err)
	}

	session, err := c.conn.NewSession()
	if err != nil {
		return fmt.Errorf("creating session: %w", err)
	}
	defer func() {
		_ = session.Close() // Best-effort session close after upload.
	}()

	go func() {
		w, _ := session.StdinPipe()
		defer func() {
			_ = w.Close() // Best-effort close of SCP stdin pipe.
		}()
		_, _ = fmt.Fprintf(w, "C0755 %d %s\n", stat.Size(), filepath.Base(remotePath))
		_, _ = io.Copy(w, f)
		_, _ = fmt.Fprint(w, "\x00")
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
// SSH agent is tried first because it likely already holds unlocked keys,
// which avoids passphrase prompts for encrypted private keys.
func resolveAuth(cfg ClientConfig) ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod

	// 1. SSH agent (highest priority — handles passphrase-protected keys transparently)
	if m, err := agentAuth(); err == nil {
		methods = append(methods, m)
	}

	// 2. Explicit key path
	if cfg.KeyPath != "" {
		m, err := keyAuth(expandPath(cfg.KeyPath), cfg.KeyPassphrase)
		if err != nil {
			var passErr *ssh.PassphraseMissingError
			if errors.As(err, &passErr) {
				log.Printf("SSH key %s is encrypted with a passphrase; skipping (add it to ssh-agent with ssh-add)", cfg.KeyPath)
			}
		} else {
			methods = append(methods, m)
		}
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
		m, err := keyAuth(p, "")
		if err != nil {
			var passErr *ssh.PassphraseMissingError
			if errors.As(err, &passErr) {
				log.Printf("SSH key %s is encrypted with a passphrase; skipping (add it to ssh-agent with ssh-add)", k)
			}
			continue
		}
		methods = append(methods, m)
	}

	// 4. Password
	if cfg.Password != "" {
		methods = append(methods, ssh.Password(cfg.Password))
	}

	if len(methods) == 0 {
		return nil, fmt.Errorf("no SSH auth methods available (if your key is passphrase-protected, use ssh-agent: eval $(ssh-agent) && ssh-add)")
	}

	return methods, nil
}

func keyAuth(path, passphrase string) (ssh.AuthMethod, error) {
	key, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var signer ssh.Signer
	if passphrase != "" {
		signer, err = ssh.ParsePrivateKeyWithPassphrase(key, []byte(passphrase))
	} else {
		signer, err = ssh.ParsePrivateKey(key)
	}
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

// IsKeyEncrypted checks whether an SSH private key file is passphrase-protected.
// Returns true if the key exists and requires a passphrase to decrypt.
func IsKeyEncrypted(path string) bool {
	path = expandPath(path)
	key, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	_, err = ssh.ParsePrivateKey(key)
	if err != nil {
		var passErr *ssh.PassphraseMissingError
		return errors.As(err, &passErr)
	}
	return false
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
