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
)

const (
	defaultDirSrc = "E:\\DCIM\\100MSDCF"
	defaultDirDst = "D:\\raw"

	defaultDirDstJPG = "D:\\jpeg"

	// defaultConcurrency caps how many files are copied/removed at once.
	// dirSrc is typically an SD card behind a single physical read channel,
	// so unbounded per-file concurrency doesn't help throughput and can hurt
	// it (more random access, more scheduling overhead). This is a starting
	// point, not a measured optimum -- tune with -concurrency.
	defaultConcurrency = 4
)

// Options holds the flags that control cleanSDCard's behavior.
type Options struct {
	DryRun                bool
	KeepJPG               bool
	KeepSrc               bool
	Overwrite             bool
	DeleteZombieEditFiles bool
	Concurrency           int
}

func main() {
	var (
		editFileExtensions        = []string{"xmp"} // lightroom's default edit file extension when edited in local machine
		extensionsToCopy          = []string{"arw", "raw"}
		extensionsJPG             = []string{"jpg", "jpeg"}
		opts                      Options
		dirSrc, dirDst, dirDstJPG string
	)

	flag.BoolVar(&opts.DryRun, "dry-run", false, "Simulate operations without modifying files (default: false)")
	flag.BoolVar(&opts.Overwrite, "overwrite", false, "Overwrite existing files in destination (default: false)")
	flag.BoolVar(&opts.KeepJPG, "keep-jpg", true, "Keep JPG files in destination (default: true)")
	flag.BoolVar(&opts.KeepSrc, "keep-src", false, "Keep files in the source (SD card) directory after copying instead of removing them (default: false)")
	flag.BoolVar(&opts.DeleteZombieEditFiles, "delete-zombie-edit-files", true, "Delete zombie edit files (default: true)")
	flag.IntVar(&opts.Concurrency, "concurrency", defaultConcurrency, "Maximum number of files to copy/remove concurrently (default: 4). Tune based on your card reader's actual throughput.")
	flag.StringVar(&dirSrc, "src", defaultDirSrc, "Source directory")
	flag.StringVar(&dirDst, "dst", defaultDirDst, "Destination directory")
	flag.StringVar(&dirDstJPG, "dst-jpg", defaultDirDstJPG, "Destination directory for JPG files")
	flag.Parse()

	log.Printf("Starting copying files from %s to %s with extensions %v\n", dirSrc, dirDst, extensionsToCopy)
	if opts.DryRun {
		log.Println("Running in Dry-Run mode. No files will be modified.")
	}
	if opts.Overwrite {
		log.Println("Running in Overwrite mode. Existing files in destination will be overwritten.")
	} else {
		log.Println("Running in Skip-Existing mode. Existing files in destination will be skipped.")
	}
	if opts.KeepSrc {
		log.Println("Running in Keep-Src mode. Files in the source directory will not be removed.")
	}

	totalCopied, removedCount, err := cleanSDCard(
		osFileSystem{},
		editFileExtensions,
		extensionsToCopy,
		extensionsJPG,
		dirSrc,
		dirDst,
		dirDstJPG,
		opts,
	)
	if err != nil {
		log.Fatalf("failed cleaning SD card: %s", err.Error())
	}

	log.Printf("\nSummary:\nFiles Copied: %d\nFiles Removed: %d\n", totalCopied, removedCount)
}

// cleanSDCard copies files from dirSrc to dirDst and removes files from dirSrc.
// It returns the number of files copied, the number of files removed, and any error.
func cleanSDCard(
	fsys FileSystem,
	editFileExtensions, extensionsToCopy, extensionsJPG []string,
	dirSrc, dirDst, dirDstJPG string,
	opts Options,
) (int, int, error) {
	if !opts.DryRun {
		if err := fsys.MkdirAll(dirDst, 0755); err != nil {
			return 0, 0, fmt.Errorf("failed to create destination directory: %w", err)
		}
	}

	// List dirSrc once and reuse it for the raw copy, JPG copy, and removal
	// steps below, instead of re-reading it once per extension group. dirSrc
	// is typically a slow SD card, so this avoids redundant directory reads
	// against it.
	entries, err := fsys.ReadDir(dirSrc)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to read source directory: %w", err)
	}

	// copy raw files
	totalCopied, err := copyFiles(fsys, entries, dirSrc, dirDst, extensionsToCopy, opts.DryRun, opts.Overwrite, opts.Concurrency)
	if err != nil {
		return totalCopied, 0, fmt.Errorf("failed to copy files with extensions %v (copied %d): %w", extensionsToCopy, totalCopied, err)
	}

	// copy jpg
	if opts.KeepJPG {
		countJPGToCopy, err := copyFiles(fsys, entries, dirSrc, dirDstJPG, extensionsJPG, opts.DryRun, opts.Overwrite, opts.Concurrency)
		if err != nil {
			return totalCopied, 0, fmt.Errorf("failed to copy JPG files to %s (copied %d): %w", dirDstJPG, countJPGToCopy, err)
		}
		if opts.DryRun {
			log.Printf("[dry-run] would copy %d JPG files\n", countJPGToCopy)
		} else {
			log.Printf("copied %d JPG files to %s\n", countJPGToCopy, dirDstJPG)
		}
		totalCopied += countJPGToCopy
	}

	// remove source files
	removedCount := 0
	if !opts.DryRun && !opts.KeepSrc {
		removedCount, err = removeFiles(fsys, entries, dirSrc, opts.Concurrency)
		if err != nil {
			return totalCopied, removedCount, fmt.Errorf("failed to remove source files: %w", err)
		}
	}

	// delete zombie edit files
	if !opts.DryRun && opts.DeleteZombieEditFiles {
		for _, editFileExtension := range editFileExtensions {
			count, err := deleteZombieEditFiles(fsys, editFileExtension, dirDst, extensionsToCopy, true, opts.Concurrency)
			if err != nil {
				return totalCopied, removedCount, fmt.Errorf("failed to delete zombie edit files with extension %s: %w", editFileExtension, err)
			}
			removedCount += count
		}
	}

	return totalCopied, removedCount, nil
}
