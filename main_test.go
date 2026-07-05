package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCleanSDCard(t *testing.T) {
	fsys := newFakeFileSystem()
	dirSrc := "src"
	dirDst := "dst"
	dirDstJPG := "dst-jpg"
	editFileExtensions := []string{"xmp"}
	extensionsToCopy := []string{"raw"}
	extensionsJPG := []string{"jpg"}
	opts := Options{
		DryRun:                false,
		Overwrite:             false,
		DeleteZombieEditFiles: false,
		KeepJPG:               false,
		KeepSrc:               false,
	}

	fileCount := 30
	expectedFiles := make([]string, fileCount)
	for i := range fileCount {
		name := fmt.Sprintf("file%d.%s", i+1, extensionsToCopy[0])
		fsys.addFile(filepath.Join(dirSrc, name), "content")
		expectedFiles[i] = name
	}

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

	assert.NoError(t, err)
	assert.Equal(t, fileCount, totalCopied)
	assert.Equal(t, fileCount, removedCount)

	entries, err := fsys.ReadDir(dirDst)
	assert.NoError(t, err)
	assert.Equal(t, fileCount, len(entries))

	copiedFiles := make([]string, len(entries))
	for i, entry := range entries {
		copiedFiles[i] = entry.Name()
	}
	assert.ElementsMatch(t, copiedFiles, expectedFiles)

	entries, err = fsys.ReadDir(dirSrc)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(entries))
}

func TestCleanSDCardReadsSourceDirOnce(t *testing.T) {
	fake := newFakeFileSystem()
	dirSrc := "src"
	dirDst := "dst"
	dirDstJPG := "dst-jpg"

	fake.addFile(filepath.Join(dirSrc, "photo1.raw"), "content")
	fake.addFile(filepath.Join(dirSrc, "photo2.arw"), "content")
	fake.addFile(filepath.Join(dirSrc, "photo3.jpg"), "content")

	counting := newReadDirCountingFileSystem(fake)

	_, _, err := cleanSDCard(
		counting,
		[]string{"xmp"},
		[]string{"raw", "arw"},
		[]string{"jpg", "jpeg"},
		dirSrc,
		dirDst,
		dirDstJPG,
		Options{KeepJPG: true, DeleteZombieEditFiles: false},
	)

	assert.NoError(t, err)
	assert.Equal(t, 1, counting.callsFor(dirSrc), "dirSrc should only be listed once, shared across the raw copy, JPG copy, and removal steps")
}

func TestDeleteZombieEditFiles(t *testing.T) {
	t.Run("deletes zombie edit files when no corresponding raw file exists", func(t *testing.T) {
		fsys := newFakeFileSystem()

		// Create zombie edit files (no corresponding raw files)
		for i := range 3 {
			fsys.addFile(fmt.Sprintf("photo%d.xmp", i+1), "")
		}

		count, err := deleteZombieEditFiles(fsys, "xmp", ".", []string{"arw", "raw"}, false)

		assert.NoError(t, err)
		assert.Equal(t, 3, count)

		// Verify all zombie files are deleted
		entries, err := fsys.ReadDir(".")
		require.NoError(t, err)
		assert.Equal(t, 0, len(entries))
	})

	t.Run("keeps edit files when corresponding raw file exists", func(t *testing.T) {
		fsys := newFakeFileSystem()

		// Create edit file with corresponding raw file
		fsys.addFile("photo1.xmp", "")
		fsys.addFile("photo1.arw", "")

		// Create another edit file with corresponding raw file (different extension)
		fsys.addFile("photo2.xmp", "")
		fsys.addFile("photo2.raw", "")

		count, err := deleteZombieEditFiles(fsys, "xmp", ".", []string{"arw", "raw"}, false)

		assert.NoError(t, err)
		assert.Equal(t, 0, count)

		// Verify all files still exist
		entries, err := fsys.ReadDir(".")
		require.NoError(t, err)
		assert.Equal(t, 4, len(entries))
	})

	t.Run("mixed scenario with some zombie and some valid edit files", func(t *testing.T) {
		fsys := newFakeFileSystem()

		// Create valid edit file (has corresponding raw)
		fsys.addFile("valid.xmp", "")
		fsys.addFile("valid.arw", "")

		// Create zombie edit files (no corresponding raw)
		fsys.addFile("zombie1.xmp", "")
		fsys.addFile("zombie2.xmp", "")

		count, err := deleteZombieEditFiles(fsys, "xmp", ".", []string{"arw", "raw"}, false)

		assert.NoError(t, err)
		assert.Equal(t, 2, count)

		// Verify valid files still exist and zombies are deleted
		entries, err := fsys.ReadDir(".")
		require.NoError(t, err)

		fileNames := make([]string, len(entries))
		for i, entry := range entries {
			fileNames[i] = entry.Name()
		}
		assert.ElementsMatch(t, []string{"valid.xmp", "valid.arw"}, fileNames)
	})

	t.Run("does not delete non-edit files", func(t *testing.T) {
		fsys := newFakeFileSystem()

		// Create non-edit files
		fsys.addFile("photo.jpg", "")
		fsys.addFile("photo.png", "")

		count, err := deleteZombieEditFiles(fsys, "xmp", ".", []string{"arw", "raw"}, false)

		assert.NoError(t, err)
		assert.Equal(t, 0, count)

		// Verify files still exist
		entries, err := fsys.ReadDir(".")
		require.NoError(t, err)
		assert.Equal(t, 2, len(entries))
	})

	t.Run("handles empty directory", func(t *testing.T) {
		fsys := newFakeFileSystem()

		count, err := deleteZombieEditFiles(fsys, "xmp", ".", []string{"arw", "raw"}, false)

		assert.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("returns error for non-existent directory", func(t *testing.T) {
		fsys := newFakeFileSystem()

		count, err := deleteZombieEditFiles(fsys, "xmp", "/non/existent/path", []string{"arw", "raw"}, false)

		assert.Error(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("recursive mode deletes zombie files in subdirectories", func(t *testing.T) {
		fsys := newFakeFileSystem()
		subDir := "subdir"
		fsys.addDir(subDir)

		// Create zombie edit file in root
		fsys.addFile("root_zombie.xmp", "")

		// Create zombie edit file in subdirectory
		fsys.addFile(filepath.Join(subDir, "sub_zombie.xmp"), "")

		// Create valid edit file with raw in subdirectory
		fsys.addFile(filepath.Join(subDir, "valid.xmp"), "")
		fsys.addFile(filepath.Join(subDir, "valid.arw"), "")

		count, err := deleteZombieEditFiles(fsys, "xmp", ".", []string{"arw", "raw"}, true)

		assert.NoError(t, err)
		assert.Equal(t, 2, count) // root_zombie.xmp + sub_zombie.xmp

		// Verify valid files in subdirectory still exist
		entries, err := fsys.ReadDir(subDir)
		require.NoError(t, err)
		fileNames := make([]string, len(entries))
		for i, entry := range entries {
			fileNames[i] = entry.Name()
		}
		assert.ElementsMatch(t, []string{"valid.xmp", "valid.arw"}, fileNames)
	})

	t.Run("non-recursive mode skips subdirectories", func(t *testing.T) {
		fsys := newFakeFileSystem()
		subDir := "subdir"
		fsys.addDir(subDir)

		// Create zombie edit file in root
		fsys.addFile("root_zombie.xmp", "")

		// Create zombie edit file in subdirectory
		fsys.addFile(filepath.Join(subDir, "sub_zombie.xmp"), "")

		count, err := deleteZombieEditFiles(fsys, "xmp", ".", []string{"arw", "raw"}, false)

		assert.NoError(t, err)
		assert.Equal(t, 1, count) // only root_zombie.xmp

		// Verify subdirectory file still exists
		entries, err := fsys.ReadDir(subDir)
		require.NoError(t, err)
		assert.Equal(t, 1, len(entries))
		assert.Equal(t, "sub_zombie.xmp", entries[0].Name())
	})
}

func TestCopyFilesDeadlock(t *testing.T) {
	t.Run("does not deadlock when a copy error occurs", func(t *testing.T) {
		dirSrc := t.TempDir()
		dirDst := t.TempDir()

		// Create a source file
		srcFilePath := filepath.Join(dirSrc, "file1.txt")
		err := os.WriteFile(srcFilePath, []byte("hello"), 0644)
		require.NoError(t, err)

		// Create a directory in the destination with the same name as the source file.
		// This will cause os.Create(dstPath) in osFileSystem.CopyFile to fail.
		err = os.Mkdir(filepath.Join(dirDst, "file1.txt"), 0755)
		require.NoError(t, err)

		fsys := osFileSystem{}
		entries, err := fsys.ReadDir(dirSrc)
		require.NoError(t, err)

		// This call would hang if the deadlock is present.
		// We expect it to complete with an error.
		count, err := copyFiles(fsys, entries, dirSrc, dirDst, []string{"txt"}, false, true)

		assert.Error(t, err)
		assert.Equal(t, 0, count)
	})
}
