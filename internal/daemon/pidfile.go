package daemon

import (
	"os"
	"path/filepath"
	"strconv"

	"github.com/spencerbull/yokai/internal/config"
)

// pidFileName is the daemon PID file stored under the config directory so the
// upgrade flow can locate and restart a running daemon after its binary is
// replaced (the daemon is launched detached and otherwise survives upgrades,
// silently keeping the previous version running).
const pidFileName = "daemon.pid"

// PidFilePath returns the path to the daemon PID file.
func PidFilePath() (string, error) {
	dir, err := config.ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, pidFileName), nil
}

// WritePidFile records the current process PID so `yokai upgrade` can find the
// running daemon. It is best-effort: a failure to persist is non-fatal.
func WritePidFile() error {
	path, err := PidFilePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())), 0o600)
}

// RemovePidFile deletes the daemon PID file if present.
func RemovePidFile() {
	if path, err := PidFilePath(); err == nil {
		_ = os.Remove(path)
	}
}
