package workerpool

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunProcessesEveryItem(t *testing.T) {
	items := make([]int, 100)
	for i := range items {
		items[i] = i
	}
	var sum int64
	Run(context.Background(), 8, items, func(_ context.Context, i int) {
		atomic.AddInt64(&sum, int64(i))
	})
	want := int64(100 * 99 / 2)
	if sum != want {
		t.Errorf("sum = %d, want %d", sum, want)
	}
}

func TestRunBoundsConcurrency(t *testing.T) {
	items := make([]int, 50)
	var current, max int64
	Run(context.Background(), 5, items, func(_ context.Context, _ int) {
		n := atomic.AddInt64(&current, 1)
		for {
			m := atomic.LoadInt64(&max)
			if n <= m || atomic.CompareAndSwapInt64(&max, m, n) {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
		atomic.AddInt64(&current, -1)
	})
	if max > 5 {
		t.Errorf("observed concurrency %d, want <= 5", max)
	}
}

func TestRunStopsOnCancellation(t *testing.T) {
	items := make([]int, 1000)
	ctx, cancel := context.WithCancel(context.Background())
	var processed int64
	cancel() // cancel before starting: nothing should be scheduled at all
	Run(ctx, 4, items, func(_ context.Context, _ int) {
		atomic.AddInt64(&processed, 1)
	})
	if got := atomic.LoadInt64(&processed); got != 0 {
		t.Errorf("processed = %d, want 0 for an already-cancelled context", got)
	}
}

func TestRunEmptyItems(t *testing.T) {
	Run(context.Background(), 4, []int{}, func(_ context.Context, _ int) {
		t.Error("fn should not be called for an empty item list")
	})
}
