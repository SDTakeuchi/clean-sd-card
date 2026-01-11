package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testFile struct {
	name     string
	isZombie bool // true if it's a zombie edit file (no corresponding raw)
	inSubdir bool // true if file should be in subdirectory
}

func TestDeleteZombieEditFiles(t *testing.T) {

	tests := []struct {
		name                string
		files               []testFile
		editExtension       string
		rawExtensions       []string
		recursive           bool
		expectedDeleteCount int
		expectedRemaining   []string // files that should remain after cleanup
		shouldError         bool
		description         string
	}{
		{
			name: "deletes zombie edit files when no corresponding raw file exists",
			files: []testFile{
				{name: "photo1.xmp", isZombie: true},
				{name: "photo2.xmp", isZombie: true},
				{name: "photo3.xmp", isZombie: true},
			},
			editExtension:       "xmp",
			rawExtensions:       []string{"arw", "raw"},
			recursive:           false,
			expectedDeleteCount: 3,
			expectedRemaining:   []string{},
			description:         "All XMP files without corresponding RAW files should be deleted",
		},
		{
			name: "keeps edit files when corresponding raw file exists",
			files: []testFile{
				{name: "photo1.xmp", isZombie: false},
				{name: "photo1.arw", isZombie: false},
				{name: "photo2.xmp", isZombie: false},
				{name: "photo2.raw", isZombie: false},
			},
			editExtension:       "xmp",
			rawExtensions:       []string{"arw", "raw"},
			recursive:           false,
			expectedDeleteCount: 0,
			expectedRemaining:   []string{"photo1.arw", "photo1.xmp", "photo2.raw", "photo2.xmp"},
			description:         "XMP files with corresponding RAW files should be kept",
		},
		{
			name: "mixed scenario - keeps valid and deletes zombies",
			files: []testFile{
				{name: "valid.xmp", isZombie: false},
				{name: "valid.arw", isZombie: false},
				{name: "zombie1.xmp", isZombie: true},
				{name: "zombie2.xmp", isZombie: true},
			},
			editExtension:       "xmp",
			rawExtensions:       []string{"arw", "raw"},
			recursive:           false,
			expectedDeleteCount: 2,
			expectedRemaining:   []string{"valid.arw", "valid.xmp"},
			description:         "Should delete only zombie XMP files and keep valid pairs",
		},
		{
			name: "ignores non-edit file extensions",
			files: []testFile{
				{name: "photo.jpg", isZombie: false},
				{name: "photo.png", isZombie: false},
				{name: "document.txt", isZombie: false},
			},
			editExtension:       "xmp",
			rawExtensions:       []string{"arw", "raw"},
			recursive:           false,
			expectedDeleteCount: 0,
			expectedRemaining:   []string{"document.txt", "photo.jpg", "photo.png"},
			description:         "Non-XMP files should be ignored and left untouched",
		},
		{
			name:                "handles empty directory gracefully",
			files:               []testFile{},
			editExtension:       "xmp",
			rawExtensions:       []string{"arw", "raw"},
			recursive:           false,
			expectedDeleteCount: 0,
			expectedRemaining:   []string{},
			description:         "Empty directory should result in zero deletions",
		},
		{
			name: "recursive mode processes subdirectories",
			files: []testFile{
				{name: "root_zombie.xmp", isZombie: true, inSubdir: false},
				{name: "sub_zombie.xmp", isZombie: true, inSubdir: true},
				{name: "valid.xmp", isZombie: false, inSubdir: true},
				{name: "valid.arw", isZombie: false, inSubdir: true},
			},
			editExtension:       "xmp",
			rawExtensions:       []string{"arw", "raw"},
			recursive:           true,
			expectedDeleteCount: 2,
			expectedRemaining:   []string{"valid.arw", "valid.xmp"}, // in subdir
			description:         "Recursive mode should delete zombies in root and subdirectories",
		},
		{
			name: "non-recursive mode skips subdirectories",
			files: []testFile{
				{name: "root_zombie.xmp", isZombie: true, inSubdir: false},
				{name: "sub_zombie.xmp", isZombie: true, inSubdir: true},
			},
			editExtension:       "xmp",
			rawExtensions:       []string{"arw", "raw"},
			recursive:           false,
			expectedDeleteCount: 1,
			expectedRemaining:   []string{"sub_zombie.xmp"}, // should remain in subdir
			description:         "Non-recursive mode should only process root directory",
		},
		{
			name: "handles multiple raw extensions correctly",
			files: []testFile{
				{name: "photo1.xmp", isZombie: false},
				{name: "photo1.arw", isZombie: false},
				{name: "photo2.xmp", isZombie: false},
				{name: "photo2.raw", isZombie: false},
				{name: "photo3.xmp", isZombie: false},
				{name: "photo3.dng", isZombie: false},
				{name: "zombie.xmp", isZombie: true},
			},
			editExtension:       "xmp",
			rawExtensions:       []string{"arw", "raw", "dng"},
			recursive:           false,
			expectedDeleteCount: 1,
			expectedRemaining:   []string{"photo1.arw", "photo1.xmp", "photo2.raw", "photo2.xmp", "photo3.dng", "photo3.xmp"},
			description:         "Should match XMP files against multiple RAW extensions",
		},
		{
			name: "case insensitive extension matching",
			files: []testFile{
				{name: "photo1.XMP", isZombie: false},
				{name: "photo1.ARW", isZombie: false},
				{name: "photo2.Xmp", isZombie: false},
				{name: "photo2.Raw", isZombie: false},
			},
			editExtension:       "xmp",
			rawExtensions:       []string{"arw", "raw"},
			recursive:           false,
			expectedDeleteCount: 0,
			expectedRemaining:   []string{"photo1.ARW", "photo1.XMP", "photo2.Raw", "photo2.Xmp"},
			description:         "Extension matching should be case-insensitive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dirTest := t.TempDir()
			subDir := filepath.Join(dirTest, "subdir")

			// Create subdirectory if needed
			if hasSubdirFiles(tt.files) {
				err := os.MkdirAll(subDir, 0755)
				require.NoError(t, err)
			}

			// Create test files
			for _, f := range tt.files {
				targetDir := dirTest
				if f.inSubdir {
					targetDir = subDir
				}
				_, err := os.Create(filepath.Join(targetDir, f.name))
				require.NoError(t, err, "Failed to create test file: %s", f.name)
			}

			// Execute the function under test
			count, err := deleteZombieEditFiles(tt.editExtension, dirTest, tt.rawExtensions, tt.recursive)

			// Assertions
			if tt.shouldError {
				assert.Error(t, err, tt.description)
			} else {
				assert.NoError(t, err, tt.description)
			}

			assert.Equal(t, tt.expectedDeleteCount, count, "Deleted file count mismatch. %s", tt.description)

			// Verify remaining files
			var actualRemaining []string

			// Check files in root
			entries, err := os.ReadDir(dirTest)
			require.NoError(t, err)
			for _, entry := range entries {
				if !entry.IsDir() {
					actualRemaining = append(actualRemaining, entry.Name())
				}
			}

			// For non-recursive mode with subdirectory files, check subdirectory separately
			if !tt.recursive && hasSubdirFiles(tt.files) {
				// Check if we expect files to remain in subdirectory
				entries, err := os.ReadDir(subDir)
				require.NoError(t, err)
				for _, entry := range entries {
					if !entry.IsDir() {
						actualRemaining = append(actualRemaining, entry.Name())
					}
				}
			} else if tt.recursive && hasSubdirFiles(tt.files) {
				// For recursive mode, check subdirectory
				entries, err := os.ReadDir(subDir)
				require.NoError(t, err)
				for _, entry := range entries {
					if !entry.IsDir() {
						actualRemaining = append(actualRemaining, entry.Name())
					}
				}
			}

			assert.ElementsMatch(t, tt.expectedRemaining, actualRemaining, "Remaining files don't match. %s", tt.description)
		})
	}

	// Separate test for non-existent directory (can't use table-driven approach)
	t.Run("returns error for non-existent directory", func(t *testing.T) {
		count, err := deleteZombieEditFiles("xmp", "/non/existent/path/12345", []string{"arw", "raw"}, false)

		assert.Error(t, err, "Should return error for non-existent directory")
		assert.Equal(t, 0, count, "Delete count should be 0 on error")
	})
}

// Helper function to check if any files should be in subdirectory
func hasSubdirFiles(files []testFile) bool {
	for _, f := range files {
		if f.inSubdir {
			return true
		}
	}
	return false
}
