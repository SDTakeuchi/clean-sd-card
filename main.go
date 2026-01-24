package main

// Sample usage:
//
//	go run . -dry-run
//	go run . -overwrite
//	go run . -dry-run -overwrite

import (
	"flag"
	"fmt"
	"log"
	"os"
)

func main() {
	var (
		flagDryRun                bool
		flagOverwrite             bool
		flagKeepJPGs              bool
		flagDeleteZombieEditFiles bool
		dirSrc                    string
		dirDst                    string
		dirDstJPGs                string // Directory to store JPG files

		/// folderCreationThresholdOneDay is the minimum number of photos taken on the same date
		// required to create a separate folder for that date
		folderCreationThresholdOneDay int

		// folderCreationThresholdConsecutiveDays is the minimum number of consecutive days
		// where photo count exceeds folderCreationThresholdOneDay required to create
		// a separate folder grouping those days together
		folderCreationThresholdConsecutiveDays int
	)

	flag.BoolVar(&flagDryRun, "dry-run", false, "Simulate operations without modifying files (default: false)")
	flag.BoolVar(&flagOverwrite, "overwrite", false, "Overwrite existing files in destination (default: false)")
	flag.BoolVar(&flagKeepJPGs, "keep-jpgs", true, "Keep JPG files in destination (default: true)")
	flag.BoolVar(&flagDeleteZombieEditFiles, "delete-zombie-edit-files", true, "Delete zombie edit files (default: true)")

	flag.StringVar(&dirSrc, "src", defaultDirSrc, "Source directory")
	flag.StringVar(&dirDst, "dst", defaultDirDst, "Destination directory")
	flag.StringVar(&dirDstJPGs, "dst-jpg", defaultDirDstJPGs, "Destination directory for JPG files")
	flag.IntVar(&folderCreationThresholdOneDay, "folder-creation-threshold-one-day", defaultFolderCreationThresholdOneDay, "Threshold for creating a new directory in one day")
	flag.IntVar(&folderCreationThresholdConsecutiveDays, "folder-creation-threshold-consecutive-days", defaultFolderCreationThresholdConsecutiveDays, "Threshold for creating a new directory in consecutive days")

	flag.Parse()

	if flagDryRun {
		log.Println("Running in Dry-Run mode. No files will be modified.")
	}
	if flagOverwrite {
		log.Println("Running in Overwrite mode. Existing files in destination will be overwritten.")
	} else {
		log.Println("Running in Skip-Existing mode. Existing files in destination will be skipped.")
	}

	totalCopied, removedCount, err := cleanSDCard(
		EditFileExtensions,
		ExtensionsToCopy,
		ExtensionsJPG,
		dirSrc,
		dirDst,
		dirDstJPGs,
		folderCreationThresholdOneDay,
		folderCreationThresholdConsecutiveDays,
		flagDryRun,
		flagOverwrite,
		flagDeleteZombieEditFiles,
		flagKeepJPGs,
	)
	if err != nil {
		log.Fatalf("Error cleaning SD card: %s", err.Error())
	}

	log.Printf("\nSummary:\nFiles Copied: %d\nFiles Removed: %d\n", totalCopied, removedCount)
}

// cleanSDCard copies files from dirSrc to dirDst and removes files from dirSrc.
// It returns the number of files copied, the number of files removed, and any error.
func cleanSDCard(editFileExtensions, extensionsToCopy, extensionsJPG []string, dirSrc, dirDst, dirDstJPGs string, folderCreationThresholdOneDay int, folderCreationThresholdConsecutiveDays int, flagDryRun, flagOverwrite, flagDeleteZombieEditFiles, flagKeepJPGs bool) (int, int, error) {
	if !flagDryRun {
		if err := os.MkdirAll(dirDst, 0755); err != nil {
			return 0, 0, fmt.Errorf("creating destination directory: %w", err)
		}
	}

	totalCopied := 0
	for _, ext := range extensionsToCopy {
		n, err := copyFiles(dirSrc, dirDst, ext, folderCreationThresholdOneDay, folderCreationThresholdConsecutiveDays, flagDryRun, flagOverwrite)
		if err != nil {
			return totalCopied, 0, fmt.Errorf("processing .%s files (copied %d): %w", ext, n, err)
		}
		totalCopied += n
	}

	// Process JPG files if flagKeepJPGs is false
	if !flagKeepJPGs {
		if !flagDryRun {
			if err := os.MkdirAll(dirDstJPGs, 0755); err != nil {
				return totalCopied, 0, fmt.Errorf("creating JPG destination directory: %w", err)
			}
		}

		for _, ext := range extensionsJPG {
			n, err := copyFiles(dirSrc, dirDstJPGs, ext, folderCreationThresholdOneDay, folderCreationThresholdConsecutiveDays, flagDryRun, flagOverwrite)
			if err != nil {
				return totalCopied, 0, fmt.Errorf("processing .%s files (copied %d): %w", ext, n, err)
			}
			totalCopied += n
		}
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
