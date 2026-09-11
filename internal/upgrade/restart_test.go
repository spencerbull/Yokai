package upgrade

import (
	"os"
	"path/filepath"
	"testing"
)

const netTable = "  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n" +
	"   0: 0100007F:1D31 00000000:0000 0A 00000000:00000000 00:00000000 00000000 1000 0 12345 1 0000000000000000 100 0 0\n" +
	"   1: 0100007F:C350 00000000:0000 0A 00000000:00000000 00:00000000 00000000 1000 0 999 1 0000000000000000 100 0 0\n"

func TestReadPidFileRoundtrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	yokaiDir := filepath.Join(dir, "yokai")
	if err := os.MkdirAll(yokaiDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(yokaiDir, daemonPidFile), []byte("4242\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := readPidFile(); got != 4242 {
		t.Fatalf("readPidFile = %d, want 4242", got)
	}
}

func TestReadPidFileMissing(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if got := readPidFile(); got != 0 {
		t.Fatalf("expected 0 for missing pid file, got %d", got)
	}
}

func TestSocketInodesFromTable(t *testing.T) {
	inodes := socketInodesFromTable(netTable, "1D31")
	if len(inodes) != 1 || inodes[0] != "12345" {
		t.Fatalf("socketInodesFromTable = %v, want [12345]", inodes)
	}
	// Case-insensitive port match; other port should not match.
	if got := socketInodesFromTable(netTable, "1d31"); len(got) != 1 {
		t.Fatalf("case-insensitive match failed: %v", got)
	}
	if got := socketInodesFromTable(netTable, "C350"); len(got) != 1 || got[0] != "999" {
		t.Fatalf("expected inode 999 for C350, got %v", got)
	}
	if got := socketInodesFromTable(netTable, "ABCD"); len(got) != 0 {
		t.Fatalf("expected no match, got %v", got)
	}
}

func TestPidAlive(t *testing.T) {
	if pidAlive(0) || pidAlive(-1) {
		t.Fatal("pidAlive should be false for non-positive PIDs")
	}
	if !pidAlive(os.Getpid()) {
		t.Fatal("pidAlive(self) should be true")
	}
}
