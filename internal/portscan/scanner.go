// Package portscan implements nett's native TCP connect scanner. It never
// shells out to nmap/naabu/masscan; every connection attempt is a plain Go
// net.Dial against the target, gated by the caller's scope check and a
// shared rate limiter.
package portscan

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/offsecfergali/nett/internal/ratelimit"
	"github.com/offsecfergali/nett/internal/workerpool"
)

// State is the outcome of probing one port.
type State string

const (
	StateOpen     State = "open"
	StateClosed   State = "closed"
	StateFiltered State = "filtered" // timed out: probably dropped by a firewall
	StateUnknown  State = "unknown"  // an error other than timeout/refused
)

// Result is the outcome of scanning one port.
type Result struct {
	IP      net.IP
	Port    int
	State   State
	Latency time.Duration
	Err     string // populated for StateUnknown
}

// Config bounds a Scanner's behavior; callers derive it from
// config.ConcurrencyConfig and their own timeout/rate policy.
type Config struct {
	Concurrency     int
	Timeout         time.Duration
	RateLimitPerSec float64
}

// Scanner performs bounded-concurrency, rate-limited TCP connect scans.
// Scope enforcement is the caller's responsibility (done once per target IP
// before Scan is invoked), not the scanner's — this keeps the primitive a
// plain, independently testable network operation, per
// docs/ARCHITECTURE.md §3's "shared capabilities have no knowledge of the
// pipeline."
type Scanner struct {
	cfg     Config
	limiter *ratelimit.Limiter
}

// New builds a Scanner from cfg.
func New(cfg Config) *Scanner {
	if cfg.Concurrency < 1 {
		cfg.Concurrency = 1
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 3 * time.Second
	}
	return &Scanner{cfg: cfg, limiter: ratelimit.New(cfg.RateLimitPerSec)}
}

// Scan connects to ip on every port in ports and reports each outcome. It
// returns early (with whatever results were gathered) if ctx is cancelled.
func (s *Scanner) Scan(ctx context.Context, ip net.IP, ports []int) []Result {
	results := make([]Result, 0, len(ports))
	var mu sync.Mutex

	workerpool.Run(ctx, s.cfg.Concurrency, ports, func(ctx context.Context, port int) {
		if err := s.limiter.Wait(ctx); err != nil {
			return
		}
		r := s.probe(ctx, ip, port)
		mu.Lock()
		results = append(results, r)
		mu.Unlock()
	})
	return results
}

func (s *Scanner) probe(ctx context.Context, ip net.IP, port int) Result {
	addr := net.JoinHostPort(ip.String(), fmt.Sprintf("%d", port))
	dctx, cancel := context.WithTimeout(ctx, s.cfg.Timeout)
	defer cancel()

	start := time.Now()
	var d net.Dialer
	conn, err := d.DialContext(dctx, "tcp", addr)
	latency := time.Since(start)
	if err == nil {
		conn.Close()
		return Result{IP: ip, Port: port, State: StateOpen, Latency: latency}
	}
	return Result{IP: ip, Port: port, State: classifyError(err), Latency: latency, Err: errString(err)}
}

// classifyError distinguishes "actively refused" (closed) from "no response
// within the timeout" (filtered — likely dropped by a firewall) from
// anything else (unknown), matching how a researcher reads a connect scan.
func classifyError(err error) State {
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return StateFiltered
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return StateClosed
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		var sysErr *os.SyscallError
		if errors.As(opErr.Err, &sysErr) && errors.Is(sysErr.Err, syscall.ECONNREFUSED) {
			return StateClosed
		}
	}
	return StateUnknown
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
