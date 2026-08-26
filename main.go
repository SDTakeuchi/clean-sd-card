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
	"path/filepath"
)

const (
	defaultDirSrc = "E:\\DCIM"
	defaultDirDst = "D:\\raw"

	defaultDirDstJPG = "D:\\jpeg"

	defaultDirVideoSrc = "E:\\PRIVATE\\M4ROOT\\CLIP"
	defaultDirVideoDst = "D:\\movies"

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
		videoExtensions           = []string{"mp4", "xml"}
		opts                      Options
		dirSrc, dirDst, dirDstJPG string
		dirVideoSrc, dirVideoDst  string
	)

	flag.BoolVar(&opts.DryRun, "dry-run", false, "Simulate operations without modifying files (default: false)")
	flag.BoolVar(&opts.Overwrite, "overwrite", false, "Overwrite existing files in destination (default: false)")
	flag.BoolVar(&opts.KeepJPG, "keep-jpg", true, "Keep JPG files in destination (default: true)")
	flag.BoolVar(&opts.KeepSrc, "keep-src", true, "Keep files in the source (SD card) directory after copying instead of removing them (default: true)")
	flag.BoolVar(&opts.DeleteZombieEditFiles, "delete-zombie-edit-files", true, "Delete zombie edit files (default: true)")
	flag.IntVar(&opts.Concurrency, "concurrency", defaultConcurrency, "Maximum number of files to copy/remove concurrently (default: 4). Tune based on your card reader's actual throughput.")
	flag.StringVar(&dirSrc, "src", defaultDirSrc, "Source directory")
	flag.StringVar(&dirDst, "dst", defaultDirDst, "Destination directory")
	flag.StringVar(&dirDstJPG, "dst-jpg", defaultDirDstJPG, "Destination directory for JPG files")
	flag.StringVar(&dirVideoSrc, "video-src", defaultDirVideoSrc, "Source directory for video files")
	flag.StringVar(&dirVideoDst, "video-dst", defaultDirVideoDst, "Destination directory for video files")
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
	} else {
		log.Println("Keep-Src mode disabled. Files in the source directory will be removed after copying.")
	}

	fsys := osFileSystem{}

	totalCopied, removedCount, err := cleanSDCard(
		fsys,
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

	log.Printf("Starting copying video files from %s to %s with extensions %v\n", dirVideoSrc, dirVideoDst, videoExtensions)
	videoCopied, err := copyVideoFiles(
		fsys,
		videoExtensions,
		dirVideoSrc,
		dirVideoDst,
		opts,
	)
	if err != nil {
		log.Fatalf("failed copying video files: %s", err.Error())
	}

	log.Printf("\nSummary:\nFiles Copied: %d\nFiles Removed: %d\n", totalCopied+videoCopied, removedCount)
	logStorageSummary(dirSrc, dirDst, dirDstJPG, dirVideoSrc, dirVideoDst)
}

func getStorageInfoForPath(path string) (storageInfo, error) {
	candidate := filepath.Clean(path)
	for {
		storage, err := getStorageInfo(candidate)
		if err == nil {
			return storage, nil
		}

		parent := filepath.Dir(candidate)
		if parent == candidate {
			return storageInfo{}, err
		}
		candidate = parent
	}
}
func logStorageSummary(paths ...string) {
	log.Println("Storage:")
	seen := make(map[string]bool)
	for _, path := range paths {
		if seen[path] {
			continue
		}
		seen[path] = true

		storage, err := getStorageInfoForPath(path)
		if err != nil {
			log.Printf("%s: unavailable (%v)\n", path, err)
			continue
		}
		log.Printf("%s: total %s, free %s\n", path, formatBytes(storage.Total), formatBytes(storage.Free))
	}
}

func formatBytes(bytes uint64) string {
	const unit = 1024.0
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	}

	value := float64(bytes)
	units := []string{"KB", "MB", "GB", "TB", "PB"}
	for _, unitName := range units {
		value /= unit
		if value < unit {
			return fmt.Sprintf("%.2f %s (%d bytes)", value, unitName, bytes)
		}
	}
	return fmt.Sprintf("%.2f EB (%d bytes)", value, bytes)
}

// copyVideoFiles copies video files from dirSrc to dirDst without removing
// anything from the source directory.
func copyVideoFiles(
	fsys FileSystem,
	videoExtensions []string,
	dirSrc, dirDst string,
	opts Options,
) (int, error) {
	entries, err := fsys.ReadDir(dirSrc)
	if err != nil {
		return 0, fmt.Errorf("failed to read video source directory: %w", err)
	}

	if !opts.DryRun {
		if err := fsys.MkdirAll(dirDst, 0755); err != nil {
			return 0, fmt.Errorf("failed to create video destination directory: %w", err)
		}
	}

	copied, err := copyFiles(fsys, entries, dirSrc, dirDst, videoExtensions, opts.DryRun, opts.Overwrite, opts.Concurrency)
	if err != nil {
		return copied, fmt.Errorf("failed to copy video files with extensions %v (copied %d): %w", videoExtensions, copied, err)
	}

	return copied, nil
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

	// Walk dirSrc once and reuse the result for the raw copy, JPG copy, and
	// removal steps below. Camera folders under DCIM can be incremented (for
	// example, 100MSDCF to 101MSDCF), so every nested directory must be
	// included without re-reading the SD card for each extension group.
	sourceFiles, err := listFilesRecursively(fsys, dirSrc)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to read source directory: %w", err)
	}

	// copy raw files
	totalCopied, err := copySourceFiles(fsys, sourceFiles, dirDst, extensionsToCopy, photoDestinationFileName, opts.DryRun, opts.Overwrite, opts.Concurrency)
	if err != nil {
		return totalCopied, 0, fmt.Errorf("failed to copy files with extensions %v (copied %d): %w", extensionsToCopy, totalCopied, err)
	}

	// copy jpg
	if opts.KeepJPG {
		countJPGToCopy, err := copySourceFiles(fsys, sourceFiles, dirDstJPG, extensionsJPG, photoDestinationFileName, opts.DryRun, opts.Overwrite, opts.Concurrency)
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
		removedCount, err = removeSourceFiles(fsys, sourceFiles, opts.Concurrency)
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
