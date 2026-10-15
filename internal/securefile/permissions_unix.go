//go:build !windows

package securefile

import (
	"errors"
	"os"
)

func Check(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Mode().Perm()&0o077 != 0 {
		return errors.New("auth token file must not be accessible by group or others")
	}
	return nil
}
