//go:build linux

package upgrade

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// platformProcessAlive reports whether the given PID exists as a live (non-
// zombie) process. A daemon launched by the TUI launcher (which releases the
// process and never reaps it) becomes a zombie after SIGTERM; kill(pid,0) still
// succeeds for a zombie, which would make waitForExit time out and the upgrade
// would wrongly conclude the daemon is still running. Treat zombies as exited.
func platformProcessAlive(pid int) bool {
	if zombie(pid) {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

// zombie reports whether /proc/<pid> exists in the zombie state (Z). It returns
// false when the process cannot be inspected (does not exist, permission, etc).
func zombie(pid int) bool {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return false
	}
	// stat format: "pid (comm) state ..." — comm may contain spaces/parens, so
	// the state char is the first non-space token after the final ')'.
	idx := bytes.LastIndex(data, []byte(")"))
	if idx < 0 || idx+2 >= len(data) {
		return false
	}
	return data[idx+2] == 'Z'
}

// platformTerminate sends SIGTERM to pid.
func platformTerminate(pid int) {
	_ = syscall.Kill(pid, syscall.SIGTERM)
}

// findDaemonPIDByPort locates the PID listening on addr by walking /proc
// socket inodes. It matches the configured address and family (IPv4 vs IPv6)
// and only LISTEN (0A) sockets, so an unrelated process sharing the same
// numeric port on another address is never signalled. Returns ok=false when
// the listener cannot be found.
func findDaemonPIDByPort(addr string) (int, bool) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return 0, false
	}
	if host == "localhost" {
		host = "127.0.0.1"
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 {
		return 0, false
	}
	portHex := fmt.Sprintf("%X", port)

	anyAddr := host == "" || host == "0.0.0.0" || host == "::"
	ipv6 := false
	wantAddrHex := ""
	if !anyAddr {
		ip := net.ParseIP(host)
		if ip == nil {
			return 0, false
		}
		if ip4 := ip.To4(); ip4 != nil {
			wantAddrHex = ip4ToProcHex(ip4)
		} else {
			ipv6 = true
			wantAddrHex = ip6ToProcHex(ip)
		}
	}

	// For a specific IPv4 address only /proc/net/tcp applies; a specific IPv6
	// or any-address listener may live in either family's table.
	tables := []string{"/proc/net/tcp"}
	if ipv6 || anyAddr {
		tables = append(tables, "/proc/net/tcp6")
	}

	var inodes []string
	for _, table := range tables {
		data, err := os.ReadFile(table)
		if err != nil {
			continue
		}
		tcp6 := strings.Contains(table, "tcp6")
		inodes = append(inodes, matchingListenInodes(string(data), portHex, wantAddrHex, ipv6, tcp6, anyAddr)...)
	}
	if len(inodes) == 0 {
		return 0, false
	}

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0, false
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		fdDir := filepath.Join("/proc", entry.Name(), "fd")
		fds, err := os.ReadDir(fdDir)
		if err != nil {
			continue
		}
		for _, fd := range fds {
			link, err := os.Readlink(filepath.Join(fdDir, fd.Name()))
			if err != nil {
				continue
			}
			for _, inode := range inodes {
				if link == "socket:["+inode+"]" {
					return pid, true
				}
			}
		}
	}
	return 0, false
}

// matchingListenInodes returns the socket inodes of LISTEN (0A) entries whose
// local port matches portHex and, when the address is pinned, whose local
// address and family match too.
func matchingListenInodes(table, portHex, wantAddrHex string, ipv6, tcp6, anyAddr bool) []string {
	var inodes []string
	for _, line := range strings.Split(table, "\n")[1:] {
		fields := strings.Fields(line)
		if len(fields) < 10 {
			continue
		}
		if fields[3] != "0A" { // not LISTEN
			continue
		}
		local := fields[1]
		addrPort := strings.Split(local, ":")
		if len(addrPort) != 2 {
			continue
		}
		if !strings.EqualFold(addrPort[1], portHex) {
			continue
		}
		if !anyAddr {
			if tcp6 != ipv6 || !strings.EqualFold(addrPort[0], wantAddrHex) {
				continue
			}
		}
		inodes = append(inodes, fields[9])
	}
	return inodes
}

// socketInodesFromTable parses the body of a /proc/net/tcp[6] table, returning
// inodes whose local port matches portHex (legacy helper, kept for tests).
func socketInodesFromTable(data, portHex string) []string {
	return matchingListenInodes(data, portHex, "", false, false, true)
}

// ip4ToProcHex encodes an IPv4 address in /proc/net/tcp little-endian hex form
// (e.g. 127.0.0.1 -> "0100007F").
func ip4ToProcHex(ip net.IP) string {
	b := ip.To4()
	out := make([]byte, 4)
	for i := 0; i < 4; i++ {
		out[i] = b[3-i]
	}
	// /proc/net/tcp stores addresses uppercase hex.
	return strings.ToUpper(hex.EncodeToString(out))
}

// ip6ToProcHex encodes an IPv6 address in /proc/net/tcp6 form: four 4-byte
// words, each stored little-endian (e.g. ::1 -> "00000000000000000000000001000000").
func ip6ToProcHex(ip net.IP) string {
	b := ip.To16()
	out := make([]byte, 16)
	for i := 0; i < 16; i += 4 {
		for j := 0; j < 4; j++ {
			out[i+j] = b[i+3-j]
		}
	}
	return strings.ToUpper(hex.EncodeToString(out))
}
