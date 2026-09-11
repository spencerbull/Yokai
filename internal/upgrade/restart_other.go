//go:build !linux

package upgrade

// findDaemonPIDByPort is unsupported on this platform; rely on the PID file.
func findDaemonPIDByPort(addr string) (int, bool) {
	return 0, false
}
