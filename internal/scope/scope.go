// Package scope implements nett's scope engine: the single authority every
// network-active module consults before contacting a target. It is
// fail-closed — a host, IP, or URL is only in scope if it matches an explicit
// include rule (domain, wildcard, hostname, IP, or CIDR) and is not matched by
// any exclude rule. Nothing is implicitly in scope.
//
// Rules are supplied as plain strings (from config.ScopeConfig or CLI
// targets) and self-classified: an entry that parses as an IP is an IP rule, a
// CIDR is a CIDR rule, an entry starting with "/" is a path rule (excludes
// only), and anything else is a domain rule matching itself and every
// subdomain. Wildcard entries (containing "*") are glob-matched.
package scope

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
	"sync"

	"github.com/offsecfergali/nett/internal/config"
)

// Scope is a concurrency-safe set of include/exclude rules. The zero value is
// not usable; construct with New.
type Scope struct {
	mu sync.RWMutex

	domains        []string // normalized apex-style rules: host == d || host endswith "."+d
	wildcards      []*regexp.Regexp
	excludeDomains []string
	excludeWild    []*regexp.Regexp
	excludePaths   []*regexp.Regexp

	ips          map[string]struct{}
	cidrs        []*net.IPNet
	excludeIPs   map[string]struct{}
	excludeCIDRs []*net.IPNet

	// dynamic holds IPs learned at runtime because they were resolved from an
	// in-scope hostname (LearnIP). This is what lets "scope by domain" extend
	// to the IPs that domain actually resolves to, without requiring the user
	// to separately list every IP in config.
	dynamic map[string]struct{}
}

// New builds a Scope from the raw config sections. cfg.Include and cfg.Exclude
// entries are self-classified (IP/CIDR/path/domain); cfg.Wildcards entries are
// always treated as glob patterns matched against the hostname.
func New(cfg config.ScopeConfig) (*Scope, error) {
	s := &Scope{
		ips:        make(map[string]struct{}),
		excludeIPs: make(map[string]struct{}),
		dynamic:    make(map[string]struct{}),
	}
	for _, raw := range cfg.Include {
		if err := s.addInclude(raw); err != nil {
			return nil, fmt.Errorf("scope: include %q: %w", raw, err)
		}
	}
	for _, raw := range cfg.Exclude {
		if err := s.addExclude(raw); err != nil {
			return nil, fmt.Errorf("scope: exclude %q: %w", raw, err)
		}
	}
	for _, raw := range cfg.Wildcards {
		re, err := globToRegexp(raw)
		if err != nil {
			return nil, fmt.Errorf("scope: wildcard %q: %w", raw, err)
		}
		s.wildcards = append(s.wildcards, re)
	}
	return s, nil
}

// AddDomain adds an apex domain rule at runtime (host itself and every
// subdomain). Callers (e.g. the scan command) use this to bring CLI-supplied
// targets into scope without requiring the user to duplicate them in config.
func (s *Scope) AddDomain(domain string) {
	d := normalizeHost(domain)
	if d == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.domains = append(s.domains, d)
}

// LearnIP marks ip as dynamically in scope because it was observed by
// resolving host, which was itself already in scope. It is a no-op if host is
// not in scope — callers should still check AllowHost before resolving.
func (s *Scope) LearnIP(host string, ip net.IP) {
	if ip == nil || !s.AllowHost(host) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dynamic[ip.String()] = struct{}{}
}

// AllowHost reports whether host is in scope: matched by an include rule
// (domain or wildcard) and not matched by any exclude rule.
func (s *Scope) AllowHost(host string) bool {
	h := normalizeHost(host)
	if h == "" {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, d := range s.excludeDomains {
		if hostMatchesDomain(h, d) {
			return false
		}
	}
	for _, re := range s.excludeWild {
		if re.MatchString(h) {
			return false
		}
	}
	for _, d := range s.domains {
		if hostMatchesDomain(h, d) {
			return true
		}
	}
	for _, re := range s.wildcards {
		if re.MatchString(h) {
			return true
		}
	}
	return false
}

// AllowIP reports whether ip is in scope: matched by a static include rule
// (IP or CIDR), or previously learned via LearnIP from an in-scope host, and
// not matched by any exclude rule.
func (s *Scope) AllowIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	key := ip.String()
	s.mu.RLock()
	defer s.mu.RUnlock()

	if _, excluded := s.excludeIPs[key]; excluded {
		return false
	}
	for _, n := range s.excludeCIDRs {
		if n.Contains(ip) {
			return false
		}
	}
	if _, ok := s.ips[key]; ok {
		return true
	}
	if _, ok := s.dynamic[key]; ok {
		return true
	}
	for _, n := range s.cidrs {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// AllowURL reports whether rawURL is in scope: its host must be in scope via
// AllowHost, and its path must not match any excluded path pattern.
func (s *Scope) AllowURL(rawURL string) (bool, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false, fmt.Errorf("scope: parse url %q: %w", rawURL, err)
	}
	if u.Hostname() == "" {
		return false, fmt.Errorf("scope: url %q has no host", rawURL)
	}
	if !s.AllowHost(u.Hostname()) {
		return false, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	path := u.Path
	if path == "" {
		path = "/"
	}
	for _, re := range s.excludePaths {
		if re.MatchString(path) {
			return false, nil
		}
	}
	return true, nil
}

func (s *Scope) addInclude(raw string) error {
	if ip := net.ParseIP(raw); ip != nil {
		s.mu.Lock()
		s.ips[ip.String()] = struct{}{}
		s.mu.Unlock()
		return nil
	}
	if _, n, err := net.ParseCIDR(raw); err == nil {
		s.mu.Lock()
		s.cidrs = append(s.cidrs, n)
		s.mu.Unlock()
		return nil
	}
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil {
			return fmt.Errorf("not a valid URL: %w", err)
		}
		s.AddDomain(u.Hostname())
		return nil
	}
	if strings.Contains(raw, "*") {
		re, err := globToRegexp(raw)
		if err != nil {
			return err
		}
		s.mu.Lock()
		s.wildcards = append(s.wildcards, re)
		s.mu.Unlock()
		return nil
	}
	s.AddDomain(raw)
	return nil
}

func (s *Scope) addExclude(raw string) error {
	if ip := net.ParseIP(raw); ip != nil {
		s.mu.Lock()
		s.excludeIPs[ip.String()] = struct{}{}
		s.mu.Unlock()
		return nil
	}
	if _, n, err := net.ParseCIDR(raw); err == nil {
		s.mu.Lock()
		s.excludeCIDRs = append(s.excludeCIDRs, n)
		s.mu.Unlock()
		return nil
	}
	if strings.HasPrefix(raw, "/") {
		re, err := globToRegexp(raw)
		if err != nil {
			return err
		}
		s.mu.Lock()
		s.excludePaths = append(s.excludePaths, re)
		s.mu.Unlock()
		return nil
	}
	if strings.Contains(raw, "*") {
		re, err := globToRegexp(raw)
		if err != nil {
			return err
		}
		s.mu.Lock()
		s.excludeWild = append(s.excludeWild, re)
		s.mu.Unlock()
		return nil
	}
	d := normalizeHost(raw)
	if d == "" {
		return fmt.Errorf("empty rule")
	}
	s.mu.Lock()
	s.excludeDomains = append(s.excludeDomains, d)
	s.mu.Unlock()
	return nil
}

// hostMatchesDomain reports whether host equals domain or is a subdomain of
// it (e.g. "api.example.com" matches domain "example.com").
func hostMatchesDomain(host, domain string) bool {
	return host == domain || strings.HasSuffix(host, "."+domain)
}

// normalizeHost lowercases and strips a trailing dot / leading "*." so
// wildcard-shaped domain entries (e.g. rejected as domains) never sneak in
// unnormalized; wildcard entries are handled separately via globToRegexp.
func normalizeHost(host string) string {
	h := strings.ToLower(strings.TrimSpace(host))
	h = strings.TrimSuffix(h, ".")
	return h
}

// globToRegexp compiles a case-insensitive glob (only "*" is special,
// matching any sequence including empty) into an anchored regexp.
func globToRegexp(pattern string) (*regexp.Regexp, error) {
	p := strings.ToLower(strings.TrimSpace(pattern))
	var b strings.Builder
	b.WriteString("(?i)^")
	for _, part := range strings.Split(p, "*") {
		b.WriteString(regexp.QuoteMeta(part))
		b.WriteString(".*")
	}
	s := strings.TrimSuffix(b.String(), ".*")
	s += "$"
	return regexp.Compile(s)
}
