//go:build !linux

package agent

import (
	"fmt"
	"os"
)

func openPageCacheBlob(path string) (*os.File, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("page-cache blob is not a regular file")
	}
	return os.Open(path)
}

func adviseDropPageCache(*os.File) error {
	return fmt.Errorf("POSIX_FADV_DONTNEED is unavailable on this platform")
}
