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
	"time"

	"github.com/rwcarlsen/goexif/exif"
)

// fileWithDate represents a file and its EXIF date
type fileWithDate struct {
	name string
	date time.Time
}

// dateGroup represents files grouped by date
type dateGroup struct {
	date  time.Time
	files []string
}

// copyFiles copies files with the given extension from srcDir to dstDir.
// If flagDryRun is true, it counts files without copying.
// If flagOverwrite is true, it overwrites existing files in dstDir.
// It returns the number of files copied and any error.
func copyFiles(srcDir, dstDir, ext string, folderCreationThresholdOneDay, folderCreationThresholdConsecutiveDays int, flagDryRun, flagOverwrite bool) (int, error) {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return 0, err
	}

	// First pass: read all files and extract EXIF dates
	filesWithDates := make([]fileWithDate, 0)

	var wg sync.WaitGroup
	for _, entry := range entries {
		wg.Go(func() {
			if entry.IsDir() {
				return
			}

			name := entry.Name()
			if !strings.EqualFold(filepath.Ext(name), "."+ext) {
				return
			}

			filePath := filepath.Join(srcDir, name)
			file, err := os.Open(filePath)
			if err != nil {
				log.Printf("Error opening file %s: %v\n", name, err)
				return
			}

			exifData, err := exif.Decode(file)
			file.Close()
			if err != nil {
				log.Printf("Error reading EXIF from %s: %v\n", name, err)
				return
			}

			date, err := exifData.DateTime()
			if err != nil {
				log.Printf("Error getting date from EXIF for %s: %v\n", name, err)
				return
			}

			filesWithDates = append(filesWithDates, fileWithDate{name: name, date: date})
		})
	}
	wg.Wait()

	if len(filesWithDates) == 0 {
		return 0, nil
	}

	// Group files by date (YYYY-MM-DD)
	dateGroups := groupFilesByDate(filesWithDates)

	// Detect consecutive days and merge if needed
	finalGroups := detectConsecutiveDays(dateGroups, folderCreationThresholdOneDay, folderCreationThresholdConsecutiveDays)

	// Copy files concurrently based on groups
	var count atomic.Int32
	errsChan := make(chan fileCopyError, len(filesWithDates))

	for _, group := range finalGroups {
		// Determine the actual destination directory
		var actualDestDir string
		if group.destDir == "." {
			actualDestDir = dstDir
		} else {
			actualDestDir = filepath.Join(dstDir, group.destDir)
			// Create the subdirectory if needed
			if !flagDryRun {
				if err := os.MkdirAll(actualDestDir, 0755); err != nil {
					return int(count.Load()), err
				}
			}
		}

		for _, fileName := range group.files {
			wg.Add(1)
			go func(name, destDir string) {
				defer wg.Done()

				srcPath := filepath.Join(srcDir, name)
				dstPath := filepath.Join(destDir, name)

				if !flagOverwrite {
					if _, statErr := os.Stat(dstPath); statErr == nil {
						log.Printf("Skipping copying existing file: %s\n", name)
						return
					}
				}

				if flagDryRun {
					log.Printf("[DryRun] Would copy %s to %s\n", name, destDir)
					count.Add(1)
					return
				}

				if copyErr := copyFile(srcPath, dstPath); copyErr != nil {
					errsChan <- fileCopyError{fileName: name, err: copyErr}
					return
				}

				log.Printf("Copied %s to %s\n", name, destDir)
				count.Add(1)
			}(fileName, actualDestDir)
		}
	}

	wg.Wait()
	close(errsChan)

	var errs error
	for e := range errsChan {
		errs = errors.Join(errs, e)
	}

	return int(count.Load()), errs
}

// groupFilesByDate groups files by their date (YYYY-MM-DD)
func groupFilesByDate(filesWithDates []fileWithDate) []dateGroup {
	dateMap := make(map[string]*dateGroup)

	for _, fwd := range filesWithDates {
		dateKey := fwd.date.Format(dateFormat)
		if group, ok := dateMap[dateKey]; ok {
			group.files = append(group.files, fwd.name)
		} else {
			dateMap[dateKey] = &dateGroup{
				date:  fwd.date,
				files: []string{fwd.name},
			}
		}
	}

	groups := make([]dateGroup, 0, len(dateMap))
	for _, group := range dateMap {
		groups = append(groups, *group)
	}

	// Sort groups by date
	for i := range len(groups) {
		for j := range len(groups) {
			if groups[i].date.After(groups[j].date) {
				groups[i], groups[j] = groups[j], groups[i]
			}
		}
	}

	return groups
}

// groupWithDestDir represents a group of files with their destination directory
type groupWithDestDir struct {
	files   []string
	destDir string
}

// detectConsecutiveDays analyzes date groups and creates appropriate folder structure
func detectConsecutiveDays(dateGroups []dateGroup, thresholdOneDay, thresholdConsecutiveDays int) []groupWithDestDir {
	result := make([]groupWithDestDir, 0)

	i := 0
	for i < len(dateGroups) {
		currentGroup := dateGroups[i]
		fileCount := len(currentGroup.files)

		// First, check if current day qualifies for consecutive day detection (>= thresholdConsecutiveDays)
		if fileCount >= thresholdConsecutiveDays {
			// Look ahead for consecutive days with >= thresholdConsecutiveDays photos per day
			consecutiveGroups := []dateGroup{currentGroup}
			j := i + 1

			for j < len(dateGroups) {
				nextGroup := dateGroups[j]
				lastGroup := consecutiveGroups[len(consecutiveGroups)-1]

				// Check if dates are consecutive and next day has enough photos
				if isConsecutiveDay(lastGroup.date, nextGroup.date) && len(nextGroup.files) >= thresholdConsecutiveDays {
					consecutiveGroups = append(consecutiveGroups, nextGroup)
					j++
				} else {
					break
				}
			}

			// If we have 2 or more consecutive days with >= thresholdConsecutiveDays photos each, group them
			if len(consecutiveGroups) >= 2 {
				allFiles := make([]string, 0)
				for _, g := range consecutiveGroups {
					allFiles = append(allFiles, g.files...)
				}

				firstDate := consecutiveGroups[0].date
				lastDate := consecutiveGroups[len(consecutiveGroups)-1].date
				destDir := firstDate.Format("20060102") + "-" + lastDate.Format("20060102")

				result = append(result, groupWithDestDir{
					files:   allFiles,
					destDir: destDir,
				})

				i = j
				continue
			}
		}

		// If not part of consecutive days, check if it's a high-volume single day
		if fileCount >= thresholdOneDay {
			// Single high-volume day, create folder for this day
			destDir := currentGroup.date.Format("20060102")
			result = append(result, groupWithDestDir{
				files:   currentGroup.files,
				destDir: destDir,
			})
			i++
		} else {
			// Low-volume day, files go to root destination directory
			result = append(result, groupWithDestDir{
				files:   currentGroup.files,
				destDir: ".",
			})
			i++
		}
	}

	return result
}

// isConsecutiveDay checks if date2 is the day after date1
func isConsecutiveDay(date1, date2 time.Time) bool {
	nextDay := date1.AddDate(0, 0, 1)
	y1, m1, d1 := nextDay.Date()
	y2, m2, d2 := date2.Date()
	return y1 == y2 && m1 == m2 && d1 == d2
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
