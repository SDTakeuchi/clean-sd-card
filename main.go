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
		editFileExtensions                                   = []string{"xmp"} // lightroom's default edit file extension when editted in local machine
		extensionsToCopy                                     = []string{"arw", "raw"}
		flagDryRun, flagOverwrite, flagDeleteZombieEditFiles bool
		dirSrc, dirDst                                       string
	)

	// read options from command line.
	// set flagDryRun and flagOverwrite accordingly.
	flag.BoolVar(&flagDryRun, "dry-run", false, "Simulate operations without modifying files (default: false)")
	flag.BoolVar(&flagOverwrite, "overwrite", false, "Overwrite existing files in destination (default: false)")
	flag.BoolVar(&flagDeleteZombieEditFiles, "delete-zombie-edit-files", true, "Delete zombie edit files (default: true)")
	flag.StringVar(&dirSrc, "src", defaultDirSrc, "Source directory")
	flag.StringVar(&dirDst, "dst", defaultDirDst, "Destination directory")
	flag.Parse()

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

	// print summary of operations
	log.Printf("\nSummary:\nFiles Copied: %d\nFiles Removed: %d\n", totalCopied, removedCount)
}

// cleans the SD card by copying files from dirSrc to dirDst and removing files from dirSrc
// returns number of files copied and number of files removed and error if any
func cleanSDCard(editFileExtensions, extensionsToCopy []string, dirSrc, dirDst string, flagDryRun, flagOverwrite, flagDeleteZombieEditFiles bool) (int, int, error) {
	// copy files with specified extensions
	// skip execution if flagDryRun is true
	// Note: copyFiles handles dry-run logic internally (counting instead of copying)
	if !flagDryRun {
		if err := os.MkdirAll(dirDst, 0755); err != nil {
			return 0, 0, fmt.Errorf("Error creating destination directory: %w", err)
		}
	}

	totalCopied := 0
	for _, ext := range extensionsToCopy {
		n, err := copyFiles(dirSrc, dirDst, ext, flagDryRun, flagOverwrite)
		if err != nil {
			log.Fatalf(`Error processing .%s
files_copied: %d
errors:\n%s`,
				ext,
				n,
				err.Error())
		}
		totalCopied += n
	}

	// remove files from dirSrc
	// skip execution if flagDryRun is true
	removedCount := 0
	if !flagDryRun {
		var err error
		removedCount, err = removeFiles(dirSrc)
		if err != nil {
			return totalCopied, removedCount, fmt.Errorf("Error removing files: %w", err)
		}
	}

	// delete zombie edit files
	// skip execution if flagDryRun is true
	if !flagDryRun {
		for _, editFileExtension := range editFileExtensions {
			count, err := deleteZombieEditFiles(editFileExtension, dirSrc, extensionsToCopy, flagDeleteZombieEditFiles)
			if err != nil {
				return totalCopied, removedCount, fmt.Errorf("Error deleting zombie edit files with extension %s: %w", editFileExtension, err)
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
	return fmt.Sprintf("error copying file %s: %s\n", e.fileName, e.err.Error())
}

// copies files with given extension from dirSrc to dirDst
// if flagDryRun is true, do not perform actual copy, just count files to be copied
// if flagOverwrite is true, overwrite existing files in dirDst
// if flagOverwrite is false, skip files that already exist in dirDst
//
// returns number of files copied and error if any
func copyFiles(srcDir, dstDir, ext string, flagDryRun, flagOverwrite bool) (int, error) {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return 0, err
	}

	var count atomic.Int32
	var wg sync.WaitGroup
	errsChan := make(chan fileCopyError)

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
				if _, err = os.Stat(dstPath); err == nil {
					log.Printf("Skipping copying existing file: %s\n", name)
					return
				}
			}

			if flagDryRun {
				log.Printf("[DryRun] Would copy %s\n", name)
				count.Add(1)
				return
			}

			if err = copyFile(srcPath, dstPath); err != nil {
				errsChan <- fileCopyError{fileName: name, err: err}
				return
			}

			log.Printf("Copied %s\n", name)
			count.Add(1)
		})
	}

	wg.Wait()

	if len(errsChan) > 0 {
		for e := range errsChan {
			err = errors.Join(err, e)
		}
	}

	return int(count.Load()), err
}

// copies a file from src dir to dst dir
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

// removes all files in the directory
// returns number of files removed and error if any
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

// deletes all zombie Edit files (no arw/raw files with the same name exists) in the directory
//
// editFileExtension: extension of edit files to delete
// dir: directory to delete zombie Edit files from
// isRecursive: if true, delete zombie Edit files in subdirectories recursively
// returns number of files deleted and error if any
func deleteZombieEditFiles(editFileExtension, dir string, rawFileExtensions []string, isRecursive bool) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, fmt.Errorf("error reading directory: %w", err)
	}

	var count atomic.Int32
	var wg sync.WaitGroup
	var errsChan chan error

	for _, entry := range entries {
		wg.Go(func() {
			if entry.IsDir() {
				if isRecursive {
					n, err := deleteZombieEditFiles(editFileExtension, filepath.Join(dir, entry.Name()), rawFileExtensions, isRecursive)
					if err != nil {
						errsChan <- fmt.Errorf("error deleting zombie edit files in subdirectory with name %s: %w", entry.Name(), err)
					}
					count.Add(int32(n))
				}
				return
			}

			editFileName := entry.Name()
			if !strings.HasSuffix(editFileName, "."+editFileExtension) {
				return
			}

			// Check if any corresponding raw file exists
			extTrimmed := strings.TrimSuffix(editFileName, "."+editFileExtension)
			hasCorrespondingRawFile := false

			for _, rawFileExt := range rawFileExtensions {
				expectedRawFileName := extTrimmed + "." + rawFileExt

				_, err := os.Stat(filepath.Join(dir, expectedRawFileName))
				if err == nil {
					// Raw file exists, this is not a zombie
					hasCorrespondingRawFile = true
					break
				}
				if !errors.Is(err, os.ErrNotExist) {
					// unexpected error, return error
					errsChan <- fmt.Errorf("error checking if %s exists: %w", expectedRawFileName, err)
					return
				}
			}

			if hasCorrespondingRawFile {
				// Not a zombie, skip deletion
				return
			}

			// No corresponding raw file found, delete the zombie edit file
			if err := os.Remove(filepath.Join(dir, editFileName)); err != nil {
				errsChan <- fmt.Errorf("error removing zombie edit file: %s: %w", editFileName, err)
				return
			}

			log.Printf("Removed zombie edit file: %s\n", editFileName)
			count.Add(1)
		})
	}

	wg.Wait()

	if len(errsChan) > 0 {
		for e := range errsChan {
			err = errors.Join(err, e)
		}
	}

	return int(count.Load()), err
}
