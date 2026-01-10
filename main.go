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
		flagDryRun, flagOverwrite, flagDeleteZombieEditFiles bool
		dirSrc, dirDst                                       string

		// put pictures in a new directory when more than N pictures were taken in the same day.
		minDailyPhotosForDir int
		// minDailyPhotosForEvent is the threshold for detecting a multi-day shooting event.
		// When 2 or more consecutive days each have N or more photos, all photos from
		// those consecutive days are grouped together and moved into a new event directory.
		minDailyPhotosForEvent int
	)

	flag.BoolVar(&flagDryRun, "dry-run", false, "Simulate operations without modifying files (default: false)")
	flag.BoolVar(&flagOverwrite, "overwrite", false, "Overwrite existing files in destination (default: false)")
	flag.BoolVar(&flagDeleteZombieEditFiles, "delete-zombie-edit-files", true, "Delete zombie edit files (default: true)")
	flag.StringVar(&dirSrc, "src", defaultDirSrc, "Source directory")
	flag.StringVar(&dirDst, "dst", defaultDirDst, "Destination directory")
	flag.IntVar(&minDailyPhotosForDir, "min-photos-per-day-to-create-new-dir", defaultMinDailyPhotosForDir, "Minimum number of photos per day to create a new directory")
	flag.IntVar(&minDailyPhotosForEvent, "min-photos-per-day-for-event-to-create-new-dir", defaultMinDailyPhotosForEvent, "Minimum number of photos per day for an event to create a new directory")
	flag.IntVar(&minDailyPhotosForEvent, "min-photos-per-day-for-event-to-create-new-dir", defaultMinDailyPhotosForEvent, "Minimum number of photos per day for an event to create a new directory")

	if flagDryRun {
		log.Println("Running in Dry-Run mode. No files will be modified.")
	}
	if flagOverwrite {
		log.Println("Running in Overwrite mode. Existing files in destination will be overwritten.")
	} else {
		log.Println("Running in Skip-Existing mode. Existing files in destination will be skipped.")
	}

	totalCopied, removedCount, err := cleanSDCard(EditFileExtensions, ExtensionsToCopy, dirSrc, dirDst, flagDryRun, flagOverwrite, flagDeleteZombieEditFiles)
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
