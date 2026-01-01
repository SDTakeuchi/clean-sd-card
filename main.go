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
		extensionsToCopy          = []string{"arw", "raw"}
		flagDryRun, flagOverwrite bool
		dirSrc, dirDst            string
	)

	// read options from command line.
	// set flagDryRun and flagOverwrite accordingly.
	flag.BoolVar(&flagDryRun, "dry-run", false, "Simulate operations without modifying files")
	flag.BoolVar(&flagOverwrite, "overwrite", false, "Overwrite existing files in destination")
	flag.StringVar(&dirSrc, "src", defaultDirSrc, "Source directory")
	flag.StringVar(&dirDst, "dst", defaultDirDst, "Destination directory")
	flag.Parse()

	if flagDryRun {
		fmt.Println("Running in Dry-Run mode. No files will be modified.")
	}
	if flagOverwrite {
		fmt.Println("Running in Overwrite mode. Existing files in destination will be overwritten.")
	} else {
		fmt.Println("Running in Skip-Existing mode. Existing files in destination will be skipped.")
	}

	totalCopied, removedCount, err := cleanSDCard(extensionsToCopy, dirSrc, dirDst, flagDryRun, flagOverwrite)
	if err != nil {
		log.Fatalf("Error cleaning SD card: %s", err.Error())
	}

	// print summary of operations
	fmt.Printf("\nSummary:\nFiles Copied: %d\nFiles Removed: %d\n", totalCopied, removedCount)
}

// cleans the SD card by copying files from dirSrc to dirDst and removing files from dirSrc
// returns number of files copied and number of files removed and error if any
func cleanSDCard(extensionsToCopy []string, dirSrc, dirDst string, flagDryRun, flagOverwrite bool) (int, int, error) {
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
					fmt.Printf("Skipping copying existing file: %s\n", name)
					return
				}
			}

			if flagDryRun {
				fmt.Printf("[DryRun] Would copy %s\n", name)
				count.Add(1)
				return
			}

			if err = copyFile(srcPath, dstPath); err != nil {
				errsChan <- fileCopyError{fileName: name, err: err}
				return
			}

			fmt.Printf("Copied %s\n", name)
			count.Add(1)
		})
	}

	wg.Wait()

	if len(errsChan) > 0 {
		var builder strings.Builder
		for e := range errsChan {
			builder.WriteString(e.Error())
		}
		err = errors.New(builder.String())
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
		fmt.Printf("Removed %s\n", entry.Name())
		count++
	}
	return count, nil
}
