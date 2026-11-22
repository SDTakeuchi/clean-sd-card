package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// progressDisplay owns all terminal output while copying is in progress.
// Log messages pass through Write so they do not overwrite the live bars.
type progressDisplay struct {
	mu            sync.Mutex
	out           io.Writer
	active        map[string]*fileProgress
	plan          copyPlan
	planned       bool
	finishedBytes int64
	lines         int
	copied        int
	skipped       int
	failed        int
	removed       int
	bytes         int64
	stop          chan struct{}
	done          chan struct{}
}

type copyPlan struct {
	sizes map[string]int64
	bytes int64
}

// buildCopyPlan measures every file eligible for copying. Skipped files stay
// in the plan because deciding to skip them is still completed work.
func buildCopyPlan(fsys FileSystem, photos []sourceFile, videos []os.DirEntry, videoDir string, rawExts, jpgExts, videoExts []string, keepJPG bool) (copyPlan, error) {
	plan := copyPlan{sizes: make(map[string]int64)}
	add := func(path string) error {
		if _, exists := plan.sizes[path]; exists {
			return nil
		}
		info, err := fsys.Stat(path)
		if err != nil {
			return fmt.Errorf("measuring source file %s: %w", path, err)
		}
		plan.sizes[path] = info.Size()
		plan.bytes += info.Size()
		return nil
	}
	for _, file := range photos {
		if matchesAnyExtension(file.name, rawExts) || (keepJPG && matchesAnyExtension(file.name, jpgExts)) {
			if err := add(file.path); err != nil {
				return copyPlan{}, err
			}
		}
	}
	for _, entry := range videos {
		if !entry.IsDir() && matchesAnyExtension(entry.Name(), videoExts) {
			if err := add(filepath.Join(videoDir, entry.Name())); err != nil {
				return copyPlan{}, err
			}
		}
	}
	return plan, nil
}

func (p *progressDisplay) SetPlan(plan copyPlan) {
	p.mu.Lock()
	p.plan = plan
	p.planned = true
	p.mu.Unlock()
}

type fileProgress struct {
	size int64
	done int64
}

type progressWriter struct {
	io.Writer
	progress *progressDisplay
	path     string
}

func (w progressWriter) Write(p []byte) (int, error) {
	n, err := w.Writer.Write(p)
	w.progress.Advance(w.path, int64(n))
	return n, err
}

func progressFor(fsys FileSystem) *progressDisplay {
	if f, ok := fsys.(osFileSystem); ok {
		return f.progress
	}
	return nil
}

func newTerminalProgress(out *os.File) *progressDisplay {
	info, err := out.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 || os.Getenv("TERM") == "dumb" || !enableTerminalOutput(out) {
		return nil
	}
	p := &progressDisplay{
		out:    out,
		active: make(map[string]*fileProgress),
		stop:   make(chan struct{}),
		done:   make(chan struct{}),
	}
	go p.run()
	return p
}

func (p *progressDisplay) run() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	defer close(p.done)
	for {
		select {
		case <-ticker.C:
			p.mu.Lock()
			p.renderLocked()
			p.mu.Unlock()
		case <-p.stop:
			return
		}
	}
}

func (p *progressDisplay) Begin(path string, size int64) {
	p.mu.Lock()
	p.active[path] = &fileProgress{size: size}
	p.mu.Unlock()
}

func (p *progressDisplay) Advance(path string, n int64) {
	p.mu.Lock()
	if file := p.active[path]; file != nil {
		file.done += n
	}
	p.bytes += n
	p.mu.Unlock()
}

func (p *progressDisplay) Copied(path string) {
	p.mu.Lock()
	delete(p.active, path)
	p.finishedBytes += p.plan.sizes[path]
	p.copied++
	p.mu.Unlock()
}

func (p *progressDisplay) Failed(path string) {
	p.mu.Lock()
	delete(p.active, path)
	p.failed++
	p.mu.Unlock()
}

func (p *progressDisplay) Skipped(path string) {
	p.mu.Lock()
	p.finishedBytes += p.plan.sizes[path]
	p.skipped++
	p.mu.Unlock()
}

func (p *progressDisplay) Removed() {
	p.mu.Lock()
	p.removed++
	p.mu.Unlock()
}

func (p *progressDisplay) clearLocked() {
	for range p.lines {
		fmt.Fprint(p.out, "\x1b[1A\r\x1b[2K")
	}
	p.lines = 0
}

func (p *progressDisplay) renderLocked() {
	p.clearLocked()
	if p.planned {
		percent, done := p.overallLocked()
		const width = 20
		filled := percent * width / 100
		fmt.Fprintf(p.out, "Overall [%s%s] %3d%%  %d/%d files  %s/%s\n",
			strings.Repeat("#", filled), strings.Repeat("-", width-filled),
			percent, p.copied+p.skipped, len(p.plan.sizes), progressBytes(done), progressBytes(p.plan.bytes))
	} else {
		fmt.Fprintln(p.out, "Scanning source files...")
	}
	p.lines++
	fmt.Fprintf(p.out, "Transferred: %s  Active: %d  Skipped: %d  Failed: %d  Removed: %d\n",
		progressBytes(p.bytes), len(p.active), p.skipped, p.failed, p.removed)
	p.lines++
	paths := make([]string, 0, len(p.active))
	for path := range p.active {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		file := p.active[path]
		nameRunes := []rune(filepath.Base(path))
		if len(nameRunes) > 24 {
			nameRunes = append(nameRunes[:21], '.', '.', '.')
		}
		name := string(nameRunes)
		if file.size <= 0 {
			fmt.Fprintf(p.out, "  %-24s %s\n", name, progressBytes(file.done))
		} else {
			const width = 16
			filled := int(file.done * width / file.size)
			if filled > width {
				filled = width
			}
			fmt.Fprintf(p.out, "  %-24s [%s%s] %3d%% %s/%s\n", name,
				strings.Repeat("#", filled), strings.Repeat("-", width-filled),
				file.done*100/file.size, progressBytes(file.done), progressBytes(file.size))
		}
		p.lines++
	}
}

func (p *progressDisplay) overallLocked() (int, int64) {
	if len(p.plan.sizes) == 0 {
		return 100, 0
	}
	done := p.finishedBytes
	for path, file := range p.active {
		size := p.plan.sizes[path]
		if file.done < size {
			done += file.done
		} else {
			done += size
		}
	}
	if done > p.plan.bytes {
		done = p.plan.bytes
	}
	if p.plan.bytes == 0 {
		return (p.copied + p.skipped) * 100 / len(p.plan.sizes), 0
	}
	return int(done * 100 / p.plan.bytes), done
}

func progressBytes(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	value := float64(n)
	for _, unit := range []string{"KiB", "MiB", "GiB", "TiB"} {
		value /= 1024
		if value < 1024 {
			return fmt.Sprintf("%.1f %s", value, unit)
		}
	}
	return fmt.Sprintf("%.1f PiB", value/1024)
}

// Write satisfies io.Writer for the standard logger.
func (p *progressDisplay) Write(message []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.clearLocked()
	n, err := p.out.Write(message)
	p.renderLocked()
	return n, err
}

func (p *progressDisplay) Close() {
	close(p.stop)
	<-p.done
	p.mu.Lock()
	p.clearLocked()
	if p.planned {
		percent, _ := p.overallLocked()
		fmt.Fprintf(p.out, "Overall: %d%% (%d/%d files)  Transferred: %s  Skipped: %d  Failed: %d  Removed: %d\n",
			percent, p.copied+p.skipped, len(p.plan.sizes), progressBytes(p.bytes), p.skipped, p.failed, p.removed)
	}
	p.mu.Unlock()
}
