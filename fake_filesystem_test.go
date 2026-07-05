package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// fakeFileSystem is an in-memory FileSystem used in tests so that test
// correctness doesn't depend on the real disk (and its OS-specific quirks,
// such as transient file locks held by indexers/antivirus on Windows).
type fakeFileSystem struct {
	mu    sync.Mutex
	files map[string][]byte
	dirs  map[string]bool
}

func newFakeFileSystem() *fakeFileSystem {
	return &fakeFileSystem{
		files: make(map[string][]byte),
		dirs:  map[string]bool{".": true},
	}
}

func cleanFakePath(p string) string {
	return filepath.ToSlash(filepath.Clean(p))
}

// addFile seeds a file (and its parent directories) for a test.
func (f *fakeFileSystem) addFile(path, content string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	path = cleanFakePath(path)
	f.files[path] = []byte(content)
	f.markDirTree(cleanFakePath(filepath.Dir(path)))
}

// addDir seeds an (empty) directory for a test.
func (f *fakeFileSystem) addDir(path string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.markDirTree(cleanFakePath(path))
}

// markDirTree marks path and all of its ancestors as existing directories.
// Callers must hold f.mu.
func (f *fakeFileSystem) markDirTree(path string) {
	for {
		f.dirs[path] = true
		parent := cleanFakePath(filepath.Dir(path))
		if parent == path {
			return
		}
		path = parent
	}
}

func (f *fakeFileSystem) ReadDir(dir string) ([]os.DirEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	dir = cleanFakePath(dir)
	if !f.dirs[dir] {
		return nil, fmt.Errorf("readdir %s: %w", dir, os.ErrNotExist)
	}

	isDirByName := make(map[string]bool)
	for path := range f.files {
		if cleanFakePath(filepath.Dir(path)) == dir {
			isDirByName[filepath.Base(path)] = false
		}
	}
	for path := range f.dirs {
		if path == dir {
			continue
		}
		if cleanFakePath(filepath.Dir(path)) == dir {
			isDirByName[filepath.Base(path)] = true
		}
	}

	entries := make([]os.DirEntry, 0, len(isDirByName))
	for name, isDir := range isDirByName {
		entries = append(entries, fakeDirEntry{name: name, isDir: isDir})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	return entries, nil
}

func (f *fakeFileSystem) Stat(path string) (os.FileInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	path = cleanFakePath(path)
	if content, ok := f.files[path]; ok {
		return fakeFileInfo{name: filepath.Base(path), size: int64(len(content))}, nil
	}
	if f.dirs[path] {
		return fakeFileInfo{name: filepath.Base(path), isDir: true}, nil
	}
	return nil, fmt.Errorf("stat %s: %w", path, os.ErrNotExist)
}

func (f *fakeFileSystem) Remove(path string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	path = cleanFakePath(path)
	if _, ok := f.files[path]; !ok {
		return fmt.Errorf("remove %s: %w", path, os.ErrNotExist)
	}
	delete(f.files, path)
	return nil
}

func (f *fakeFileSystem) MkdirAll(path string, _ os.FileMode) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.markDirTree(cleanFakePath(path))
	return nil
}

func (f *fakeFileSystem) CopyFile(src, dst string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	src = cleanFakePath(src)
	content, ok := f.files[src]
	if !ok {
		return fmt.Errorf("open %s: %w", src, os.ErrNotExist)
	}

	dst = cleanFakePath(dst)
	f.files[dst] = append([]byte(nil), content...)
	f.markDirTree(cleanFakePath(filepath.Dir(dst)))
	return nil
}

type fakeDirEntry struct {
	name  string
	isDir bool
}

func (e fakeDirEntry) Name() string { return e.name }
func (e fakeDirEntry) IsDir() bool  { return e.isDir }

func (e fakeDirEntry) Type() fs.FileMode {
	if e.isDir {
		return fs.ModeDir
	}
	return 0
}

func (e fakeDirEntry) Info() (fs.FileInfo, error) {
	return fakeFileInfo{name: e.name, isDir: e.isDir}, nil
}

type fakeFileInfo struct {
	name  string
	size  int64
	isDir bool
}

func (i fakeFileInfo) Name() string { return i.name }
func (i fakeFileInfo) Size() int64  { return i.size }

func (i fakeFileInfo) Mode() fs.FileMode {
	if i.isDir {
		return fs.ModeDir
	}
	return 0
}

func (i fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (i fakeFileInfo) IsDir() bool        { return i.isDir }
func (i fakeFileInfo) Sys() any           { return nil }
