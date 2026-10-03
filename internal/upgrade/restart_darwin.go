//go:build darwin

package upgrade

import (
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

// platformProcessAlive reports whether pid exists as a live (non-zombie)
// process. A daemon launched by the TUI launcher (which releases and never
// reaps) becomes a zombie after SIGTERM; signal 0 still succeeds for it, which
// would make waitForExit time out. Detect the zombie (Z) stat and treat it as
// exited.
func platformProcessAlive(pid int) bool {
	if darwinZombie(pid) {
		return false
	}
	err := syscall.Kill(pid, syscall.Signal(0))
	return err == nil || err == syscall.EPERM
}

// platformTerminate sends SIGTERM to pid.
func platformTerminate(pid int) {
	_ = syscall.Kill(pid, syscall.SIGTERM)
}

// findDaemonPIDByPort is unsupported on Darwin (no /proc); rely on the PID file.
func findDaemonPIDByPort(addr string) (int, bool) {
	return 0, false
}

// daemonPIDLooksOwned verifies the kernel process name and command line through
// ps. This supports daemons predating the health PID while rejecting stale
// pidfiles that now refer to an unrelated process.
func daemonPIDLooksOwned(pid int, addr string) bool {
	pidArg := strconv.Itoa(pid)
	comm, err := exec.Command("ps", "-o", "ucomm=", "-p", pidArg).Output()
	if err != nil {
		return false
	}
	args, err := exec.Command("ps", "-ww", "-o", "args=", "-p", pidArg).Output()
	if err != nil {
		return false
	}
	return commandLooksLikeYokaiDaemon(strings.TrimSpace(string(comm)), strings.TrimSpace(string(args)))
}

// darwinZombie reports whether pid is a zombie by inspecting its ps stat field
// (e.g. "Z", "Z+"). Uses ps because the stdlib syscall package exposes no
// arg-taking sysctl on Darwin.
func darwinZombie(pid int) bool {
	out, err := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return false
	}
	return statIndicatesZombie(string(out))
}
