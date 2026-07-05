package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
)

// FileSystem abstracts the filesystem operations that copyFiles, removeFiles,
// and deleteZombieEditFiles depend on, so that callers (tests, in particular)
// can substitute a fake implementation instead of touching the real disk.
type FileSystem interface {
	ReadDir(dir string) ([]os.DirEntry, error)
	Stat(path string) (os.FileInfo, error)
	Remove(path string) error
	MkdirAll(path string, perm os.FileMode) error
	CopyFile(src, dst string) error
}

// osFileSystem implements FileSystem using the real OS filesystem.
type osFileSystem struct{}

func (osFileSystem) ReadDir(dir string) ([]os.DirEntry, error) {
	return os.ReadDir(dir)
}

func (osFileSystem) Stat(path string) (os.FileInfo, error) {
	return os.Stat(path)
}

func (osFileSystem) Remove(path string) error {
	return os.Remove(path)
}

func (osFileSystem) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

func (osFileSystem) CopyFile(src, dst string) error {
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

type fileCopyError struct {
	fileName string
	err      error
}

func (e fileCopyError) Error() string {
	return fmt.Sprintf("failed to copy file %s: %s", e.fileName, e.err.Error())
}

// forEachEntryConcurrently runs fn concurrently for each entry, aggregating
// the increments fn reports and any errors it returns.
func forEachEntryConcurrently(entries []os.DirEntry, fn func(entry os.DirEntry) (int, error)) (int, error) {
	var count atomic.Int32
	var wg sync.WaitGroup
	errsChan := make(chan error, len(entries))

	for _, entry := range entries {
		wg.Go(func() {
			n, err := fn(entry)
			if err != nil {
				errsChan <- err
				return
			}
			count.Add(int32(n))
		})
	}

	wg.Wait()
	close(errsChan)

	var errs error
	for e := range errsChan {
		errs = errors.Join(errs, e)
	}

	return int(count.Load()), errs
}

// matchesAnyExtension reports whether name has one of exts as its extension.
func matchesAnyExtension(name string, exts []string) bool {
	nameExt := filepath.Ext(name)
	for _, ext := range exts {
		if strings.EqualFold(nameExt, "."+ext) {
			return true
		}
	}
	return false
}

// copyFiles copies entries whose extension is in exts from srcDir to dstDir.
// entries is a directory listing of srcDir supplied by the caller so that a
// single srcDir listing can be shared across multiple extension groups
// instead of re-reading the (potentially slow, e.g. SD card) source directory
// once per group.
// If flagDryRun is true, it counts files without copying.
// If flagOverwrite is true, it overwrites existing files in dstDir.
// It returns the number of files copied and any error.
func copyFiles(fsys FileSystem, entries []os.DirEntry, srcDir, dstDir string, exts []string, flagDryRun, flagOverwrite bool) (int, error) {
	return forEachEntryConcurrently(entries, func(entry os.DirEntry) (int, error) {
		if entry.IsDir() {
			return 0, nil
		}

		name := entry.Name()
		if !matchesAnyExtension(name, exts) {
			return 0, nil
		}

		srcPath := filepath.Join(srcDir, name)
		dstPath := filepath.Join(dstDir, name)

		if !flagOverwrite {
			if _, statErr := fsys.Stat(dstPath); statErr == nil {
				log.Printf("skipping copying existing file: %s\n", name)
				return 0, nil
			}
		}

		if flagDryRun {
			log.Printf("[dry-run] would copy %s\n", name)
			return 1, nil
		}

		if copyErr := fsys.CopyFile(srcPath, dstPath); copyErr != nil {
			return 0, fileCopyError{fileName: name, err: copyErr}
		}

		log.Printf("copied %s\n", name)
		return 1, nil
	})
}

// removeFiles removes all files in entries, a directory listing of dir
// supplied by the caller (see copyFiles).
// It returns the number of files removed and any error.
func removeFiles(fsys FileSystem, entries []os.DirEntry, dir string) (int, error) {
	return forEachEntryConcurrently(entries, func(entry os.DirEntry) (int, error) {
		if entry.IsDir() {
			return 0, nil
		}

		path := filepath.Join(dir, entry.Name())
		if err := fsys.Remove(path); err != nil {
			return 0, fmt.Errorf("failed to remove file %s: %w", entry.Name(), err)
		}
		log.Printf("removed %s\n", entry.Name())
		return 1, nil
	})
}

// deleteZombieEditFiles deletes edit files that have no corresponding raw file.
// It checks for raw files with extensions in rawFileExtensions.
// If isRecursive is true, it processes subdirectories recursively.
// It returns the number of files deleted and any error.
func deleteZombieEditFiles(fsys FileSystem, editFileExtension, dir string, rawFileExtensions []string, isRecursive bool) (int, error) {
	entries, err := fsys.ReadDir(dir)
	if err != nil {
		return 0, fmt.Errorf("reading directory: %w", err)
	}

	return forEachEntryConcurrently(entries, func(entry os.DirEntry) (int, error) {
		if entry.IsDir() {
			if !isRecursive {
				return 0, nil
			}
			n, err := deleteZombieEditFiles(fsys, editFileExtension, filepath.Join(dir, entry.Name()), rawFileExtensions, isRecursive)
			if err != nil {
				return 0, fmt.Errorf("failed to process subdirectory %s: %w", entry.Name(), err)
			}
			return n, nil
		}

		editFileName := entry.Name()
		if !strings.HasSuffix(editFileName, "."+editFileExtension) {
			return 0, nil
		}

		editFileNameWithoutExt := strings.TrimSuffix(editFileName, "."+editFileExtension)

		for _, rawFileExt := range rawFileExtensions {
			expectedRawFileName := editFileNameWithoutExt + "." + rawFileExt
			if _, err := fsys.Stat(filepath.Join(dir, expectedRawFileName)); err == nil {
				return 0, nil
			} else if !errors.Is(err, os.ErrNotExist) {
				return 0, fmt.Errorf("failed to check if %s exists: %w", expectedRawFileName, err)
			}
		}

		if err := fsys.Remove(filepath.Join(dir, editFileName)); err != nil {
			return 0, fmt.Errorf("failed to remove zombie edit file %s: %w", editFileName, err)
		}

		log.Printf("removed zombie edit file: %s\n", editFileName)
		return 1, nil
	})
}
