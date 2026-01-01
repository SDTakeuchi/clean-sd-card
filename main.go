package main

// Sample usage:
//
//	go run main.go -dry-run
//	go run main.go -overwrite
//	go run main.go -dry-run -overwrite

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	dirFrom = "E:\\DCIM\\100MSDCF"
	dirTo   = "D:\\raw"
)

var (
	extensionsToCopy = []string{"arw", "raw"}
	flagDryRun       = false // default: false
	flagOverwrite    = false // default: false; if true, overwrite existing files in dirTo
)

func main() {
	// read options from command line.
	// set flagDryRun and flagOverwrite accordingly.
	flag.BoolVar(&flagDryRun, "dry-run", false, "Simulate operations without modifying files")
	flag.BoolVar(&flagOverwrite, "overwrite", false, "Overwrite existing files in destination")
	flag.Parse()

	if flagDryRun {
		fmt.Println("Running in Dry-Run mode. No files will be modified.")
	}
	if flagOverwrite {
		fmt.Println("Running in Overwrite mode. Existing files in destination will be overwritten.")
	} else {
		fmt.Println("Running in Skip-Existing mode. Existing files in destination will be skipped.")
	}

	// copy files with specified extensions
	// skip execution if flagDryRun is true
	// Note: copyFiles handles dry-run logic internally (counting instead of copying)

	if !flagDryRun {
		if err := os.MkdirAll(dirTo, 0755); err != nil {
			fmt.Printf("Error creating destination directory: %v\n", err)
			return
		}
	}

	totalCopied := 0
	for _, ext := range extensionsToCopy {
		n, err := copyFiles(dirFrom, dirTo, ext, flagDryRun, flagOverwrite)
		if err != nil {
			fmt.Printf("Error processing .%s files: %v\n", ext, err)
		}
		totalCopied += n
	}

	// remove files from dirFrom
	// skip execution if flagDryRun is true
	removedCount := 0
	if !flagDryRun {
		var err error
		removedCount, err = removeFiles(dirFrom)
		if err != nil {
			fmt.Printf("Error removing files: %v\n", err)
		}
	}

	// print summary of operations
	fmt.Printf("\nSummary:\nFiles Copied: %d\nFiles Removed: %d\n", totalCopied, removedCount)
}

// copies files with given extension from dirFrom to dirTo
// if flagDryRun is true, do not perform actual copy, just count files to be copied
// if flagOverwrite is true, overwrite existing files in dirTo
// if flagOverwrite is false, skip files that already exist in dirTo
//
// returns number of files copied and error if any
func copyFiles(dirFrom, dirTo, ext string, flagDryRun, flagOverwrite bool) (int, error) {
	entries, err := os.ReadDir(dirFrom)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.EqualFold(filepath.Ext(name), "."+ext) {
			continue
		}

		srcPath := filepath.Join(dirFrom, name)
		dstPath := filepath.Join(dirTo, name)

		if !flagOverwrite {
			if _, err := os.Stat(dstPath); err == nil {
				fmt.Printf("Skipping copying existing file: %s\n", name)
				continue
			}
		}

		if flagDryRun {
			fmt.Printf("[DryRun] Would copy %s\n", name)
			count++
			continue
		}

		if err := copyFile(srcPath, dstPath); err != nil {
			return count, err
		}
		fmt.Printf("Copied %s\n", name)
		count++
	}
	return count, nil
}

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
