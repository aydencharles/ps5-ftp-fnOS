package archive

import (
	"errors"
	"os"
)

func publishFallback(stage, destination string) error {
	if _, err := os.Lstat(destination); err == nil {
		return os.ErrExist
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(stage, destination)
}
