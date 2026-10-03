//go:build linux

package agent

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

// fenceTensorFoldProcesses SIGKILLs every local process whose initial
// environment contains marker, and returns only once none remain. Launcher
// descendants inherit the launch-specific marker, so this reaches a launcher
// that survived an agent restart and any child that left its process group.
func fenceTensorFoldProcesses(marker string) error {
	deadline := time.Now().Add(5 * time.Second)
	for {
		pids, err := tensorFoldProcessesWithEnvironment(marker)
		if err != nil {
			return err
		}
		if len(pids) == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%d TensorFold launcher processes survived SIGKILL", len(pids))
		}
		for _, pid := range pids {
			if err := syscall.Kill(pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
				return fmt.Errorf("kill TensorFold launcher process %d: %w", pid, err)
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func tensorFoldProcessesWithEnvironment(marker string) ([]int, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}
	self := os.Getpid()
	needle := []byte(marker)
	var pids []int
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid == self {
			continue
		}
		// Exited processes and other users' processes are unreadable; neither
		// can be a live launcher this agent started. Zombies read as empty.
		environment, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "environ"))
		if err != nil {
			continue
		}
		for _, variable := range bytes.Split(environment, []byte{0}) {
			if bytes.Equal(variable, needle) {
				pids = append(pids, pid)
				break
			}
		}
	}
	return pids, nil
}
