//go:build windows

package upgrade

import (
	"debug/buildinfo"
	"unsafe"

	"golang.org/x/sys/windows"
)

const windowsStillActive = 259

const windowsCommandLineBufferSize = 1<<16 + 32

// findDaemonPIDByPort is unsupported on Windows; a healthy daemon proves
// ownership by reporting the same PID that it persisted at startup.
func findDaemonPIDByPort(addr string) (int, bool) {
	return 0, false
}

// daemonPIDLooksOwned verifies the Go build identity of the executable attached
// to pid. This supports daemons predating the health PID while rejecting stale
// pidfiles that now refer to an unrelated process.
func daemonPIDLooksOwned(pid int, addr string) bool {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer func() { _ = windows.CloseHandle(handle) }()

	path := make([]uint16, 32768)
	size := uint32(len(path))
	if err := windows.QueryFullProcessImageName(handle, 0, &path[0], &size); err != nil {
		return false
	}
	info, err := buildinfo.ReadFile(windows.UTF16ToString(path[:size]))
	if err != nil || !isYokaiBuild(info) {
		return false
	}

	commandLine, ok := windowsProcessCommandLine(handle)
	if !ok {
		return false
	}
	args, err := windows.DecomposeCommandLine(commandLine)
	return err == nil && hasDaemonSubcommand(args)
}

func windowsProcessCommandLine(handle windows.Handle) (string, bool) {
	buffer := make([]byte, windowsCommandLineBufferSize)
	var returned uint32
	if err := windows.NtQueryInformationProcess(handle, windows.ProcessCommandLineInformation, unsafe.Pointer(&buffer[0]), uint32(len(buffer)), &returned); err != nil {
		return "", false
	}
	commandLine := (*windows.NTUnicodeString)(unsafe.Pointer(&buffer[0]))
	if commandLine.Buffer == nil || commandLine.Length == 0 || commandLine.Length%2 != 0 {
		return "", false
	}
	value := unsafe.Slice(commandLine.Buffer, int(commandLine.Length/2))
	return windows.UTF16ToString(value), true
}

// platformProcessAlive checks the native process exit code. os.FindProcess and
// signal 0 do not provide a liveness probe on Windows.
func platformProcessAlive(pid int) bool {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer func() { _ = windows.CloseHandle(handle) }()

	var exitCode uint32
	return windows.GetExitCodeProcess(handle, &exitCode) == nil && exitCode == windowsStillActive
}

// platformTerminate forcefully stops pid because Windows has no POSIX
// SIGTERM. Ownership is established before this function is called.
func platformTerminate(pid int) {
	handle, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return
	}
	defer func() { _ = windows.CloseHandle(handle) }()
	_ = windows.TerminateProcess(handle, 1)
}
