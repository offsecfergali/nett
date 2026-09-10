package ratelimit

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestUnlimitedNeverBlocks(t *testing.T) {
	l := New(0)
	ctx := context.Background()
	start := time.Now()
	for i := 0; i < 1000; i++ {
		if err := l.Wait(ctx); err != nil {
			t.Fatalf("Wait: %v", err)
		}
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Errorf("unlimited limiter took %v for 1000 waits, want near-instant", elapsed)
	}
}

func TestLimiterThrottles(t *testing.T) {
	l := New(50) // 50/sec, burst 50
	ctx := context.Background()

	// Drain the initial burst instantly.
	for i := 0; i < 50; i++ {
		if err := l.Wait(ctx); err != nil {
			t.Fatalf("Wait (burst): %v", err)
		}
	}

	// The next 25 tokens must be rate-limited to ~50/sec, so 25 of them
	// should take at least ~400ms (allowing generous scheduling slack).
	start := time.Now()
	for i := 0; i < 25; i++ {
		if err := l.Wait(ctx); err != nil {
			t.Fatalf("Wait (throttled): %v", err)
		}
	}
	elapsed := time.Since(start)
	if elapsed < 300*time.Millisecond {
		t.Errorf("25 tokens at 50/sec took %v, want at least ~400ms", elapsed)
	}
}

func TestLimiterRespectsContextCancellation(t *testing.T) {
	l := New(1) // very slow: 1/sec
	ctx := context.Background()
	if err := l.Wait(ctx); err != nil {
		t.Fatalf("first Wait: %v", err)
	}

	cctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err := l.Wait(cctx)
	if err == nil {
		t.Error("Wait should have returned an error once the context deadline passed")
	}
}

func TestLimiterConcurrentUse(t *testing.T) {
	l := New(200)
	ctx := context.Background()
	var count int64
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				if err := l.Wait(ctx); err != nil {
					t.Errorf("Wait: %v", err)
					return
				}
				atomic.AddInt64(&count, 1)
			}
		}()
	}
	wg.Wait()
	if count != 200 {
		t.Errorf("count = %d, want 200", count)
	}
}
