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
	replaceDownloadFile = os.Rename
)

func writeDownloadFile(destPath string, write func(io.Writer) (int64, error)) (int64, error) {
	dir := filepath.Dir(destPath)
	temp, err := createDownloadTempFile(dir, "."+filepath.Base(destPath)+".tmp-*")
	if err != nil {
		return 0, err
	}
	tempPath := temp.Name()
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(tempPath)
		}
	}()

	n, err := write(temp)
	if err != nil {
		_ = temp.Close()
		return 0, err
	}
	if err := temp.Close(); err != nil {
		return 0, fmt.Errorf("closing temporary file: %w", err)
	}
	if err := replaceDownloadFile(tempPath, destPath); err != nil {
		return 0, fmt.Errorf("replacing destination: %w", err)
	}
	committed = true
	return n, nil
}
