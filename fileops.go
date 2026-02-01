package main

import (
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
)

// copyFiles copies files with the given extension from srcDir to dstDir.
// If flagDryRun is true, it counts files without copying.
// If flagOverwrite is true, it overwrites existing files in dstDir.
// It returns the number of files copied and any error.
func copyFiles(srcDir, dstDir, ext string, flagDryRun, flagOverwrite bool) (int, error) {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return 0, err
	}

	var count atomic.Int32
	var wg sync.WaitGroup
	errChan := make(chan fileCopyError, len(entries))

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		wg.Go(func() {
			name := entry.Name()
			if !strings.EqualFold(filepath.Ext(name), "."+ext) {
				return
			}

			srcPath := filepath.Join(srcDir, name)
			dstPath := filepath.Join(dstDir, name)

			if !flagOverwrite {
				if _, statErr := os.Stat(dstPath); statErr == nil {
					log.Printf("Skipping copying existing file: %s\n", name)
					return
				}
			}

			if flagDryRun {
				log.Printf("[DryRun] Would copy %s\n", name)
				count.Add(1)
				return
			}

			if copyErr := copyFile(srcPath, dstPath); copyErr != nil {
				errChan <- fileCopyError{fileName: name, err: copyErr}
				return
			}

			log.Printf("Copied %s\n", name)
			count.Add(1)
		})
	}

	wg.Wait()
	close(errsChan)

	var errs error
	for e := range errsChan {
		err = errors.Join(errs, e)
	}

	return int(count.Load()), errs
}

// copyFile copies a single file from src to dst.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

// removeFiles removes all files in the directory.
// It returns the number of files removed and any error.
func removeFiles(dir string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		if err := os.Remove(path); err != nil {
			return count, err
		}
		log.Printf("Removed %s\n", entry.Name())
		count++
	}
	return count, nil
}