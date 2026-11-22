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

func TestOverallCopyProgressIncludesPhotosAndVideos(t *testing.T) {
	fsys := newFakeFileSystem()
	photoDir := "photos"
	videoDir := "videos"
	rawPath := filepath.Join(photoDir, "photo.raw")
	jpgPath := filepath.Join(photoDir, "photo.jpg")
	videoPath := filepath.Join(videoDir, "clip.mp4")
	xmlPath := filepath.Join(videoDir, "clip.xml")
	fsys.addFile(rawPath, strings.Repeat("r", 100))
	fsys.addFile(jpgPath, strings.Repeat("j", 50))
	fsys.addFile(videoPath, strings.Repeat("v", 200))
	fsys.addFile(xmlPath, strings.Repeat("x", 10))
	fsys.addFile(filepath.Join(videoDir, "thumbnail.jpg"), "ignored")
	photos, err := listFilesRecursively(fsys, photoDir)
	require.NoError(t, err)
	videos, err := fsys.ReadDir(videoDir)
	require.NoError(t, err)

	plan, err := buildCopyPlan(fsys, photos, videos, videoDir, []string{"raw"}, []string{"jpg"}, []string{"mp4", "xml"}, true)
	require.NoError(t, err)
	assert.Len(t, plan.sizes, 4)
	assert.Equal(t, int64(360), plan.bytes)

	var output bytes.Buffer
	p := &progressDisplay{out: &output, active: make(map[string]*fileProgress)}
	p.SetPlan(plan)
	p.Begin(rawPath, 100)
	p.Advance(rawPath, 100)
	p.Copied(rawPath)
	p.Skipped(jpgPath)
	p.Begin(videoPath, 200)
	p.Advance(videoPath, 100)
	percent, done := p.overallLocked()
	assert.Equal(t, 69, percent)
	assert.Equal(t, int64(250), done)
	p.renderLocked()
	assert.Contains(t, output.String(), "69%  2/4 files  250 B/360 B")

	p.Advance(videoPath, 100)
	p.Copied(videoPath)
	p.Skipped(xmlPath)
	percent, done = p.overallLocked()
	assert.Equal(t, 100, percent)
	assert.Equal(t, int64(360), done)

	withoutJPG, err := buildCopyPlan(fsys, photos, videos, videoDir, []string{"raw"}, []string{"jpg"}, []string{"mp4", "xml"}, false)
	require.NoError(t, err)
	assert.Len(t, withoutJPG.sizes, 3)
	assert.Equal(t, int64(310), withoutJPG.bytes)
}

func TestPreparedCopyReusesScannedSourceDirectories(t *testing.T) {
	base := newFakeFileSystem()
	base.addFile(filepath.Join("photos", "photo.raw"), "raw")
	base.addFile(filepath.Join("videos", "clip.mp4"), "video")
	fsys := newReadDirCountingFileSystem(base)
	photos, err := listFilesRecursively(fsys, "photos")
	require.NoError(t, err)
	videos, err := fsys.ReadDir("videos")
	require.NoError(t, err)
	_, err = buildCopyPlan(fsys, photos, videos, "videos", []string{"raw"}, nil, []string{"mp4"}, false)
	require.NoError(t, err)

	_, _, err = cleanSDCardWithFiles(fsys, photos, nil, []string{"raw"}, nil,
		"photos", "raw-dst", "jpg-dst", Options{KeepSrc: true, Concurrency: 2})
	require.NoError(t, err)
	_, _, err = copyVideoFilesWithEntries(fsys, videos, []string{"mp4"},
		"videos", "video-dst", Options{KeepSrc: true, Concurrency: 2})
	require.NoError(t, err)
	assert.Equal(t, 1, fsys.callsFor("photos"))
	assert.Equal(t, 1, fsys.callsFor("videos"))
}
