//go:build !windows

package cmd

import "os"

func replaceServiceAccountFile(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath) //nolint:gosec // Both paths are derived from the config-owned service account path.
}

func secureServiceAccountFile(string) error {
	return nil
}
