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
	dirTest := "test"
	dirSrc := filepath.Join(dirTest, "src")
	dirDst := filepath.Join(dirTest, "dst")
	editFileExtensions := []string{"xmp"}
	extensionsToCopy := []string{"raw"}
	flagDryRun := false
	flagOverwrite := false
	flagDeleteZombieEditFiles := false

	fileCount := 30

	defer func() {
		if err := os.RemoveAll(dirTest); err != nil {
			t.Errorf("failed to remove %s after test, delete the dir manually: %v", dirSrc, err)
		}
	}()

	// create dummy files in src directory
	err := os.MkdirAll(dirSrc, 0755)
	require.NoError(t, err)
	for i := range fileCount {
		filePath := filepath.Join(dirSrc, fmt.Sprintf("file%d.%s", i+1, extensionsToCopy[0]))
		if _, err := os.Create(filePath); err != nil {
			require.NoError(t, err)
		}
	}

	totalCopied, removedCount, err := cleanSDCard(editFileExtensions, extensionsToCopy, dirSrc, dirDst, flagDryRun, flagOverwrite, flagDeleteZombieEditFiles)

	assert.NoError(t, err)
	assert.Equal(t, fileCount, totalCopied)
	assert.Equal(t, fileCount, removedCount)

	entries, err := os.ReadDir(dirDst)
	assert.NoError(t, err)
	assert.Equal(t, fileCount, len(entries))

	copiedFiles := make([]string, len(entries))
	for i, entry := range entries {
		copiedFiles[i] = entry.Name()
	}

	expectedFiles := make([]string, fileCount)
	for i := range fileCount {
		expectedFiles[i] = fmt.Sprintf("file%d.%s", i+1, extensionsToCopy[0])
	}

	assert.ElementsMatch(t, copiedFiles, expectedFiles)

	entries, err := os.ReadDir(dirSrc)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(entries))
}