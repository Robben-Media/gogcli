//go:build !windows

package cmd

import (
	"os"
	"syscall"
)

var downloadProcessUmask = func() os.FileMode {
	mask := syscall.Umask(0)
	syscall.Umask(mask)
	return os.FileMode(mask)
}()

func modeForNewDownload(mode os.FileMode) os.FileMode {
	return applyDownloadModeMask(mode, downloadProcessUmask)
}

func replaceFile(source, destination string) error {
	return os.Rename(source, destination)
}
