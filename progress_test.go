package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCopyFileReportsByteProgress(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.mp4")
	dst := filepath.Join(dir, "destination.mp4")
	content := bytes.Repeat([]byte("video data"), 100000)
	require.NoError(t, os.WriteFile(src, content, 0644))

	progress := &progressDisplay{active: make(map[string]*fileProgress)}
	fsys := osFileSystem{progress: progress}
	require.NoError(t, fsys.CopyFile(src, dst))

	got, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, content, got)
	assert.Equal(t, int64(len(content)), progress.bytes)
	assert.Equal(t, 1, progress.copied)
	assert.Empty(t, progress.active)

	require.NoError(t, fsys.Remove(src))
	assert.Equal(t, 1, progress.removed)
}

func TestCopyFileProgressReportsFailure(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.mp4")
	dst := filepath.Join(dir, "destination.mp4")
	require.NoError(t, os.WriteFile(src, []byte("video"), 0644))
	require.NoError(t, os.Mkdir(dst, 0755))

	progress := &progressDisplay{active: make(map[string]*fileProgress)}
	err := (osFileSystem{progress: progress}).CopyFile(src, dst)
	require.Error(t, err)
	assert.Zero(t, progress.copied)
	assert.Equal(t, 1, progress.failed)
	assert.Empty(t, progress.active)
}

func TestProgressDisplayPreservesLogMessages(t *testing.T) {
	var output bytes.Buffer
	p := &progressDisplay{out: &output, active: make(map[string]*fileProgress)}
	p.Begin("clip.mp4", 100)
	p.Advance("clip.mp4", 50)
	p.mu.Lock()
	p.renderLocked()
	p.mu.Unlock()

	_, err := p.Write([]byte("copy warning\n"))
	require.NoError(t, err)
	assert.Contains(t, output.String(), "copy warning\n")
	assert.Contains(t, output.String(), "[########--------]  50%")
	assert.True(t, strings.Index(output.String(), "copy warning") < strings.LastIndex(output.String(), "clip.mp4"))
}
