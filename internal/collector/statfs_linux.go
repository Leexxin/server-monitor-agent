//go:build linux

package collector

import "syscall"

func filesystemStats(path string) (fsStats, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return fsStats{}, err
	}
	blockSize := uint64(stat.Bsize)
	return fsStats{
		size: saturatingMul(stat.Blocks, blockSize), available: saturatingMul(stat.Bavail, blockSize),
		free: saturatingMul(stat.Bfree, blockSize), files: stat.Files, filesFree: stat.Ffree,
	}, nil
}
