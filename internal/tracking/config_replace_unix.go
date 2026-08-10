//go:build !windows

package tracking

import (
	"fmt"
	"os"
)

func replaceTrackingConfig(source, destination string) error {
	if err := os.Rename(source, destination); err != nil {
		return fmt.Errorf("rename tracking config: %w", err)
	}

	return nil
}
