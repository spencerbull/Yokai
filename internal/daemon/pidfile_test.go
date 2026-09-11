package daemon

import (
	"os"
	"strconv"
	"testing"
)

func TestWriteAndRemovePidFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	if err := WritePidFile(); err != nil {
		t.Fatalf("WritePidFile: %v", err)
	}
	p, err := PidFilePath()
	if err != nil {
		t.Fatalf("PidFilePath: %v", err)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("reading pid file: %v", err)
	}
	pid, err := strconv.Atoi(string(data))
	if err != nil {
		t.Fatalf("parsing pid %q: %v", string(data), err)
	}
	if pid != os.Getpid() {
		t.Fatalf("pid file = %d, want current pid %d", pid, os.Getpid())
	}

	RemovePidFile()
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatalf("pid file not removed: %v", err)
	}
}

func TestRemovePidFileIdempotent(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	RemovePidFile() // should not panic or error
}
