// Package ratelimit provides a small, dependency-free token-bucket limiter
// shared by every active module (DNS, HTTP, port scanning, content
// discovery, ...) so "respect the configured rate limit" is implemented once
// and used everywhere, per docs/ARCHITECTURE.md's concurrency model.
package ratelimit

import (
	"context"
	"sync"
	"time"
)

// Limiter is a token-bucket rate limiter safe for concurrent use. A
// non-positive rate means unlimited: Wait always returns immediately.
type Limiter struct {
	mu     sync.Mutex
	rate   float64 // tokens added per second
	burst  float64 // bucket capacity
	tokens float64
	last   time.Time
}

// New returns a Limiter allowing ratePerSecond operations per second on
// average, with a burst capacity equal to one second's worth of tokens
// (minimum 1). A ratePerSecond <= 0 disables limiting entirely.
func New(ratePerSecond float64) *Limiter {
	burst := ratePerSecond
	if burst < 1 {
		burst = 1
	}
	return &Limiter{
		rate:   ratePerSecond,
		burst:  burst,
		tokens: burst,
		last:   time.Now(),
	}
}

// Wait blocks until a token is available or ctx is done, whichever comes
// first. It returns ctx.Err() if the context is cancelled first.
func (l *Limiter) Wait(ctx context.Context) error {
	if l.rate <= 0 {
		return nil
	}
	for {
		wait, ok := l.take()
		if ok {
			return nil
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// take attempts to consume one token. On success it returns (0, true). On
// failure it returns the duration the caller should wait before retrying.
func (l *Limiter) take() (time.Duration, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(l.last).Seconds()
	l.tokens += elapsed * l.rate
	if l.tokens > l.burst {
		l.tokens = l.burst
	}
	l.last = now

	if l.tokens >= 1 {
		l.tokens--
		return 0, true
	}
	deficit := 1 - l.tokens
	return time.Duration(deficit / l.rate * float64(time.Second)), false
}
