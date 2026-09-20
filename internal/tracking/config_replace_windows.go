//go:build windows

package tracking

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

const replaceTrackingConfigWriteThrough = 0x1

var replaceTrackingConfigW = windows.NewLazySystemDLL("kernel32.dll").NewProc("ReplaceFileW")

func replaceTrackingConfig(source, destination string) error {
	sourcePath, err := windows.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	destinationPath, err := windows.UTF16PtrFromString(destination)
	if err != nil {
		return err
	}

	if _, err := os.Stat(destination); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		return windows.MoveFileEx(sourcePath, destinationPath, windows.MOVEFILE_WRITE_THROUGH)
	}
	result, _, callErr := replaceTrackingConfigW.Call(
		uintptr(unsafe.Pointer(destinationPath)),
		uintptr(unsafe.Pointer(sourcePath)),
		0,
		replaceTrackingConfigWriteThrough,
		0,
		0,
	)
	if result != 0 {
		return nil
	}
	if callErr != windows.ERROR_SUCCESS {
		return callErr
	}
	return windows.ERROR_GEN_FAILURE
}
