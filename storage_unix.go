//go:build !windows

package main

import (
	"fmt"
	"syscall"
)

type storageInfo struct {
	Total uint64
	Free  uint64
}

func getStorageInfo(path string) (storageInfo, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return storageInfo{}, fmt.Errorf("get disk space for %s: %w", path, err)
	}

	return storageInfo{
		Total: stat.Blocks * uint64(stat.Bsize),
		Free:  stat.Bavail * uint64(stat.Bsize),
	}, nil
}
