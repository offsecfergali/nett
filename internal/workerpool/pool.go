// Package workerpool provides the one bounded-concurrency primitive every
// active module (port scanning, brute force, HTTP probing, ...) runs its work
// through, per docs/ARCHITECTURE.md §4: "no one goroutine per target."
package workerpool

import (
	"context"
	"sync"
)

// Run calls fn once for each item in items, running at most concurrency
// invocations of fn concurrently. It returns once every item has been
// processed or ctx is cancelled, whichever comes first; fn itself is
// responsible for checking ctx and returning promptly if it is cancelled
// mid-flight (e.g. by deriving dial/request timeouts from it).
func Run[T any](ctx context.Context, concurrency int, items []T, fn func(context.Context, T)) {
	if concurrency < 1 {
		concurrency = 1
	}
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

loop:
	for _, item := range items {
		// Checked explicitly (not just as a select case) because a send on
		// a non-full buffered channel is always immediately ready, which
		// would otherwise let a cancelled context lose the select race
		// indefinitely once workers are draining fast.
		if ctx.Err() != nil {
			break loop
		}
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			break loop
		}
		wg.Add(1)
		go func(it T) {
			defer wg.Done()
			defer func() { <-sem }()
			fn(ctx, it)
		}(item)
	}
	wg.Wait()
}
