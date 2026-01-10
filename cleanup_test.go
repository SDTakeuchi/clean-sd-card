package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeleteZombieEditFiles(t *testing.T) {
	t.Run("deletes zombie edit files when no corresponding raw file exists", func(t *testing.T) {
		dirTest := t.TempDir()

		// Create zombie edit files (no corresponding raw files)
		for i := range 3 {
			_, err := os.Create(filepath.Join(dirTest, fmt.Sprintf("photo%d.xmp", i+1)))
			require.NoError(t, err)
		}

		count, err := deleteZombieEditFiles("xmp", dirTest, []string{"arw", "raw"}, false)

		assert.NoError(t, err)
		assert.Equal(t, 3, count)

		// Verify all zombie files are deleted
		entries, err := os.ReadDir(dirTest)
		require.NoError(t, err)
		assert.Equal(t, 0, len(entries))
	})

	t.Run("keeps edit files when corresponding raw file exists", func(t *testing.T) {
		dirTest := t.TempDir()

		// Create edit file with corresponding raw file
		_, err := os.Create(filepath.Join(dirTest, "photo1.xmp"))
		require.NoError(t, err)
		_, err = os.Create(filepath.Join(dirTest, "photo1.arw"))
		require.NoError(t, err)

		// Create another edit file with corresponding raw file (different extension)
		_, err = os.Create(filepath.Join(dirTest, "photo2.xmp"))
		require.NoError(t, err)
		_, err = os.Create(filepath.Join(dirTest, "photo2.raw"))
		require.NoError(t, err)

		count, err := deleteZombieEditFiles("xmp", dirTest, []string{"arw", "raw"}, false)

		assert.NoError(t, err)
		assert.Equal(t, 0, count)

		// Verify all files still exist
		entries, err := os.ReadDir(dirTest)
		require.NoError(t, err)
		assert.Equal(t, 4, len(entries))
	})

	t.Run("mixed scenario with some zombie and some valid edit files", func(t *testing.T) {
		dirTest := t.TempDir()

		// Create valid edit file (has corresponding raw)
		_, err := os.Create(filepath.Join(dirTest, "valid.xmp"))
		require.NoError(t, err)
		_, err = os.Create(filepath.Join(dirTest, "valid.arw"))
		require.NoError(t, err)

		// Create zombie edit files (no corresponding raw)
		_, err = os.Create(filepath.Join(dirTest, "zombie1.xmp"))
		require.NoError(t, err)
		_, err = os.Create(filepath.Join(dirTest, "zombie2.xmp"))
		require.NoError(t, err)

		count, err := deleteZombieEditFiles("xmp", dirTest, []string{"arw", "raw"}, false)

		assert.NoError(t, err)
		assert.Equal(t, 2, count)

		// Verify valid files still exist and zombies are deleted
		entries, err := os.ReadDir(dirTest)
		require.NoError(t, err)

		fileNames := make([]string, len(entries))
		for i, entry := range entries {
			fileNames[i] = entry.Name()
		}
		assert.ElementsMatch(t, []string{"valid.xmp", "valid.arw"}, fileNames)
	})

	t.Run("does not delete non-edit files", func(t *testing.T) {
		dirTest := t.TempDir()

		// Create non-edit files
		_, err := os.Create(filepath.Join(dirTest, "photo.jpg"))
		require.NoError(t, err)
		_, err = os.Create(filepath.Join(dirTest, "photo.png"))
		require.NoError(t, err)

		count, err := deleteZombieEditFiles("xmp", dirTest, []string{"arw", "raw"}, false)

		assert.NoError(t, err)
		assert.Equal(t, 0, count)

		// Verify files still exist
		entries, err := os.ReadDir(dirTest)
		require.NoError(t, err)
		assert.Equal(t, 2, len(entries))
	})

	t.Run("handles empty directory", func(t *testing.T) {
		dirTest := t.TempDir()

		count, err := deleteZombieEditFiles("xmp", dirTest, []string{"arw", "raw"}, false)

		assert.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("returns error for non-existent directory", func(t *testing.T) {
		count, err := deleteZombieEditFiles("xmp", "/non/existent/path", []string{"arw", "raw"}, false)

		assert.Error(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("recursive mode deletes zombie files in subdirectories", func(t *testing.T) {
		dirTest := t.TempDir()
		subDir := filepath.Join(dirTest, "subdir")
		err := os.MkdirAll(subDir, 0755)
		require.NoError(t, err)

		// Create zombie edit file in root
		_, err = os.Create(filepath.Join(dirTest, "root_zombie.xmp"))
		require.NoError(t, err)

		// Create zombie edit file in subdirectory
		_, err = os.Create(filepath.Join(subDir, "sub_zombie.xmp"))
		require.NoError(t, err)

		// Create valid edit file with raw in subdirectory
		_, err = os.Create(filepath.Join(subDir, "valid.xmp"))
		require.NoError(t, err)
		_, err = os.Create(filepath.Join(subDir, "valid.arw"))
		require.NoError(t, err)

		count, err := deleteZombieEditFiles("xmp", dirTest, []string{"arw", "raw"}, true)

		assert.NoError(t, err)
		assert.Equal(t, 2, count) // root_zombie.xmp + sub_zombie.xmp

		// Verify valid files in subdirectory still exist
		entries, err := os.ReadDir(subDir)
		require.NoError(t, err)
		fileNames := make([]string, len(entries))
		for i, entry := range entries {
			fileNames[i] = entry.Name()
		}
		assert.ElementsMatch(t, []string{"valid.xmp", "valid.arw"}, fileNames)
	})

	t.Run("non-recursive mode skips subdirectories", func(t *testing.T) {
		dirTest := t.TempDir()
		subDir := filepath.Join(dirTest, "subdir")
		err := os.MkdirAll(subDir, 0755)
		require.NoError(t, err)

		// Create zombie edit file in root
		_, err = os.Create(filepath.Join(dirTest, "root_zombie.xmp"))
		require.NoError(t, err)

		// Create zombie edit file in subdirectory
		_, err = os.Create(filepath.Join(subDir, "sub_zombie.xmp"))
		require.NoError(t, err)

		count, err := deleteZombieEditFiles("xmp", dirTest, []string{"arw", "raw"}, false)

		assert.NoError(t, err)
		assert.Equal(t, 1, count) // only root_zombie.xmp

		// Verify subdirectory file still exists
		entries, err := os.ReadDir(subDir)
		require.NoError(t, err)
		assert.Equal(t, 1, len(entries))
		assert.Equal(t, "sub_zombie.xmp", entries[0].Name())
	})
}
