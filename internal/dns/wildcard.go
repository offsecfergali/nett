package dns

import (
	"context"
	"crypto/rand"
	"fmt"
	"net"
)

// WildcardResult describes whether a domain answers A/AAAA queries for
// arbitrary, never-registered subdomains (a "catch-all" DNS wildcard), and
// if so, which IPs it answers with.
type WildcardResult struct {
	Detected bool
	IPs      []net.IP
}

// Is reports whether ips is indistinguishable from the wildcard's own
// answer — i.e. a discovered subdomain that resolves to exactly the
// wildcard's IP set is almost certainly not a distinct, deliberately
// provisioned host, and callers (brute force, CT discovery) should treat it
// as noise rather than a genuine finding.
func (w *WildcardResult) Is(ips []net.IP) bool {
	if w == nil || !w.Detected || len(ips) == 0 {
		return false
	}
	wildcardSet := make(map[string]struct{}, len(w.IPs))
	for _, ip := range w.IPs {
		wildcardSet[ip.String()] = struct{}{}
	}
	for _, ip := range ips {
		if _, ok := wildcardSet[ip.String()]; !ok {
			return false
		}
	}
	return true
}

// wildcardProbes is the number of random labels queried; more than one
// guards against a transient/misleading single answer.
const wildcardProbes = 3

// DetectWildcard queries wildcardProbes random, near-certainly-unregistered
// subdomains of domain. If one or more resolve, domain has a DNS wildcard;
// the union of every IP any probe resolved to is returned so callers can
// filter genuine discoveries from wildcard noise via Is.
func DetectWildcard(ctx context.Context, r *Resolver, domain string) (*WildcardResult, error) {
	seen := make(map[string]net.IP)
	for i := 0; i < wildcardProbes; i++ {
		label, err := randomLabel(24)
		if err != nil {
			return nil, fmt.Errorf("dns: generate wildcard probe label: %w", err)
		}
		fqdn := label + "." + domain
		ips, err := r.LookupA(ctx, fqdn)
		if err != nil {
			continue // treat a lookup failure as "did not resolve", not a fatal error
		}
		for _, ip := range ips {
			seen[ip.String()] = ip
		}
	}
	if len(seen) == 0 {
		return &WildcardResult{Detected: false}, nil
	}
	out := &WildcardResult{Detected: true}
	for _, ip := range seen {
		out.IPs = append(out.IPs, ip)
	}
	return out, nil
}

// randomLabel returns a lowercase hex label of n bytes' worth of randomness
// (2n hex characters), used to build a subdomain that could not plausibly be
// registered by coincidence.
func randomLabel(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	const hexDigits = "0123456789abcdef"
	out := make([]byte, n*2)
	for i, b := range buf {
		out[i*2] = hexDigits[b>>4]
		out[i*2+1] = hexDigits[b&0xF]
	}
	return string(out), nil
}
