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
	mu      sync.Mutex
	out     io.Writer
	active  map[string]*fileProgress
	lines   int
	copied  int
	skipped int
	failed  int
	removed int
	bytes   int64
	stop    chan struct{}
	done    chan struct{}
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
	p.copied++
	p.mu.Unlock()
}

func (p *progressDisplay) Failed(path string) {
	p.mu.Lock()
	delete(p.active, path)
	p.failed++
	p.mu.Unlock()
}

func (p *progressDisplay) Skipped() {
	p.mu.Lock()
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
	fmt.Fprintf(p.out, "Copied: %d (%s)  Active: %d  Skipped: %d  Failed: %d  Removed: %d\n",
		p.copied, progressBytes(p.bytes), len(p.active), p.skipped, p.failed, p.removed)
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
	fmt.Fprintf(p.out, "Copied: %d (%s)  Skipped: %d  Failed: %d  Removed: %d\n",
		p.copied, progressBytes(p.bytes), p.skipped, p.failed, p.removed)
	p.mu.Unlock()
}
