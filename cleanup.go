package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
)

// deleteZombieEditFiles deletes edit files that have no corresponding raw file.
// It checks for raw files with extensions in rawFileExtensions.
// If isRecursive is true, it processes subdirectories recursively.
// It returns the number of files deleted and any error.
func deleteZombieEditFiles(editFileExtension, dir string, rawFileExtensions []string, isRecursive bool) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, fmt.Errorf("reading directory: %w", err)
	}

	var count atomic.Int32
	var wg sync.WaitGroup
	errsChan := make(chan error, len(entries))

	for _, entry := range entries {
		wg.Go(func() {
			if entry.IsDir() {
				if isRecursive {
					n, err := deleteZombieEditFiles(editFileExtension, filepath.Join(dir, entry.Name()), rawFileExtensions, isRecursive)
					if err != nil {
						errsChan <- fmt.Errorf("processing subdirectory %s: %w", entry.Name(), err)
						return
					}
					count.Add(int32(n))
				}
				return
			}

			editFileName := entry.Name()
			if !strings.HasSuffix(editFileName, "."+editFileExtension) {
				return
			}

			extTrimmed := strings.TrimSuffix(editFileName, "."+editFileExtension)
			hasCorrespondingRawFile := false

			for _, rawFileExt := range rawFileExtensions {
				expectedRawFileName := extTrimmed + "." + rawFileExt
				if _, err := os.Stat(filepath.Join(dir, expectedRawFileName)); err == nil {
					hasCorrespondingRawFile = true
					break
				} else if !errors.Is(err, os.ErrNotExist) {
					errsChan <- fmt.Errorf("checking if %s exists: %w", expectedRawFileName, err)
					return
				}
			}

			if hasCorrespondingRawFile {
				return
			}

			if err := os.Remove(filepath.Join(dir, editFileName)); err != nil {
				errsChan <- fmt.Errorf("removing zombie edit file %s: %w", editFileName, err)
				return
			}

			log.Printf("Removed zombie edit file: %s\n", editFileName)
			count.Add(1)
		})
	}

	wg.Wait()
	close(errsChan)

	var errs error
	for e := range errsChan {
		errs = errors.Join(errs, e)
	}

	return int(count.Load()), errs
}
