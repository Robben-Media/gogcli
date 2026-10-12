package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// WriteFileAtomic replaces path with data. Each call stages into its own
// owner-only temporary file in the destination directory, so concurrent
// writers never share a staging pathname, and the rename is the commit
// point. On failure the previous destination is left intact and the
// attempt's staging file is removed. The staging name uses a short fixed
// prefix so destinations near the filename length limit stay writable.
func WriteFileAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".gog-staging-*.tmp")
	if err != nil {
		return fmt.Errorf("create staging file: %w", err)
	}

	tmpPath := tmp.Name()
	committed := false

	defer func() {
		if !committed {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()

		return fmt.Errorf("write staging file: %w", err)
	}

	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()

		return fmt.Errorf("sync staging file: %w", err)
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close staging file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace %s: %w", filepath.Base(path), err)
	}

	committed = true

	return nil
}
