//go:build !windows

package cmd

import "os"

func replaceServiceAccountFile(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}

func secureServiceAccountFile(string) error {
	return nil
}
