// Package permute implements nett's DNS brute-force / permutation engine
// (the -brute flag): given a user-supplied wordlist, it generates candidate
// hostnames, resolves each one through the native DNS engine, and reports
// which are live — filtering out anything that merely reproduces the
// target's own DNS wildcard.
package permute

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/offsecfergali/nett/internal/dns"
	"github.com/offsecfergali/nett/internal/scope"
	"github.com/offsecfergali/nett/internal/workerpool"
)

// Finding is the resolution outcome for one candidate hostname.
type Finding struct {
	Hostname      string
	IPs           []net.IP
	Resolved      bool
	WildcardMatch bool
}

// LoadWordlist reads one label per line from path, trimming whitespace,
// lowercasing, and skipping blank lines and "#" comments.
func LoadWordlist(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("permute: open wordlist %q: %w", path, err)
	}
	defer f.Close()

	seen := make(map[string]struct{})
	var words []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.ToLower(strings.TrimSpace(scanner.Text()))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if _, dup := seen[line]; dup {
			continue
		}
		seen[line] = struct{}{}
		words = append(words, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("permute: read wordlist %q: %w", path, err)
	}
	if len(words) == 0 {
		return nil, fmt.Errorf("permute: wordlist %q contains no usable entries", path)
	}
	return words, nil
}

// commonAffixes are appended/prepended to each base word when permutations
// are enabled, covering the environment-naming conventions real recon turns
// up most often (staging/dev/test hosts, numbered instances).
var commonAffixes = []string{"dev", "staging", "stage", "test", "uat", "qa", "01", "02"}

// GeneratePermutations returns word itself plus a small set of common
// environment-naming variants ("dev-word", "word-dev", "word01", ...), so a
// short wordlist still finds the handful of conventional variants a
// pentester would try by hand.
func GeneratePermutations(word string) []string {
	out := make([]string, 0, 1+len(commonAffixes)*3)
	out = append(out, word)
	for _, affix := range commonAffixes {
		out = append(out, affix+"-"+word, word+"-"+affix, word+affix)
	}
	return out
}

// BruteForce resolves <word>.<domain> for each word in words (or, if
// enablePermutations is set, for every GeneratePermutations(word) variant),
// bounded to concurrency simultaneous DNS lookups. domain must already be
// in scope; every generated candidate is checked against sc before it is
// resolved. Results matching the domain's own DNS wildcard (if any) are
// still returned, with WildcardMatch set, so callers can decide whether to
// discard them — a wildcard match is not proof a host is absent, only that
// its answer is uninformative on its own.
func BruteForce(ctx context.Context, r *dns.Resolver, sc *scope.Scope, domain string, words []string, enablePermutations bool, concurrency int) ([]Finding, error) {
	if !sc.AllowHost(domain) {
		return nil, fmt.Errorf("permute: %q is not in scope", domain)
	}

	wildcard, _ := dns.DetectWildcard(ctx, r, domain)

	seen := make(map[string]struct{})
	var candidates []string
	addCandidate := func(host string) {
		if _, dup := seen[host]; dup {
			return
		}
		if !sc.AllowHost(host) {
			return
		}
		seen[host] = struct{}{}
		candidates = append(candidates, host)
	}
	for _, w := range words {
		labels := []string{w}
		if enablePermutations {
			labels = GeneratePermutations(w)
		}
		for _, label := range labels {
			addCandidate(label + "." + domain)
		}
	}
	sort.Strings(candidates)

	findings := make(map[string]*Finding, len(candidates))
	for _, h := range candidates {
		findings[h] = &Finding{Hostname: h}
	}

	var mu sync.Mutex
	workerpool.Run(ctx, concurrency, candidates, func(ctx context.Context, host string) {
		ips, err := r.LookupA(ctx, host)
		if err != nil || len(ips) == 0 {
			return
		}
		for _, ip := range ips {
			sc.LearnIP(host, ip)
		}
		mu.Lock()
		f := findings[host]
		f.IPs = ips
		f.Resolved = true
		f.WildcardMatch = wildcard.Is(ips)
		mu.Unlock()
	})

	out := make([]Finding, 0, len(candidates))
	for _, h := range candidates {
		if findings[h].Resolved {
			out = append(out, *findings[h])
		}
	}
	return out, nil
}
