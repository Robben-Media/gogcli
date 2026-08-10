//go:build !windows

package tracking

import "os"

func replaceTrackingConfig(source, destination string) error {
	return os.Rename(source, destination)
}
