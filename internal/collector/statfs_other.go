//go:build !linux && !windows

package collector

import "fmt"

func filesystemStats(string) (fsStats, error) {
	return fsStats{}, fmt.Errorf("filesystem collector is supported on Linux only")
}
