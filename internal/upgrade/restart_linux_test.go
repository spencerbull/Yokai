//go:build linux

package upgrade

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestDaemonPIDLooksOwnedRejectsNonYokaiDaemonArgument(t *testing.T) {
	cmd := exec.Command("sh", "-c", "while :; do sleep 1; done", "daemon")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper process: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	deadline := time.Now().Add(time.Second)
	for !isDaemonArgv(cmd.Process.Pid) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !isDaemonArgv(cmd.Process.Pid) {
		cmdline, _ := os.ReadFile(filepath.Join("/proc", strconv.Itoa(cmd.Process.Pid), "cmdline"))
		t.Fatalf("helper argv never contained daemon: %q", cmdline)
	}
	if daemonPIDLooksOwned(cmd.Process.Pid, "127.0.0.1:1") {
		t.Fatal("non-Yokai process with daemon argument was accepted as the daemon")
	}
}
