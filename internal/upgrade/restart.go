package upgrade

import (
	"debug/buildinfo"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spencerbull/yokai/internal/config"
)

const (
	defaultDaemonAddr = "127.0.0.1:7473"
	daemonPidFile     = "daemon.pid"
	yokaiMainPackage  = "github.com/spencerbull/yokai/cmd/yokai"
)

func isYokaiBuild(info *buildinfo.BuildInfo) bool {
	return info != nil && info.Path == yokaiMainPackage
}

func hasDaemonSubcommand(args []string) bool {
	return len(args) >= 2 && args[1] == "daemon"
}

func commandLooksLikeYokaiDaemon(comm, args string) bool {
	name := strings.TrimSuffix(strings.ToLower(filepath.Base(strings.TrimSpace(comm))), ".exe")
	if name != "yokai" {
		return false
	}
	return hasDaemonSubcommand(strings.Fields(args))
}

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

// daemonPIDMatchesHealth reports whether pid is the process identifying itself
// through the configured daemon listener. This gives platforms without /proc a
// safe ownership check while still rejecting stale pidfiles and unrelated
// health endpoints that do not implement Yokai's process identity response.
func daemonPIDMatchesHealth(pid int, addr string) bool {
	if pid <= 0 {
		return false
	}
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://" + addr + "/health")
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	var health struct {
		Status string `json:"status"`
		PID    int    `json:"pid"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		return false
	}
	return health.Status == "ok" && health.PID > 0 && health.PID == pid
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
	return platformProcessAlive(pid)
}

// statIndicatesZombie reports whether a ps/proc process-status token denotes a
// zombie (process exited but not yet reaped).
func statIndicatesZombie(stat string) bool {
	return strings.Contains(stat, "Z")
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

	// A daemon may have written its PID file but not yet reached
	// ListenAndServe (startup does config/Tailscale/store work after that), so
	// health alone can miss a running daemon. Resolve a live PID first — but
	// validate ownership so a stale pidfile left by a SIGKILL-crashed daemon
	// (whose deferred removal never ran) is never trusted if the PID was
	// recycled by an unrelated process.
	livePID := readPidFile()
	pidFileOwned := livePID > 0 && ((pidAlive(livePID) && daemonPIDLooksOwned(livePID, addr)) || daemonPIDMatchesHealth(livePID, addr))
	if !pidFileOwned {
		livePID = 0
		if pid, ok := findDaemonPIDByPort(addr); ok {
			livePID = pid
		}
	}

	if livePID <= 0 && !daemonHealthy(addr) {
		fmt.Println("No running daemon detected; nothing to restart.")
		return nil
	}

	fmt.Println("Restarting daemon with the new version...")

	stopped := false
	if livePID > 0 {
		platformTerminate(livePID)
		stopped = waitForExit(livePID, 8*time.Second)
	}
	if !stopped {
		return fmt.Errorf("a daemon is running but its process could not be located/stopped; stop it and run %q manually", currentBinaryPath)
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
