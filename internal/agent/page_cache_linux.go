//go:build linux

package agent

import (
	"os"

	"golang.org/x/sys/unix"
)

func openPageCacheBlob(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}

func adviseDropPageCache(file *os.File) error {
	return unix.Fadvise(int(file.Fd()), 0, 0, unix.FADV_DONTNEED)
}
