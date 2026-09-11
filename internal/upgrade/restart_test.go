package upgrade

import (
	"net"
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

func TestIPToProcHex(t *testing.T) {
	if got := ip4ToProcHex(net.ParseIP("127.0.0.1")); got != "0100007F" {
		t.Fatalf("ip4ToProcHex(127.0.0.1) = %q, want 0100007F", got)
	}
	if got := ip4ToProcHex(net.ParseIP("0.0.0.0")); got != "00000000" {
		t.Fatalf("ip4ToProcHex(0.0.0.0) = %q, want 00000000", got)
	}
	if got := ip6ToProcHex(net.ParseIP("::1")); got != "00000000000000000000000001000000" {
		t.Fatalf("ip6ToProcHex(::1) = %q, want ...01000000", got)
	}
}

// listenerTable reuses the full-width /proc/net/tcp line layout so field 9 is
// the inode: one LISTEN on 127.0.0.1:7473, one non-LISTEN on the same tuple,
// and one LISTEN on 0.0.0.1:7473 (same numeric port, different address).
const listenerTable = "  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n" +
	"   0: 0100007F:1D31 00000000:0000 0A 00000000:00000000 00:00000000 00000000 1000 0 12345 1 0000000000000000 100 0 0\n" +
	"   1: 0100007F:1D31 00000000:0000 01 00000000:00000000 00:00000000 00000000 1000 0 54321 1 0000000000000000 100 0 0\n" +
	"   2: 0800007F:1D31 00000000:0000 0A 00000000:00000000 00:00000000 00000000 1000 0 88888 1 0000000000000000 100 0 0\n"

func TestMatchingListenInodesAddressAndStateFiltering(t *testing.T) {
	// Matching address + LISTEN state selects the right inode.
	got := matchingListenInodes(listenerTable, "1D31", "0100007F", false, false, false)
	if len(got) != 1 || got[0] != "12345" {
		t.Fatalf("matching listen = %v, want [12345]", got)
	}
	// Same numeric port but a different, absent address (e.g. 0xDEADBEEF) is excluded.
	if got := matchingListenInodes(listenerTable, "1D31", "DEADBEEF", false, false, false); len(got) != 0 {
		t.Fatalf("wrong address should not match, got %v", got)
	}
	// Address-family mismatch (requested IPv6 against an IPv4 table) matches nothing.
	if got := matchingListenInodes(listenerTable, "1D31", "00000000000000000000000001000000", true, true, false); len(got) != 0 {
		t.Fatalf("family mismatch should not match, got %v", got)
	}
	// Any-address mode keeps LISTEN entries regardless of address (non-LISTEN excluded).
	if got := matchingListenInodes(listenerTable, "1D31", "", false, false, true); len(got) != 2 {
		t.Fatalf("any-address listen = %v, want 2 (non-LISTEN excluded)", got)
	}
}
