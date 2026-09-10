package portscan

import (
	"context"
	"errors"
	"net"
	"strconv"
	"testing"
	"time"
)

func listenerPort(t *testing.T) (int, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()
	_, portStr, _ := net.SplitHostPort(ln.Addr().String())
	port, _ := strconv.Atoi(portStr)
	return port, func() { ln.Close() }
}

func closedPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	_, portStr, _ := net.SplitHostPort(ln.Addr().String())
	port, _ := strconv.Atoi(portStr)
	ln.Close() // nothing listens here now; loopback connections are refused
	return port
}

func TestScanDetectsOpenPort(t *testing.T) {
	port, cleanup := listenerPort(t)
	defer cleanup()

	s := New(Config{Concurrency: 4, Timeout: time.Second})
	results := s.Scan(context.Background(), net.ParseIP("127.0.0.1"), []int{port})
	if len(results) != 1 {
		t.Fatalf("results = %+v, want 1", results)
	}
	if results[0].State != StateOpen {
		t.Errorf("State = %q, want open", results[0].State)
	}
}

func TestScanDetectsClosedPort(t *testing.T) {
	port := closedPort(t)
	s := New(Config{Concurrency: 4, Timeout: time.Second})
	results := s.Scan(context.Background(), net.ParseIP("127.0.0.1"), []int{port})
	if len(results) != 1 {
		t.Fatalf("results = %+v, want 1", results)
	}
	if results[0].State != StateClosed {
		t.Errorf("State = %q, want closed", results[0].State)
	}
}

func TestScanMultiplePorts(t *testing.T) {
	p1, cleanup1 := listenerPort(t)
	defer cleanup1()
	p2, cleanup2 := listenerPort(t)
	defer cleanup2()
	closed := closedPort(t)

	s := New(Config{Concurrency: 2, Timeout: time.Second})
	results := s.Scan(context.Background(), net.ParseIP("127.0.0.1"), []int{p1, p2, closed})
	if len(results) != 3 {
		t.Fatalf("results = %+v, want 3", results)
	}
	states := map[int]State{}
	for _, r := range results {
		states[r.Port] = r.State
	}
	if states[p1] != StateOpen || states[p2] != StateOpen {
		t.Errorf("states = %+v, want %d and %d open", states, p1, p2)
	}
	if states[closed] != StateClosed {
		t.Errorf("states[%d] = %q, want closed", closed, states[closed])
	}
}

func TestScanRespectsContextCancellation(t *testing.T) {
	closed := closedPort(t)
	s := New(Config{Concurrency: 1, Timeout: 5 * time.Second})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	results := s.Scan(ctx, net.ParseIP("127.0.0.1"), []int{closed})
	if len(results) != 0 {
		t.Errorf("results = %+v, want none scanned after cancellation", results)
	}
}

func TestClassifyErrorTimeout(t *testing.T) {
	err := &net.OpError{Op: "dial", Err: timeoutError{}}
	if got := classifyError(err); got != StateFiltered {
		t.Errorf("classifyError(timeout) = %q, want filtered", got)
	}
}

// The ECONNREFUSED branch of classifyError is exercised end-to-end and
// deterministically by TestScanDetectsClosedPort via a real OS-level
// connection refusal; a synthetic error here would not actually be an
// *os.SyscallError wrapping syscall.ECONNREFUSED, so it would not test that
// path.

type timeoutError struct{}

func (timeoutError) Error() string   { return "i/o timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

func TestClassifyErrorUnknown(t *testing.T) {
	if got := classifyError(errors.New("something else")); got != StateUnknown {
		t.Errorf("classifyError(generic) = %q, want unknown", got)
	}
}
