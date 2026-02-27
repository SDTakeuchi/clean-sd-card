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

	defaultDirDstJPG = "D:\\jpg"
)

func main() {
	var (
		editFileExtensions        = []string{"xmp"} // lightroom's default edit file extension when edited in local machine
		extensionsToCopy          = []string{"arw", "raw"}
		extensionsJPG             = []string{"jpg", "jpeg"}
		flagDryRun                bool
		flagKeepJPG               bool
		flagOverwrite             bool
		flagDeleteZombieEditFiles bool
		dirSrc, dirDst            string
	)

	flag.BoolVar(&flagDryRun, "dry-run", false, "Simulate operations without modifying files (default: false)")
	flag.BoolVar(&flagOverwrite, "overwrite", false, "Overwrite existing files in destination (default: false)")
	flag.BoolVar(&flagKeepJPG, "keep-jpg", true, "Keep JPG files in destination (default: true)")
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

	totalCopied, removedCount, err := cleanSDCard(
		editFileExtensions,
		extensionsToCopy,
		extensionsJPG,
		dirSrc,
		dirDst,
		flagDryRun,
		flagKeepJPG,
		flagOverwrite,
		flagDeleteZombieEditFiles,
	)
	if err != nil {
		log.Fatalf("failed cleaning SD card: %s", err.Error())
	}

	log.Printf("\nSummary:\nFiles Copied: %d\nFiles Removed: %d\n", totalCopied, removedCount)
}

// cleanSDCard copies files from dirSrc to dirDst and removes files from dirSrc.
// It returns the number of files copied, the number of files removed, and any error.
func cleanSDCard(
	editFileExtensions, extensionsToCopy, extensionsJPG []string,
	dirSrc, dirDst string,
	flagDryRun, flagKeepJPG, flagOverwrite, flagDeleteZombieEditFiles bool,
) (int, int, error) {
	if !flagDryRun {
		if err := os.MkdirAll(dirDst, 0755); err != nil {
			return 0, 0, fmt.Errorf("failed to create destination directory: %w", err)
		}
	}

	// copy raw files
	totalCopied := 0
	for _, ext := range extensionsToCopy {
		n, err := copyFiles(dirSrc, dirDst, ext, flagDryRun, flagOverwrite)
		if err != nil {
			return totalCopied, 0, fmt.Errorf("failed to copy .%s files (copied %d): %w", ext, n, err)
		}
		totalCopied += n
	}

	// copy jpg
	var countJPGToCopy int
	if flagKeepJPG {
		if flagDryRun {
			var countErr error
			// just count jpg files without copying
			for _, ext := range extensionsJPG {
				c, err := countFilesWithExtension(dirSrc, ext)
				if err != nil {
					countErr = errors.Join(countErr, err)
					continue
				}
				countJPGToCopy += c
			}
			if countErr != nil {
				log.Printf("[WARN] failed to count jpg files: %s", countErr.Error())
			} else {
				log.Printf("[dry-run] would copy %d JPG files\n", countJPGToCopy)
			}
		} else {
			for _, ext := range extensionsJPG {
				n, err := copyFiles(dirSrc, defaultDirDstJPG, ext, flagDryRun, flagOverwrite)
				if err != nil {
					return 0, 0, fmt.Errorf("failed to copy .%s files to %s (copied %d): %w", ext, defaultDirDstJPG, n, err)
				}
				countJPGToCopy += n
			}
			log.Printf("copied %d JPG files to %s\n", countJPGToCopy, defaultDirDstJPG)
			totalCopied += countJPGToCopy
		}
	}

	// remove source files
	removedCount := 0
	if !flagDryRun {
		var err error
		removedCount, err = removeFiles(dirSrc)
		if err != nil {
			return totalCopied, removedCount, fmt.Errorf("failed to remove source files: %w", err)
		}
	}

	// delete zombie edit files
	if !flagDryRun && flagDeleteZombieEditFiles {
		for _, editFileExtension := range editFileExtensions {
			count, err := deleteZombieEditFiles(editFileExtension, dirDst, extensionsToCopy, true)
			if err != nil {
				return totalCopied, removedCount, fmt.Errorf("failed to delete zombie edit files with extension %s: %w", editFileExtension, err)
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
	return fmt.Sprintf("failed to copy file %s: %s", e.fileName, e.err.Error())
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
					log.Printf("skipping copying existing file: %s\n", name)
					return
				}
			}

			if flagDryRun {
				log.Printf("[dry-run] would copy %s\n", name)
				count.Add(1)
				return
			}

			if copyErr := copyFile(srcPath, dstPath); copyErr != nil {
				errsChan <- fileCopyError{fileName: name, err: copyErr}
				return
			}

			log.Printf("copied %s\n", name)
			count.Add(1)
		})
	}

	wg.Wait()
	close(errsChan)

	for e := range errsChan {
		err = errors.Join(err, e)
	}

	return int(count.Load()), err
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

	var count atomic.Int32
	var wg sync.WaitGroup
	errsChan := make(chan error, len(entries))

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		wg.Go(func() {
			path := filepath.Join(dir, entry.Name())
			if err := os.Remove(path); err != nil {
				errsChan <- fmt.Errorf("failed to remove file %s: %w", entry.Name(), err)
				return
			}
			log.Printf("removed %s\n", entry.Name())
			count.Add(1)
		})
	}

	wg.Wait()
	close(errsChan)

	for e := range errsChan {
		err = errors.Join(err, e)
	}

	return int(count.Load()), err
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
						errsChan <- fmt.Errorf("failed to process subdirectory %s: %w", entry.Name(), err)
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

			editFileNameWithoutExt := strings.TrimSuffix(editFileName, "."+editFileExtension)
			hasCorrespondingRawFile := false

			for _, rawFileExt := range rawFileExtensions {
				expectedRawFileName := editFileNameWithoutExt + "." + rawFileExt
				if _, err := os.Stat(filepath.Join(dir, expectedRawFileName)); err == nil {
					hasCorrespondingRawFile = true
					break
				} else if !errors.Is(err, os.ErrNotExist) {
					errsChan <- fmt.Errorf("failed to check if %s exists: %w", expectedRawFileName, err)
					return
				}
			}

			if hasCorrespondingRawFile {
				return
			}

			if err := os.Remove(filepath.Join(dir, editFileName)); err != nil {
				errsChan <- fmt.Errorf("failed to remove zombie edit file %s: %w", editFileName, err)
				return
			}

			log.Printf("removed zombie edit file: %s\n", editFileName)
			count.Add(1)
		})
	}

	wg.Wait()
	close(errsChan)

	for e := range errsChan {
		err = errors.Join(err, e)
	}

	return int(count.Load()), err
}

func countFilesWithExtension(dir, ext string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, fmt.Errorf("failed to read directory %s: %w", dir, err)
	}

	var count atomic.Uint32
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.EqualFold(filepath.Ext(entry.Name()), "."+ext) {
			count.Add(1)
		}
	}

	return int(count.Load()), nil
}
