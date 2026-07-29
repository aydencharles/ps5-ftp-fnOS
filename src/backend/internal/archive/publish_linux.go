//go:build linux

package archive

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func Publish(stage, destination string) error {
	err := unix.Renameat2(unix.AT_FDCWD, stage, unix.AT_FDCWD, destination, unix.RENAME_NOREPLACE)
	if err == nil {
		return nil
	}
	if errors.Is(err, unix.EEXIST) {
		return os.ErrExist
	}
	if !errors.Is(err, unix.ENOSYS) && !errors.Is(err, unix.EINVAL) {
		return err
	}
	return publishFallback(stage, destination)
}
