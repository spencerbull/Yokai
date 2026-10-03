//go:build !windows

package agent

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

func tensorFoldConfigureCommand(command *exec.Cmd) error {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return nil
}

func tensorFoldSignalProcessGroup(pid int, force bool) error {
	signal := syscall.SIGTERM
	if force {
		signal = syscall.SIGKILL
	}
	if err := syscall.Kill(-pid, signal); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	return nil
}

func tensorFoldExitWasIntentionalKill(err error) bool {
	var exitError *exec.ExitError
	if !errors.As(err, &exitError) {
		return false
	}
	status, ok := exitError.Sys().(syscall.WaitStatus)
	return ok && status.Signaled() && status.Signal() == syscall.SIGKILL
}

func tensorFoldFileOwnedByCurrentUser(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(stat.Uid) == os.Geteuid()
}
