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

func writeDownloadFile(destPath string, write func(io.Writer) (int64, error)) (int64, error) {
	resolvedPath, err := resolveDownloadDestination(destPath)
	if err != nil {
		return 0, err
	}

	var existingMode os.FileMode
	if info, statErr := os.Stat(resolvedPath); statErr == nil {
		existingMode = info.Mode().Perm()
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
	if err := replaceDownloadFile(tempPath, resolvedPath); err != nil {
		return 0, fmt.Errorf("replacing destination: %w", err)
	}
	committed = true
	if existingMode != 0 {
		if err := os.Chmod(resolvedPath, existingMode); err != nil {
			return 0, fmt.Errorf("restoring destination permissions: %w", err)
		}
	}
	return n, nil
}

func resolveDownloadDestination(path string) (string, error) {
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
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return resolved, nil
	}
	target, readErr := os.Readlink(path)
	if readErr != nil {
		return "", fmt.Errorf("resolve destination symlink: %w", err)
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(path), target)
	}
	return filepath.Clean(target), nil
}
