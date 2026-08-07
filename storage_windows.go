//go:build windows

package main

import (
	"fmt"
	"syscall"
	"unsafe"
)

type storageInfo struct {
	Total uint64
	Free  uint64
}

var getDiskFreeSpaceEx = syscall.NewLazyDLL("kernel32.dll").NewProc("GetDiskFreeSpaceExW")

func getStorageInfo(path string) (storageInfo, error) {
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return storageInfo{}, err
	}

	var freeBytes, totalBytes, totalFreeBytes uint64
	result, _, callErr := getDiskFreeSpaceEx.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&freeBytes)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&totalFreeBytes)),
	)
	if result == 0 {
		return storageInfo{}, fmt.Errorf("get disk space for %s: %w", path, callErr)
	}

	return storageInfo{Total: totalBytes, Free: freeBytes}, nil
}
