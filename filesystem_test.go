package main

import (
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestForEachEntryConcurrentlyBoundsConcurrency(t *testing.T) {
	const maxConcurrency = 3
	const entryCount = 20

	entries := make([]os.DirEntry, entryCount)
	for i := range entries {
		entries[i] = fakeDirEntry{name: fmt.Sprintf("file%d", i)}
	}

	var current atomic.Int32
	var maxObserved atomic.Int32

	_, err := forEachEntryConcurrently(entries, maxConcurrency, func(entry os.DirEntry) (int, error) {
		n := current.Add(1)
		defer current.Add(-1)

		for {
			observed := maxObserved.Load()
			if n <= observed || maxObserved.CompareAndSwap(observed, n) {
				break
			}
		}

		// Give other goroutines a chance to overlap so the bound is
		// actually exercised, not just trivially satisfied.
		time.Sleep(5 * time.Millisecond)
		return 1, nil
	})

	assert.NoError(t, err)
	assert.LessOrEqual(t, int(maxObserved.Load()), maxConcurrency, "concurrency exceeded the configured limit")
	assert.Equal(t, int32(maxConcurrency), maxObserved.Load(), "expected concurrency to actually reach the configured limit, not stay needlessly under it")
}
