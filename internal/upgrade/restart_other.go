//go:build !linux

package upgrade

import (
	"os"
	"syscall"
)

// findDaemonPIDByPort is unsupported on this platform; rely on the PID file.
func findDaemonPIDByPort(addr string) (int, bool) {
	return 0, false
}

// platformProcessAlive reports whether the given PID exists. On Windows
// FindProcess always succeeds, so probe with signal 0; a permission error
// also implies the process exists.
func platformProcessAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}

// platformTerminate stops pid. On Windows there is no POSIX SIGTERM, so a
// force kill is the only portable stop signal.
func platformTerminate(pid int) {
	if p, err := os.FindProcess(pid); err == nil {
		_ = p.Kill()
	}
}
