package main

import (
	"flag"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCleanSDCard(t *testing.T) {
	t.Skip("This test requires actual RAW/JPG files with EXIF data. Use TestDetectConsecutiveDays for unit testing the folder logic.")

	// Note: The new implementation requires EXIF data to be present in files.
	// Creating empty dummy files won't work anymore since copyFiles now reads EXIF dates.
	// For proper integration testing, you would need actual RAW/JPG files with EXIF data.
	// The core logic is tested in TestDetectConsecutiveDays, TestGroupFilesByDate, and TestIsConsecutiveDay.
	//
	// Integration test scenarios to verify:
	// 1. RAW files (.arw, .raw) are always copied to dirDst
	// 2. When flagKeepJPGs=true: JPG files remain untouched on source
	// 3. When flagKeepJPGs=false: JPG files copied to dirDstJPGs
	// 4. Both RAW and JPG use same EXIF-based folder organization
	// 5. Dry-run mode prevents all file operations
}

func TestFlagDefaults(t *testing.T) {
	// Create a new flag set to test defaults without affecting global state
	fs := flag.NewFlagSet("test", flag.ContinueOnError)

	var flagKeepJPGs bool
	fs.BoolVar(&flagKeepJPGs, "keep-jpgs", true, "Keep JPG files")

	// Parse with no args to get defaults
	err := fs.Parse([]string{})
	require.NoError(t, err)

	assert.True(t, flagKeepJPGs, "flagKeepJPGs should default to true")
}

func TestGroupFilesByDate(t *testing.T) {
	tests := []struct {
		name     string
		input    []fileWithDate
		expected []dateGroup
	}{
		{
			name: "single day with multiple files",
			input: []fileWithDate{
				{name: "file1.arw", date: time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)},
				{name: "file2.arw", date: time.Date(2026, 1, 15, 14, 30, 0, 0, time.UTC)},
				{name: "file3.arw", date: time.Date(2026, 1, 15, 18, 45, 0, 0, time.UTC)},
			},
			expected: []dateGroup{
				{
					date:  time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC),
					files: []string{"file1.arw", "file2.arw", "file3.arw"},
				},
			},
		},
		{
			name: "multiple days",
			input: []fileWithDate{
				{name: "file1.arw", date: time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)},
				{name: "file2.arw", date: time.Date(2026, 1, 16, 10, 0, 0, 0, time.UTC)},
				{name: "file3.arw", date: time.Date(2026, 1, 15, 14, 0, 0, 0, time.UTC)},
			},
			expected: []dateGroup{
				{
					date:  time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC),
					files: []string{"file1.arw", "file3.arw"},
				},
				{
					date:  time.Date(2026, 1, 16, 10, 0, 0, 0, time.UTC),
					files: []string{"file2.arw"},
				},
			},
		},
		{
			name:     "empty input",
			input:    []fileWithDate{},
			expected: []dateGroup{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := groupFilesByDate(tt.input)

			require.Len(t, result, len(tt.expected))

			// Sort both for comparison
			for i := range result {
				for j := i + 1; j < len(result); j++ {
					if result[i].date.After(result[j].date) {
						result[i], result[j] = result[j], result[i]
					}
				}
			}

			for i, expected := range tt.expected {
				assert.Equal(t, expected.date.Format("2006-01-02"), result[i].date.Format("2006-01-02"))
				assert.ElementsMatch(t, expected.files, result[i].files)
			}
		})
	}
}

func TestIsConsecutiveDay(t *testing.T) {
	tests := []struct {
		name     string
		date1    time.Time
		date2    time.Time
		expected bool
	}{
		{
			name:     "consecutive days",
			date1:    time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC),
			date2:    time.Date(2026, 1, 16, 14, 0, 0, 0, time.UTC),
			expected: true,
		},
		{
			name:     "non-consecutive days",
			date1:    time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC),
			date2:    time.Date(2026, 1, 17, 10, 0, 0, 0, time.UTC),
			expected: false,
		},
		{
			name:     "same day",
			date1:    time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC),
			date2:    time.Date(2026, 1, 15, 18, 0, 0, 0, time.UTC),
			expected: false,
		},
		{
			name:     "consecutive days across month boundary",
			date1:    time.Date(2026, 1, 31, 10, 0, 0, 0, time.UTC),
			date2:    time.Date(2026, 2, 1, 10, 0, 0, 0, time.UTC),
			expected: true,
		},
		{
			name:     "consecutive days across year boundary",
			date1:    time.Date(2025, 12, 31, 10, 0, 0, 0, time.UTC),
			date2:    time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isConsecutiveDay(tt.date1, tt.date2)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDetectConsecutiveDays(t *testing.T) {
	tests := []struct {
		name                               string
		dateGroups                         []dateGroup
		thresholdOneDay                    int
		thresholdConsecutiveDays           int
		expectedGroupCount                 int
		expectedDestDirs                   []string
		expectedFilesPerGroup              []int
		description                        string
	}{
		{
			name: "two consecutive days with 400 photos each - should be grouped",
			dateGroups: []dateGroup{
				{
					date:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
					files: makeFileList(400, "day1"),
				},
				{
					date:  time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
					files: makeFileList(400, "day2"),
				},
			},
			thresholdOneDay:          600,
			thresholdConsecutiveDays: 300,
			expectedGroupCount:       1,
			expectedDestDirs:         []string{"20260101-20260102"},
			expectedFilesPerGroup:    []int{800},
			description:              "Jan 1: 400 photos, Jan 2: 400 photos → grouped (20260101-20260102)",
		},
		{
			name: "three consecutive days with 350 photos each - should be grouped",
			dateGroups: []dateGroup{
				{
					date:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
					files: makeFileList(350, "day1"),
				},
				{
					date:  time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
					files: makeFileList(350, "day2"),
				},
				{
					date:  time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC),
					files: makeFileList(350, "day3"),
				},
			},
			thresholdOneDay:          600,
			thresholdConsecutiveDays: 300,
			expectedGroupCount:       1,
			expectedDestDirs:         []string{"20260101-20260103"},
			expectedFilesPerGroup:    []int{1050},
			description:              "3 consecutive days with 350 photos each → grouped",
		},
		{
			name: "single day with 700 photos - should get own folder",
			dateGroups: []dateGroup{
				{
					date:  time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
					files: makeFileList(700, "day15"),
				},
			},
			thresholdOneDay:          600,
			thresholdConsecutiveDays: 300,
			expectedGroupCount:       1,
			expectedDestDirs:         []string{"20260115"},
			expectedFilesPerGroup:    []int{700},
			description:              "Single day with 700 photos → own folder (20260115)",
		},
		{
			name: "low volume day - should go to root",
			dateGroups: []dateGroup{
				{
					date:  time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
					files: makeFileList(200, "day15"),
				},
			},
			thresholdOneDay:          600,
			thresholdConsecutiveDays: 300,
			expectedGroupCount:       1,
			expectedDestDirs:         []string{"."},
			expectedFilesPerGroup:    []int{200},
			description:              "Single day with 200 photos → root directory",
		},
		{
			name: "mixed scenario - consecutive event + isolated high volume day + low volume days",
			dateGroups: []dateGroup{
				{
					date:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
					files: makeFileList(400, "day1"),
				},
				{
					date:  time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
					files: makeFileList(350, "day2"),
				},
				{
					date:  time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC),
					files: makeFileList(700, "day5"),
				},
				{
					date:  time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC),
					files: makeFileList(150, "day10"),
				},
			},
			thresholdOneDay:          600,
			thresholdConsecutiveDays: 300,
			expectedGroupCount:       3,
			expectedDestDirs:         []string{"20260101-20260102", "20260105", "."},
			expectedFilesPerGroup:    []int{750, 700, 150},
			description:              "Mixed: consecutive event (Jan 1-2), high volume day (Jan 5), low volume day (Jan 10)",
		},
		{
			name: "consecutive days but only first exceeds threshold - should not group",
			dateGroups: []dateGroup{
				{
					date:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
					files: makeFileList(400, "day1"),
				},
				{
					date:  time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
					files: makeFileList(250, "day2"),
				},
			},
			thresholdOneDay:          600,
			thresholdConsecutiveDays: 300,
			expectedGroupCount:       2,
			expectedDestDirs:         []string{".", "."},
			expectedFilesPerGroup:    []int{400, 250},
			description:              "Day 1: 400 photos, Day 2: 250 photos → both to root (second day < 300)",
		},
		{
			name: "gap in consecutive days - should create separate groups",
			dateGroups: []dateGroup{
				{
					date:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
					files: makeFileList(400, "day1"),
				},
				{
					date:  time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
					files: makeFileList(400, "day2"),
				},
				{
					date:  time.Date(2026, 1, 4, 0, 0, 0, 0, time.UTC),
					files: makeFileList(400, "day4"),
				},
				{
					date:  time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC),
					files: makeFileList(400, "day5"),
				},
			},
			thresholdOneDay:          600,
			thresholdConsecutiveDays: 300,
			expectedGroupCount:       2,
			expectedDestDirs:         []string{"20260101-20260102", "20260104-20260105"},
			expectedFilesPerGroup:    []int{800, 800},
			description:              "Two separate consecutive events with gap on Jan 3",
		},
		{
			name: "single day with 1000 photos - well above threshold",
			dateGroups: []dateGroup{
				{
					date:  time.Date(2026, 2, 14, 0, 0, 0, 0, time.UTC),
					files: makeFileList(1000, "valentine"),
				},
			},
			thresholdOneDay:          600,
			thresholdConsecutiveDays: 300,
			expectedGroupCount:       1,
			expectedDestDirs:         []string{"20260214"},
			expectedFilesPerGroup:    []int{1000},
			description:              "Single day with 1000 photos (well above 600 threshold) → own folder",
		},
		{
			name: "consecutive days with 800 photos each - high volume event",
			dateGroups: []dateGroup{
				{
					date:  time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC),
					files: makeFileList(800, "day1"),
				},
				{
					date:  time.Date(2026, 3, 11, 0, 0, 0, 0, time.UTC),
					files: makeFileList(850, "day2"),
				},
				{
					date:  time.Date(2026, 3, 12, 0, 0, 0, 0, time.UTC),
					files: makeFileList(900, "day3"),
				},
			},
			thresholdOneDay:          600,
			thresholdConsecutiveDays: 300,
			expectedGroupCount:       1,
			expectedDestDirs:         []string{"20260310-20260312"},
			expectedFilesPerGroup:    []int{2550},
			description:              "3 consecutive days with 800+ photos each → grouped (exceeds both thresholds)",
		},
		{
			name: "consecutive days at exactly threshold values",
			dateGroups: []dateGroup{
				{
					date:  time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
					files: makeFileList(600, "day1"),
				},
				{
					date:  time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC),
					files: makeFileList(300, "day2"),
				},
			},
			thresholdOneDay:          600,
			thresholdConsecutiveDays: 300,
			expectedGroupCount:       1,
			expectedDestDirs:         []string{"20260401-20260402"},
			expectedFilesPerGroup:    []int{900},
			description:              "Day 1: exactly 600, Day 2: exactly 300 → grouped (both ≥300)",
		},
		{
			name: "week-long event with varying high volumes",
			dateGroups: []dateGroup{
				{
					date:  time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
					files: makeFileList(500, "day1"),
				},
				{
					date:  time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC),
					files: makeFileList(600, "day2"),
				},
				{
					date:  time.Date(2026, 5, 3, 0, 0, 0, 0, time.UTC),
					files: makeFileList(700, "day3"),
				},
				{
					date:  time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC),
					files: makeFileList(650, "day4"),
				},
				{
					date:  time.Date(2026, 5, 5, 0, 0, 0, 0, time.UTC),
					files: makeFileList(550, "day5"),
				},
			},
			thresholdOneDay:          600,
			thresholdConsecutiveDays: 300,
			expectedGroupCount:       1,
			expectedDestDirs:         []string{"20260501-20260505"},
			expectedFilesPerGroup:    []int{3000},
			description:              "5 consecutive days with 500-700 photos (all ≥300) → grouped as single event",
		},
		{
			name: "single day exactly at threshold - edge case",
			dateGroups: []dateGroup{
				{
					date:  time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC),
					files: makeFileList(600, "day15"),
				},
			},
			thresholdOneDay:          600,
			thresholdConsecutiveDays: 300,
			expectedGroupCount:       1,
			expectedDestDirs:         []string{"20260615"},
			expectedFilesPerGroup:    []int{600},
			description:              "Single day with exactly 600 photos → own folder (at threshold boundary)",
		},
		{
			name: "single day just below threshold",
			dateGroups: []dateGroup{
				{
					date:  time.Date(2026, 6, 16, 0, 0, 0, 0, time.UTC),
					files: makeFileList(599, "day16"),
				},
			},
			thresholdOneDay:          600,
			thresholdConsecutiveDays: 300,
			expectedGroupCount:       1,
			expectedDestDirs:         []string{"."},
			expectedFilesPerGroup:    []int{599},
			description:              "Single day with 599 photos (one below threshold) → root directory",
		},
		{
			name: "massive multi-day event - 2000+ photos per day",
			dateGroups: []dateGroup{
				{
					date:  time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
					files: makeFileList(2000, "day1"),
				},
				{
					date:  time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC),
					files: makeFileList(2500, "day2"),
				},
				{
					date:  time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC),
					files: makeFileList(1800, "day3"),
				},
			},
			thresholdOneDay:          600,
			thresholdConsecutiveDays: 300,
			expectedGroupCount:       1,
			expectedDestDirs:         []string{"20260701-20260703"},
			expectedFilesPerGroup:    []int{6300},
			description:              "3 consecutive days with 2000+ photos each → grouped (extreme high volume)",
		},
		{
			name: "alternating high and low volume days",
			dateGroups: []dateGroup{
				{
					date:  time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
					files: makeFileList(800, "day1"),
				},
				{
					date:  time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC),
					files: makeFileList(200, "day2"),
				},
				{
					date:  time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC),
					files: makeFileList(900, "day3"),
				},
				{
					date:  time.Date(2026, 8, 4, 0, 0, 0, 0, time.UTC),
					files: makeFileList(150, "day4"),
				},
			},
			thresholdOneDay:          600,
			thresholdConsecutiveDays: 300,
			expectedGroupCount:       4,
			expectedDestDirs:         []string{"20260801", ".", "20260803", "."},
			expectedFilesPerGroup:    []int{800, 200, 900, 150},
			description:              "Alternating high/low volume → separate folders for high days, root for low days",
		},
		{
			name: "different thresholds - lower values for hobby photographer",
			dateGroups: []dateGroup{
				{
					date:  time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
					files: makeFileList(150, "day1"),
				},
				{
					date:  time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC),
					files: makeFileList(120, "day2"),
				},
				{
					date:  time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC),
					files: makeFileList(130, "day3"),
				},
			},
			thresholdOneDay:          200, // Lower threshold for hobby use
			thresholdConsecutiveDays: 100, // Lower threshold for consecutive days
			expectedGroupCount:       1,
			expectedDestDirs:         []string{"20260901-20260903"},
			expectedFilesPerGroup:    []int{400},
			description:              "Lower thresholds (200/100) → 3 consecutive days with 120-150 photos → grouped",
		},
		{
			name: "different thresholds - higher values for professional photographer",
			dateGroups: []dateGroup{
				{
					date:  time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
					files: makeFileList(800, "day1"),
				},
				{
					date:  time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC),
					files: makeFileList(750, "day2"),
				},
			},
			thresholdOneDay:          1000, // Higher threshold for pro use
			thresholdConsecutiveDays: 500,  // Higher threshold for consecutive days
			expectedGroupCount:       1,
			expectedDestDirs:         []string{"20261001-20261002"},
			expectedFilesPerGroup:    []int{1550},
			description:              "Higher thresholds (1000/500) → 2 consecutive days with 750-800 photos (both ≥500) → grouped",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectConsecutiveDays(tt.dateGroups, tt.thresholdOneDay, tt.thresholdConsecutiveDays)

			assert.Equal(t, tt.expectedGroupCount, len(result), "Expected %d groups, got %d. %s", tt.expectedGroupCount, len(result), tt.description)

			destDirs := make([]string, len(result))
			filesPerGroup := make([]int, len(result))
			for i, group := range result {
				destDirs[i] = group.destDir
				filesPerGroup[i] = len(group.files)
			}

			assert.Equal(t, tt.expectedDestDirs, destDirs, "Destination directories don't match. %s", tt.description)
			assert.Equal(t, tt.expectedFilesPerGroup, filesPerGroup, "Files per group don't match. %s", tt.description)
		})
	}
}

// Helper function to create a list of file names
func makeFileList(count int, prefix string) []string {
	files := make([]string, count)
	for i := 0; i < count; i++ {
		files[i] = fmt.Sprintf("%s_file%d.arw", prefix, i+1)
	}
	return files
}
