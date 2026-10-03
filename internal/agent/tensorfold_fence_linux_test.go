//go:build linux

package agent

import (
	"errors"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

func TestTensorFoldFenceKillsOnlyMarkedProcesses(t *testing.T) {
	marker := "RSYNC_RSH=/tmp/yokai-fence-test/" + t.Name() + "/ssh"
	start := func(environment []string) *exec.Cmd {
		command := exec.Command("sleep", "60")
		command.Env = environment
		// Detach from the test's process group, like a launcher that
		// outlived the agent that started it.
		command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		if err := command.Start(); err != nil {
			t.Fatal(err)
		}
		return command
	}
	marked := start([]string{"PATH=/usr/bin:/bin", marker})
	unmarked := start([]string{"PATH=/usr/bin:/bin", marker + "-other"})
	t.Cleanup(func() { _ = unmarked.Process.Kill(); _ = unmarked.Wait() })

	exited := make(chan error, 1)
	go func() { exited <- marked.Wait() }()
	if err := fenceTensorFoldProcesses(marker); err != nil {
		t.Fatalf("fence failed: %v", err)
	}
	select {
	case err := <-exited:
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || !exitErr.Sys().(syscall.WaitStatus).Signaled() {
			t.Fatalf("marked launcher was not killed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("marked launcher survived fencing")
	}
	if err := unmarked.Process.Signal(syscall.Signal(0)); err != nil {
		t.Fatalf("fence killed an unrelated process: %v", err)
	}
}
