package main

// Sample usage:
//
//	go run main.go -dry-run
//	go run main.go -overwrite
//	go run main.go -dry-run -overwrite

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
)

const (
	defaultDirSrc = "E:\\DCIM\\100MSDCF"
	defaultDirDst = "D:\\raw"
)

func main() {
	var (
		editFileExtensions                                   = []string{"xmp"} // lightroom's default edit file extension when edited in local machine
		extensionsToCopy                                     = []string{"arw", "raw"}
		flagDryRun, flagOverwrite, flagDeleteZombieEditFiles bool
		dirSrc, dirDst                                       string
	)

	flag.BoolVar(&flagDryRun, "dry-run", false, "Simulate operations without modifying files (default: false)")
	flag.BoolVar(&flagOverwrite, "overwrite", false, "Overwrite existing files in destination (default: false)")
	flag.BoolVar(&flagDeleteZombieEditFiles, "delete-zombie-edit-files", true, "Delete zombie edit files (default: true)")
	flag.StringVar(&dirSrc, "src", defaultDirSrc, "Source directory")
	flag.StringVar(&dirDst, "dst", defaultDirDst, "Destination directory")
	flag.Parse()

	log.Printf("Starting copying files from %s to %s with extensions %v\n", dirSrc, dirDst, extensionsToCopy)
	if flagDryRun {
		log.Println("Running in Dry-Run mode. No files will be modified.")
	}
	if flagOverwrite {
		log.Println("Running in Overwrite mode. Existing files in destination will be overwritten.")
	} else {
		log.Println("Running in Skip-Existing mode. Existing files in destination will be skipped.")
	}

	totalCopied, removedCount, err := cleanSDCard(editFileExtensions, extensionsToCopy, dirSrc, dirDst, flagDryRun, flagOverwrite, flagDeleteZombieEditFiles)
	if err != nil {
		log.Fatalf("Error cleaning SD card: %s", err.Error())
	}

	log.Printf("\nSummary:\nFiles Copied: %d\nFiles Removed: %d\n", totalCopied, removedCount)
}

// cleanSDCard copies files from dirSrc to dirDst and removes files from dirSrc.
// It returns the number of files copied, the number of files removed, and any error.
func cleanSDCard(editFileExtensions, extensionsToCopy []string, dirSrc, dirDst string, flagDryRun, flagOverwrite, flagDeleteZombieEditFiles bool) (int, int, error) {
	if !flagDryRun {
		if err := os.MkdirAll(dirDst, 0755); err != nil {
			return 0, 0, fmt.Errorf("creating destination directory: %w", err)
		}
	}

	totalCopied := 0
	for _, ext := range extensionsToCopy {
		n, err := copyFiles(dirSrc, dirDst, ext, flagDryRun, flagOverwrite)
		if err != nil {
			return totalCopied, 0, fmt.Errorf("processing .%s files (copied %d): %w", ext, n, err)
		}
		totalCopied += n
	}

	removedCount := 0
	if !flagDryRun {
		var err error
		removedCount, err = removeFiles(dirSrc)
		if err != nil {
			return totalCopied, removedCount, fmt.Errorf("removing files: %w", err)
		}
	}

	if !flagDryRun && flagDeleteZombieEditFiles {
		for _, editFileExtension := range editFileExtensions {
			count, err := deleteZombieEditFiles(editFileExtension, dirDst, extensionsToCopy, true)
			if err != nil {
				return totalCopied, removedCount, fmt.Errorf("deleting zombie edit files with extension %s: %w", editFileExtension, err)
			}
			removedCount += count
		}
	}

	return totalCopied, removedCount, nil
}

type fileCopyError struct {
	fileName string
	err      error
}

func (e fileCopyError) Error() string {
	return fmt.Sprintf("copying file %s: %s", e.fileName, e.err.Error())
}

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
	errsChan := make(chan fileCopyError, len(entries))

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
				errsChan <- fileCopyError{fileName: name, err: copyErr}
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
		errs = errors.Join(errs, e)
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
