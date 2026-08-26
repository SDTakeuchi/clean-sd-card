package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testConcurrency = 4

func TestDefaultDirectories(t *testing.T) {
	assert.Equal(t, "E:\\DCIM", defaultDirSrc)
	assert.Equal(t, "E:\\PRIVATE\\M4ROOT\\CLIP", defaultDirVideoSrc)
	assert.Equal(t, "D:\\movies", defaultDirVideoDst)
}

func TestCopyVideoFiles(t *testing.T) {
	fsys := newFakeFileSystem()
	dirSrc := "video-src"
	dirDst := "video-dst"

	fsys.addFile(filepath.Join(dirSrc, "clip1.mp4"), "video 1")
	fsys.addFile(filepath.Join(dirSrc, "clip2.MP4"), "video 2")
	fsys.addFile(filepath.Join(dirSrc, "clip2M01.XML"), "metadata")
	fsys.addFile(filepath.Join(dirSrc, "thumbnail.jpg"), "image")

	copied, err := copyVideoFiles(
		fsys,
		[]string{"mp4", "xml"},
		dirSrc,
		dirDst,
		Options{Concurrency: testConcurrency},
	)

	require.NoError(t, err)
	assert.Equal(t, 3, copied)

	destinationEntries, err := fsys.ReadDir(dirDst)
	require.NoError(t, err)
	assert.Len(t, destinationEntries, 3)
	assert.ElementsMatch(
		t,
		[]string{"clip1.mp4", "clip2.MP4", "clip2M01.XML"},
		[]string{destinationEntries[0].Name(), destinationEntries[1].Name(), destinationEntries[2].Name()},
	)

	sourceEntries, err := fsys.ReadDir(dirSrc)
	require.NoError(t, err)
	assert.Len(t, sourceEntries, 4, "video copies must not remove files from the SD card")
}

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
		Concurrency:           testConcurrency,
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
		Options{KeepJPG: true, DeleteZombieEditFiles: false, Concurrency: testConcurrency},
	)

	assert.NoError(t, err)
	assert.Equal(t, 1, counting.callsFor(dirSrc), "dirSrc should only be listed once, shared across the raw copy, JPG copy, and removal steps")
}

func TestCleanSDCardRecursivelyProcessesIncrementedCameraDirectories(t *testing.T) {
	fsys := newFakeFileSystem()
	dirSrc := "DCIM"
	dirDst := "dst"
	dirDstJPG := "dst-jpg"

	firstCameraDir := filepath.Join(dirSrc, "100MSDCF")
	secondCameraDir := filepath.Join(dirSrc, "101MSDCF")
	sourceFiles := map[string]string{
		filepath.Join(firstCameraDir, "A7V00015.ARW"):  "raw from 100",
		filepath.Join(firstCameraDir, "A7V00015.JPG"):  "jpg from 100",
		filepath.Join(secondCameraDir, "A7V00015.ARW"): "raw from 101",
		filepath.Join(secondCameraDir, "A7V00015.JPG"): "jpg from 101",
	}
	for path, content := range sourceFiles {
		fsys.addFile(path, content)
	}

	totalCopied, removedCount, err := cleanSDCard(
		fsys,
		[]string{"xmp"},
		[]string{"arw", "raw"},
		[]string{"jpg", "jpeg"},
		dirSrc,
		dirDst,
		dirDstJPG,
		Options{
			KeepJPG:               true,
			KeepSrc:               false,
			DeleteZombieEditFiles: false,
			Concurrency:           testConcurrency,
		},
	)

	require.NoError(t, err)
	assert.Equal(t, 4, totalCopied)
	assert.Equal(t, 4, removedCount)

	rawEntries, err := fsys.ReadDir(dirDst)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"A7V10000015.ARW", "A7V10100015.ARW"}, entryNames(rawEntries))
	assert.Equal(t, []byte("raw from 100"), fsys.files[cleanFakePath(filepath.Join(dirDst, "A7V10000015.ARW"))])
	assert.Equal(t, []byte("raw from 101"), fsys.files[cleanFakePath(filepath.Join(dirDst, "A7V10100015.ARW"))])

	jpgEntries, err := fsys.ReadDir(dirDstJPG)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"A7V10000015.JPG", "A7V10100015.JPG"}, entryNames(jpgEntries))
	assert.Equal(t, []byte("jpg from 100"), fsys.files[cleanFakePath(filepath.Join(dirDstJPG, "A7V10000015.JPG"))])
	assert.Equal(t, []byte("jpg from 101"), fsys.files[cleanFakePath(filepath.Join(dirDstJPG, "A7V10100015.JPG"))])

	firstSourceEntries, err := fsys.ReadDir(firstCameraDir)
	require.NoError(t, err)
	assert.Empty(t, firstSourceEntries)

	secondSourceEntries, err := fsys.ReadDir(secondCameraDir)
	require.NoError(t, err)
	assert.Empty(t, secondSourceEntries)
}

func TestPhotoDestinationFileName(t *testing.T) {
	tests := []struct {
		name string
		file sourceFile
		want string
	}{
		{
			name: "inserts parent folder number before JPG sequence",
			file: sourceFile{name: "A7V00015.JPG", path: filepath.Join("DCIM", "101MSDCF", "A7V00015.JPG")},
			want: "A7V10100015.JPG",
		},
		{
			name: "inserts parent folder number before ARW sequence",
			file: sourceFile{name: "A7V00015.ARW", path: filepath.Join("DCIM", "101MSDCF", "A7V00015.ARW")},
			want: "A7V10100015.ARW",
		},
		{
			name: "preserves name when parent has no numeric prefix",
			file: sourceFile{name: "A7V00015.JPG", path: filepath.Join("DCIM", "MISC", "A7V00015.JPG")},
			want: "A7V00015.JPG",
		},
		{
			name: "preserves name without five digit sequence",
			file: sourceFile{name: "thumbnail.JPG", path: filepath.Join("DCIM", "101MSDCF", "thumbnail.JPG")},
			want: "thumbnail.JPG",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, photoDestinationFileName(tt.file))
		})
	}
}

func entryNames(entries []os.DirEntry) []string {
	names := make([]string, len(entries))
	for i, entry := range entries {
		names[i] = entry.Name()
	}
	return names
}

func TestDeleteZombieEditFiles(t *testing.T) {
	t.Run("deletes zombie edit files when no corresponding raw file exists", func(t *testing.T) {
		fsys := newFakeFileSystem()

		// Create zombie edit files (no corresponding raw files)
		for i := range 3 {
			fsys.addFile(fmt.Sprintf("photo%d.xmp", i+1), "")
		}

		count, err := deleteZombieEditFiles(fsys, "xmp", ".", []string{"arw", "raw"}, false, testConcurrency)

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

		count, err := deleteZombieEditFiles(fsys, "xmp", ".", []string{"arw", "raw"}, false, testConcurrency)

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

		count, err := deleteZombieEditFiles(fsys, "xmp", ".", []string{"arw", "raw"}, false, testConcurrency)

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

		count, err := deleteZombieEditFiles(fsys, "xmp", ".", []string{"arw", "raw"}, false, testConcurrency)

		assert.NoError(t, err)
		assert.Equal(t, 0, count)

		// Verify files still exist
		entries, err := fsys.ReadDir(".")
		require.NoError(t, err)
		assert.Equal(t, 2, len(entries))
	})

	t.Run("handles empty directory", func(t *testing.T) {
		fsys := newFakeFileSystem()

		count, err := deleteZombieEditFiles(fsys, "xmp", ".", []string{"arw", "raw"}, false, testConcurrency)

		assert.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("returns error for non-existent directory", func(t *testing.T) {
		fsys := newFakeFileSystem()

		count, err := deleteZombieEditFiles(fsys, "xmp", "/non/existent/path", []string{"arw", "raw"}, false, testConcurrency)

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

		count, err := deleteZombieEditFiles(fsys, "xmp", ".", []string{"arw", "raw"}, true, testConcurrency)

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

		count, err := deleteZombieEditFiles(fsys, "xmp", ".", []string{"arw", "raw"}, false, testConcurrency)

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
		count, err := copyFiles(fsys, entries, dirSrc, dirDst, []string{"txt"}, false, true, testConcurrency)

		assert.Error(t, err)
		assert.Equal(t, 0, count)
	})
}
