//go:build !windows

package discovery

import (
	"os"
	"runtime"
	"syscall"
)

func trustedScriptOwner(info os.FileInfo) bool {
	if runtime.GOOS != "linux" {
		return true
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == 0
}
