package opentui

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/spencerbull/yokai/internal/config"
)

const (
	defaultDaemonAddr = "127.0.0.1:7473"
	tuiBinaryName     = "yokai-tui"
)

// Run launches the packaged OpenTUI client, auto-starting the daemon first.
// It falls back to source-mode Bun execution in a checkout when no bundled
// OpenTUI sidecar is available.
func Run(version string) error {
	_ = version

	daemonURL := os.Getenv("YOKAI_DAEMON_URL")
	if daemonURL == "" {
		addr, err := ensureDaemonRunning()
		if err != nil {
			return err
		}
		daemonURL = "http://" + addr
	}

	if cmd, ok, err := bundledTUICommand(daemonURL); err != nil {
		return err
	} else if ok {
		return runCommand(cmd)
	}

	if cmd, ok := sourceTUICommand(daemonURL); ok {
		return runCommand(cmd)
	}

	return fmt.Errorf("OpenTUI runtime not found: install the bundled yokai-tui sidecar or run from a checkout with Bun available")
}

func ensureDaemonRunning() (string, error) {
	addr, err := daemonAddr()
	if err != nil {
		return "", err
	}

	if daemonHealthy(addr) {
		return addr, nil
	}

	if err := startDaemonProcess(); err != nil {
		return "", err
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if daemonHealthy(addr) {
			return addr, nil
		}
		time.Sleep(150 * time.Millisecond)
	}

	return "", fmt.Errorf("starting local daemon: health check timed out")
}

func daemonAddr() (string, error) {
	cfg, err := config.Load()
	if err != nil {
		return "", fmt.Errorf("loading config: %w", err)
	}
	if cfg.Daemon.Listen == "" {
		return defaultDaemonAddr, nil
	}
	return cfg.Daemon.Listen, nil
}

func daemonHealthy(addr string) bool {
	client := &http.Client{Timeout: 300 * time.Millisecond}
	resp, err := client.Get("http://" + addr + "/health")
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode == http.StatusOK
}

func startDaemonProcess() error {
	binaryPath, err := executablePath()
	if err != nil {
		return err
	}

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
		return fmt.Errorf("starting local daemon: %w", err)
	}

	if cmd.Process != nil {
		_ = cmd.Process.Release()
	}
	_ = logFile.Close()
	return nil
}

func daemonLogFile() (*os.File, error) {
	configDir, err := config.ConfigDir()
	if err != nil {
		return nil, fmt.Errorf("resolving config dir: %w", err)
	}
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return nil, fmt.Errorf("creating config dir: %w", err)
	}
	logPath := filepath.Join(configDir, "daemon.log")
	file, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return nil, fmt.Errorf("opening daemon log: %w", err)
	}
	return file, nil
}

func bundledTUICommand(daemonURL string) (*exec.Cmd, bool, error) {
	binaryPath, err := executablePath()
	if err != nil {
		return nil, false, err
	}

	tuiPath := filepath.Join(filepath.Dir(binaryPath), sidecarBinaryName())
	if _, err := os.Stat(tuiPath); err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("checking yokai-tui runtime: %w", err)
	}

	cmd := exec.Command(tuiPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "YOKAI_DAEMON_URL="+daemonURL)
	return cmd, true, nil
}

func sourceTUICommand(daemonURL string) (*exec.Cmd, bool) {
	if _, err := exec.LookPath("bun"); err != nil {
		return nil, false
	}

	for _, dir := range sourceTUICandidates() {
		packageJSON := filepath.Join(dir, "package.json")
		if _, err := os.Stat(packageJSON); err != nil {
			continue
		}

		cmd := exec.Command("bun", "run", "src/index.tsx")
		cmd.Dir = dir
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Env = append(os.Environ(), "YOKAI_DAEMON_URL="+daemonURL)
		return cmd, true
	}

	return nil, false
}

func sourceTUICandidates() []string {
	candidates := make([]string, 0, 3)
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(cwd, "ui", "tui"))
	}
	if _, file, _, ok := runtime.Caller(0); ok {
		candidates = append(candidates, filepath.Join(filepath.Dir(file), "..", "..", "ui", "tui"))
	}
	if binaryPath, err := executablePath(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(binaryPath), "..", "ui", "tui"))
	}
	return candidates
}

func executablePath() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolving executable path: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("resolving executable symlink: %w", err)
	}
	return resolved, nil
}

func sidecarBinaryName() string {
	if runtime.GOOS == "windows" {
		return tuiBinaryName + ".exe"
	}
	return tuiBinaryName
}

func runCommand(cmd *exec.Cmd) error {
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("running OpenTUI: %w", err)
	}
	return nil
}
