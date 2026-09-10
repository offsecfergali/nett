// Package subdomain orchestrates nett's passive subdomain discovery: it
// queries Certificate Transparency (and, in later milestones, other passive
// providers) for candidate hostnames, enforces scope before resolving any of
// them, validates each candidate against the live native DNS engine, and
// flags results indistinguishable from a domain's own DNS wildcard.
package subdomain

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"

	"github.com/offsecfergali/nett/internal/ct"
	"github.com/offsecfergali/nett/internal/dns"
	"github.com/offsecfergali/nett/internal/scope"
	"github.com/offsecfergali/nett/internal/workerpool"
)

// Finding is one discovered hostname, with its provenance and current DNS
// state.
type Finding struct {
	Hostname      string
	Sources       []string
	IPs           []net.IP
	Resolved      bool
	WildcardMatch bool // true if IPs are indistinguishable from the domain's DNS wildcard
}

// Discover queries every provider for domain, keeps only candidates that
// pass sc (scope is checked before any candidate is resolved — discovery of
// a name is passive, but resolving it is active network contact), resolves
// each survivor concurrently (bounded by concurrency), and reports whether
// it matches the domain's own wildcard answer, if any.
//
// A provider error is logged by the caller via the returned per-provider
// error map's absence of that provider's results; it does not abort
// discovery from the other providers.
func Discover(ctx context.Context, r *dns.Resolver, sc *scope.Scope, providers []ct.Provider, domain string, concurrency int) ([]Finding, map[string]error, error) {
	if !sc.AllowHost(domain) {
		return nil, nil, fmt.Errorf("subdomain: %q is not in scope", domain)
	}

	wildcard, _ := dns.DetectWildcard(ctx, r, domain)

	sources := make(map[string][]string) // hostname -> source names
	providerErrs := make(map[string]error)
	for _, p := range providers {
		hits, err := p.Query(ctx, domain)
		if err != nil {
			providerErrs[p.Name()] = err
			continue
		}
		for _, h := range hits {
			sources[h] = append(sources[h], p.Name())
		}
	}

	var candidates []string
	for h := range sources {
		if sc.AllowHost(h) {
			candidates = append(candidates, h)
		}
	}
	sort.Strings(candidates)

	findings := make(map[string]*Finding, len(candidates))
	for _, h := range candidates {
		findings[h] = &Finding{Hostname: h, Sources: sources[h]}
	}

	var mu sync.Mutex
	workerpool.Run(ctx, concurrency, candidates, func(ctx context.Context, host string) {
		ips, err := r.LookupA(ctx, host)
		if err != nil {
			return
		}
		for _, ip := range ips {
			sc.LearnIP(host, ip)
		}
		mu.Lock()
		f := findings[host]
		f.IPs = ips
		f.Resolved = len(ips) > 0
		f.WildcardMatch = wildcard.Is(ips)
		mu.Unlock()
	})

	out := make([]Finding, 0, len(candidates))
	for _, h := range candidates {
		out = append(out, *findings[h])
	}
	return out, providerErrs, nil
}

// normalizeDomain lowercases and strips a trailing dot, for callers that
// accept a raw CLI argument as the scan target.
func normalizeDomain(domain string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(domain), "."))
}

// NormalizeDomain is the exported form of normalizeDomain, used by the CLI
// before passing a target into Discover or the scope engine.
func NormalizeDomain(domain string) string { return normalizeDomain(domain) }
