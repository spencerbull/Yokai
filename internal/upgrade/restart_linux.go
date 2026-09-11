//go:build linux

package upgrade

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// findDaemonPIDByPort locates the PID listening on addr by walking /proc
// socket inodes. It returns ok=false when the listener cannot be found.
func findDaemonPIDByPort(addr string) (int, bool) {
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return 0, false
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 {
		return 0, false
	}
	portHex := fmt.Sprintf("%X", port)

	inodes := socketInodesForPort("/proc/net/tcp", portHex)
	if len(inodes) == 0 {
		inodes = socketInodesForPort("/proc/net/tcp6", portHex)
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

// socketInodesForPort returns the socket inodes from a /proc/net table whose
// local port matches (in hex, e.g. "1D31" for 7473).
func socketInodesForPort(tablePath, portHex string) []string {
	data, err := os.ReadFile(tablePath)
	if err != nil {
		return nil
	}
	return socketInodesFromTable(string(data), portHex)
}

// socketInodesFromTable parses the body of a /proc/net/tcp[6] table.
func socketInodesFromTable(data, portHex string) []string {
	var inodes []string
	lines := strings.Split(data, "\n")
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) < 10 {
			continue
		}
		local := fields[1] // e.g. "0100007F:1D31"
		addrPort := strings.Split(local, ":")
		if len(addrPort) != 2 {
			continue
		}
		if strings.EqualFold(addrPort[1], portHex) {
			inodes = append(inodes, fields[9])
		}
	}
	return inodes
}
