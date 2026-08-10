package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type downloadTempFile interface {
	io.Writer
	Close() error
	Name() string
}

var (
	createDownloadTempFile = func(dir, pattern string) (downloadTempFile, error) {
		return os.CreateTemp(dir, pattern)
	}
	replaceDownloadFile = replaceFile
)

func writeDownloadFile(destPath string, newFileMode os.FileMode, write func(io.Writer) (int64, error)) (int64, error) {
	resolvedPath, err := resolveDownloadDestination(destPath)
	if err != nil {
		return 0, err
	}

	finalMode := modeForNewDownload(newFileMode)
	if info, statErr := os.Stat(resolvedPath); statErr == nil {
		finalMode = info.Mode().Perm()
	} else if !os.IsNotExist(statErr) {
		return 0, statErr
	}

	dir := filepath.Dir(resolvedPath)
	temp, err := createDownloadTempFile(dir, "."+filepath.Base(resolvedPath)+".tmp-*")
	if err != nil {
		return 0, err
	}
	tempPath := temp.Name()
	closed := false
	committed := false
	defer func() {
		if !closed {
			_ = temp.Close()
		}
		if !committed {
			_ = os.Remove(tempPath)
		}
	}()

	n, err := write(temp)
	if err != nil {
		return 0, err
	}
	if err := temp.Close(); err != nil {
		return 0, fmt.Errorf("closing temporary file: %w", err)
	}
	closed = true
	if err := os.Chmod(tempPath, finalMode); err != nil {
		return 0, fmt.Errorf("setting destination permissions: %w", err)
	}
	if err := replaceDownloadFile(tempPath, resolvedPath); err != nil {
		return 0, fmt.Errorf("replacing destination: %w", err)
	}
	committed = true
	return n, nil
}

func applyDownloadModeMask(mode, mask os.FileMode) os.FileMode {
	return mode &^ mask
}

func resolveDownloadDestination(path string) (string, error) {
	seen := make(map[string]struct{})
	for {
		path = filepath.Clean(path)
		if _, ok := seen[path]; ok {
			return "", fmt.Errorf("resolve destination symlink: cycle at %s", path)
		}
		seen[path] = struct{}{}

		info, err := os.Lstat(path)
		if err != nil {
			if os.IsNotExist(err) {
				return path, nil
			}
			return "", err
		}
		if info.Mode()&os.ModeSymlink == 0 {
			return path, nil
		}

		target, err := os.Readlink(path)
		if err != nil {
			return "", fmt.Errorf("resolve destination symlink: %w", err)
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(path), target)
		}
		path = target
	}
}
