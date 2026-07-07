package platform

import (
	"os"
	"runtime"
)

// ChmodIfSupported calls os.Chmod on platforms that support it (Unix).
// On Windows, where chmod is not meaningful, it returns nil.
func ChmodIfSupported(name string, mode os.FileMode) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	return os.Chmod(name, mode)
}
