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

// sourceFile identifies a file discovered below a source directory. Files are
// copied into the destination by base name to preserve the existing flat
// destination layout while allowing the source tree to be traversed
// recursively.
type sourceFile struct {
	name string
	path string
}

// listFilesRecursively returns every file below dir. Each directory is read
// once so callers can reuse the result for multiple extension groups and
// optional source cleanup.
func listFilesRecursively(fsys FileSystem, dir string) ([]sourceFile, error) {
	entries, err := fsys.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading directory %s: %w", dir, err)
	}

	var files []sourceFile
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		if entry.IsDir() {
			nestedFiles, err := listFilesRecursively(fsys, path)
			if err != nil {
				return nil, err
			}
			files = append(files, nestedFiles...)
			continue
		}

		files = append(files, sourceFile{name: entry.Name(), path: path})
	}

	return files, nil
}

// forEachEntryConcurrently runs fn for each entry, aggregating the increments
// fn reports and any errors it returns. At most maxConcurrency invocations of
// fn run at once (values <= 0 are treated as 1), so callers touching a
// bottlenecked device (e.g. an SD card) can bound how many concurrent
// operations hit it instead of spawning one goroutine per entry.
func forEachEntryConcurrently(entries []os.DirEntry, maxConcurrency int, fn func(entry os.DirEntry) (int, error)) (int, error) {
	if maxConcurrency <= 0 {
		maxConcurrency = 1
	}

	var count atomic.Int32
	var wg sync.WaitGroup
	errsChan := make(chan error, len(entries))
	sem := make(chan struct{}, maxConcurrency)

	for _, entry := range entries {
		sem <- struct{}{}
		wg.Go(func() {
			defer func() { <-sem }()
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
// once per group. At most maxConcurrency files are copied at once.
// If flagDryRun is true, it counts files without copying.
// If flagOverwrite is true, it overwrites existing files in dstDir.
// It returns the number of files copied and any error.
func copyFiles(fsys FileSystem, entries []os.DirEntry, srcDir, dstDir string, exts []string, flagDryRun, flagOverwrite bool, maxConcurrency int) (int, error) {
	files := make([]sourceFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		files = append(files, sourceFile{name: entry.Name(), path: filepath.Join(srcDir, entry.Name())})
	}

	return copySourceFiles(fsys, files, dstDir, exts, flagDryRun, flagOverwrite, maxConcurrency)
}

// copySourceFiles copies matching source files into a flat destination
// directory. At most maxConcurrency files are copied at once.
func copySourceFiles(fsys FileSystem, files []sourceFile, dstDir string, exts []string, flagDryRun, flagOverwrite bool, maxConcurrency int) (int, error) {
	return forEachSourceFileConcurrently(files, maxConcurrency, func(file sourceFile) (int, error) {
		if !matchesAnyExtension(file.name, exts) {
			return 0, nil
		}

		dstPath := filepath.Join(dstDir, file.name)

		if !flagOverwrite {
			if _, statErr := fsys.Stat(dstPath); statErr == nil {
				log.Printf("skipping copying existing file: %s\n", file.name)
				return 0, nil
			}
		}

		if flagDryRun {
			log.Printf("[dry-run] would copy %s\n", file.path)
			return 1, nil
		}

		if copyErr := fsys.CopyFile(file.path, dstPath); copyErr != nil {
			return 0, fileCopyError{fileName: file.path, err: copyErr}
		}

		log.Printf("copied %s\n", file.path)
		return 1, nil
	})
}

// forEachSourceFileConcurrently is the sourceFile counterpart to
// forEachEntryConcurrently.
func forEachSourceFileConcurrently(files []sourceFile, maxConcurrency int, fn func(file sourceFile) (int, error)) (int, error) {
	if maxConcurrency <= 0 {
		maxConcurrency = 1
	}

	var count atomic.Int32
	var wg sync.WaitGroup
	errsChan := make(chan error, len(files))
	sem := make(chan struct{}, maxConcurrency)

	for _, file := range files {
		sem <- struct{}{}
		wg.Go(func() {
			defer func() { <-sem }()
			n, err := fn(file)
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

// removeFiles removes all files in entries, a directory listing of dir
// supplied by the caller (see copyFiles). At most maxConcurrency files are
// removed at once.
// It returns the number of files removed and any error.
func removeFiles(fsys FileSystem, entries []os.DirEntry, dir string, maxConcurrency int) (int, error) {
	files := make([]sourceFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		files = append(files, sourceFile{name: entry.Name(), path: filepath.Join(dir, entry.Name())})
	}

	return removeSourceFiles(fsys, files, maxConcurrency)
}

// removeSourceFiles removes all supplied source files. It intentionally leaves
// their containing directories in place.
func removeSourceFiles(fsys FileSystem, files []sourceFile, maxConcurrency int) (int, error) {
	return forEachSourceFileConcurrently(files, maxConcurrency, func(file sourceFile) (int, error) {
		if err := fsys.Remove(file.path); err != nil {
			return 0, fmt.Errorf("failed to remove file %s: %w", file.path, err)
		}
		log.Printf("removed %s\n", file.path)
		return 1, nil
	})
}

// deleteZombieEditFiles deletes edit files that have no corresponding raw file.
// It checks for raw files with extensions in rawFileExtensions.
// If isRecursive is true, it processes subdirectories recursively. At most
// maxConcurrency entries are processed at once per directory level.
// It returns the number of files deleted and any error.
func deleteZombieEditFiles(fsys FileSystem, editFileExtension, dir string, rawFileExtensions []string, isRecursive bool, maxConcurrency int) (int, error) {
	entries, err := fsys.ReadDir(dir)
	if err != nil {
		return 0, fmt.Errorf("reading directory: %w", err)
	}

	return forEachEntryConcurrently(entries, maxConcurrency, func(entry os.DirEntry) (int, error) {
		if entry.IsDir() {
			if !isRecursive {
				return 0, nil
			}
			n, err := deleteZombieEditFiles(fsys, editFileExtension, filepath.Join(dir, entry.Name()), rawFileExtensions, isRecursive, maxConcurrency)
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
