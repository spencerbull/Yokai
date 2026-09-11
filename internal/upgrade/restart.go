package upgrade

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spencerbull/yokai/internal/config"
)

const (
	defaultDaemonAddr = "127.0.0.1:7473"
	daemonPidFile     = "daemon.pid"
)

// daemonAddr resolves the daemon listen address from config, defaulting to the
// standard loopback address when unset.
func daemonAddr() (string, error) {
	cfg, err := config.Load()
	if err != nil {
		return "", fmt.Errorf("loading config to resolve daemon address: %w", err)
	}
	if cfg.Daemon.Listen == "" {
		return defaultDaemonAddr, nil
	}
	return cfg.Daemon.Listen, nil
}

// daemonHealthy reports whether a daemon answers /health on addr.
func daemonHealthy(addr string) bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://" + addr + "/health")
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode == http.StatusOK
}

// waitForDaemon polls addr until it answers /health or the timeout elapses.
func waitForDaemon(addr string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if daemonHealthy(addr) {
			return true
		}
		time.Sleep(250 * time.Millisecond)
	}
	return daemonHealthy(addr)
}

// readPidFile returns the PID the daemon wrote, or 0 when absent/invalid.
func readPidFile() int {
	dir, err := config.ConfigDir()
	if err != nil {
		return 0
	}
	data, err := os.ReadFile(filepath.Join(dir, daemonPidFile))
	if err != nil {
		return 0
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	return pid
}

// pidAlive reports whether a process with the given PID exists.
func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

// waitForExit blocks until pid is gone or the timeout elapses.
func waitForExit(pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !pidAlive(pid) {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return !pidAlive(pid)
}

// restartRunningDaemon stops any daemon currently listening on addr and
// relaunches it from currentBinaryPath so a fresh install takes effect without
// a manual restart. It is a no-op when no daemon is running.
func restartRunningDaemon(currentBinaryPath string) error {
	addr, err := daemonAddr()
	if err != nil {
		return err
	}
	if !daemonHealthy(addr) {
		fmt.Println("No running daemon detected; nothing to restart.")
		return nil
	}

	fmt.Println("Restarting daemon with the new version...")

	stopped := false
	if pid := readPidFile(); pid > 0 && pidAlive(pid) {
		_ = syscall.Kill(pid, syscall.SIGTERM)
		stopped = waitForExit(pid, 8*time.Second)
	}
	if !stopped {
		// Fall back to locating the listener by port (covers daemons started
		// before PID files existed).
		if pid, ok := findDaemonPIDByPort(addr); ok {
			_ = syscall.Kill(pid, syscall.SIGTERM)
			stopped = waitForExit(pid, 8*time.Second)
		}
	}
	if !stopped {
		return fmt.Errorf("a daemon is running but its process could not be located; stop it and run %q manually", currentBinaryPath)
	}

	if err := startDaemonProcess(currentBinaryPath); err != nil {
		return err
	}
	if !waitForDaemon(addr, 10*time.Second) {
		return fmt.Errorf("daemon did not become healthy after restart (see %s)", daemonLogPath())
	}
	fmt.Println("✅ Daemon restarted with the new version.")
	return nil
}

// startDaemonProcess launches the daemon detached, mirroring the TUI launcher.
func startDaemonProcess(binaryPath string) error {
	logFile, err := daemonLogFile()
	if err != nil {
		return err
	}

	cmd := exec.Command(binaryPath, "daemon")
	cmd.Stdin = nil
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return fmt.Errorf("starting daemon: %w", err)
	}
	if cmd.Process != nil {
		_ = cmd.Process.Release()
	}
	_ = logFile.Close()
	return nil
}

// daemonLogPath returns the config-dir daemon log path the new process writes.
func daemonLogPath() string {
	if dir, err := config.ConfigDir(); err == nil {
		return filepath.Join(dir, "daemon.log")
	}
	return "daemon.log"
}

func daemonLogFile() (*os.File, error) {
	dir, err := config.ConfigDir()
	if err != nil {
		return nil, fmt.Errorf("resolving config dir: %w", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("creating config dir: %w", err)
	}
	logPath := filepath.Join(dir, "daemon.log")
	file, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("opening daemon log: %w", err)
	}
	return file, nil
}
