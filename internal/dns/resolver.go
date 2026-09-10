package dns

import (
	"context"
	"crypto/tls"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/offsecfergali/nett/internal/config"
	"github.com/offsecfergali/nett/internal/ratelimit"
)

// defaultNegativeTTL is used to cache an NXDOMAIN/empty answer when the
// response carries no SOA record to derive one from (RFC 2308 recommends
// using the zone's SOA MINIMUM; we fall back to a conservative constant).
const defaultNegativeTTL = 30 * time.Second

// Resolver is a caching DNS resolver that queries a configured list of
// upstream resolvers over UDP, TCP, DoH, or DoT. It never consults the
// system resolver or shells out to any external program.
type Resolver struct {
	cfg    config.DNSConfig
	limit  *ratelimit.Limiter
	client *http.Client // used only when cfg.Protocol == "doh"

	mu    sync.Mutex
	next  int // round-robin index into cfg.Resolvers
	cache map[string]cacheEntry
}

type cacheEntry struct {
	records []Record
	rcode   Rcode
	expires time.Time
}

// New builds a Resolver from the given configuration. It returns an error if
// no resolvers are configured or the protocol is unrecognized.
func New(cfg config.DNSConfig) (*Resolver, error) {
	if len(cfg.Resolvers) == 0 {
		return nil, fmt.Errorf("dns: no resolvers configured")
	}
	switch cfg.Protocol {
	case "udp", "tcp", "doh", "dot":
	default:
		return nil, fmt.Errorf("dns: unknown protocol %q", cfg.Protocol)
	}
	return &Resolver{
		cfg:    cfg,
		limit:  ratelimit.New(cfg.RateLimitPerSec),
		client: &http.Client{Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second},
		cache:  make(map[string]cacheEntry),
	}, nil
}

// Query resolves name for qtype, using the cache when possible and otherwise
// querying upstream with retries across every configured resolver. The
// returned records are exactly those from the answer section with the
// requested owner name and type (CNAME chases are the caller's concern via
// LookupA/LookupAAAA, which do chase them).
func (r *Resolver) Query(ctx context.Context, name string, qtype RRType) ([]Record, Rcode, error) {
	key := cacheKey(name, qtype)
	if recs, rcode, ok := r.cacheGet(key); ok {
		return recs, rcode, nil
	}

	if err := r.limit.Wait(ctx); err != nil {
		return nil, 0, err
	}

	resp, err := r.exchangeWithRetry(ctx, name, qtype)
	if err != nil {
		return nil, 0, err
	}

	recs := filterAnswers(resp.Answers, qtype)
	ttl := negativeTTLFrom(resp)
	if len(recs) > 0 {
		ttl = minTTL(recs)
	}
	r.cachePut(key, recs, resp.Rcode, ttl)
	return recs, resp.Rcode, nil
}

func (r *Resolver) exchangeWithRetry(ctx context.Context, name string, qtype RRType) (*Message, error) {
	var lastErr error
	attempts := r.cfg.Retries + 1
	if attempts < 1 {
		attempts = 1
	}
	timeout := time.Duration(r.cfg.TimeoutSeconds) * time.Second

	for attempt := 0; attempt < attempts; attempt++ {
		addr := r.pickResolver()
		id := uint16(rand.Intn(1 << 16))
		query := NewQuery(id, name, qtype)

		var (
			resp *Message
			err  error
		)
		switch r.cfg.Protocol {
		case "udp":
			resp, err = exchangeUDP(ctx, addr, query, timeout)
		case "tcp":
			resp, err = exchangeTCP(ctx, addr, query, timeout)
		case "dot":
			resp, err = exchangeDoT(ctx, addr, query, timeout, &tls.Config{ServerName: hostOnly(addr)})
		case "doh":
			resp, err = exchangeDoH(ctx, r.client, addr, query)
		default:
			return nil, fmt.Errorf("dns: unknown protocol %q", r.cfg.Protocol)
		}
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}
	return nil, fmt.Errorf("dns: query %s %s failed after %d attempt(s): %w", name, qtype, attempts, lastErr)
}

// pickResolver round-robins across the configured resolver list so load (and
// any single resolver's rate limit) is spread out.
func (r *Resolver) pickResolver() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	addr := r.cfg.Resolvers[r.next%len(r.cfg.Resolvers)]
	r.next++
	return addr
}

func hostOnly(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return host
}

func cacheKey(name string, qtype RRType) string {
	return strings.ToLower(strings.TrimSuffix(name, ".")) + "\x00" + qtype.String()
}

func (r *Resolver) cacheGet(key string) ([]Record, Rcode, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.cache[key]
	if !ok || time.Now().After(e.expires) {
		return nil, 0, false
	}
	return e.records, e.rcode, true
}

func (r *Resolver) cachePut(key string, recs []Record, rcode Rcode, ttl time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cache[key] = cacheEntry{records: recs, rcode: rcode, expires: time.Now().Add(ttl)}
}

// filterAnswers returns the answers matching qtype. A recursive resolver
// flattens a CNAME chain into one response, so a matching A/AAAA record's
// owner name is the chain's final target, not the name that was queried —
// filtering by name as well as type would incorrectly discard exactly the
// answers a chased query is looking for, so type is the only filter here.
func filterAnswers(answers []Record, qtype RRType) []Record {
	var out []Record
	for _, a := range answers {
		if a.Type == qtype {
			out = append(out, a)
		}
	}
	return out
}

func minTTL(recs []Record) time.Duration {
	min := recs[0].TTL
	for _, r := range recs[1:] {
		if r.TTL < min {
			min = r.TTL
		}
	}
	return time.Duration(min) * time.Second
}

// negativeTTLFrom derives a negative-caching TTL from the response's SOA
// authority record per RFC 2308, falling back to a constant when the
// response carries no SOA (a resolver is not obligated to attach one).
func negativeTTLFrom(resp *Message) time.Duration {
	for _, rr := range resp.Authority {
		if rr.Type == TypeSOA && rr.SOA != nil {
			return time.Duration(rr.SOA.Minimum) * time.Second
		}
	}
	return defaultNegativeTTL
}

// --- typed lookup convenience methods -------------------------------------

// LookupA returns the A records for name, following at most one CNAME chain
// hop returned inline by the resolver (a second Query call resolves the
// eventual target if the first response was CNAME-only).
func (r *Resolver) LookupA(ctx context.Context, name string) ([]net.IP, error) {
	return r.lookupIP(ctx, name, TypeA)
}

// LookupAAAA returns the AAAA records for name.
func (r *Resolver) LookupAAAA(ctx context.Context, name string) ([]net.IP, error) {
	return r.lookupIP(ctx, name, TypeAAAA)
}

func (r *Resolver) lookupIP(ctx context.Context, name string, qtype RRType) ([]net.IP, error) {
	seen := map[string]bool{}
	current := name
	for hop := 0; hop < 8; hop++ {
		if seen[strings.ToLower(current)] {
			return nil, fmt.Errorf("dns: CNAME loop resolving %s", name)
		}
		seen[strings.ToLower(current)] = true

		recs, rcode, err := r.Query(ctx, current, qtype)
		if err != nil {
			return nil, err
		}
		if len(recs) > 0 {
			ips := make([]net.IP, 0, len(recs))
			for _, rr := range recs {
				addr := rr.A
				if qtype == TypeAAAA {
					addr = rr.AAAA
				}
				ips = append(ips, net.ParseIP(addr))
			}
			return ips, nil
		}
		if rcode == RcodeNameError {
			return nil, nil
		}
		cnames, _, err := r.Query(ctx, current, TypeCNAME)
		if err != nil {
			return nil, err
		}
		if len(cnames) == 0 {
			return nil, nil
		}
		current = cnames[0].CNAME
	}
	return nil, fmt.Errorf("dns: too many CNAME hops resolving %s", name)
}

// LookupCNAME returns the canonical name for name, or "" if there is none.
func (r *Resolver) LookupCNAME(ctx context.Context, name string) (string, error) {
	recs, _, err := r.Query(ctx, name, TypeCNAME)
	if err != nil || len(recs) == 0 {
		return "", err
	}
	return recs[0].CNAME, nil
}

// LookupNS returns the nameservers for name.
func (r *Resolver) LookupNS(ctx context.Context, name string) ([]string, error) {
	recs, _, err := r.Query(ctx, name, TypeNS)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(recs))
	for i, rr := range recs {
		out[i] = rr.NS
	}
	return out, nil
}

// LookupTXT returns the TXT records for name, each joined into one string.
func (r *Resolver) LookupTXT(ctx context.Context, name string) ([]string, error) {
	recs, _, err := r.Query(ctx, name, TypeTXT)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(recs))
	for i, rr := range recs {
		out[i] = strings.Join(rr.TXT, "")
	}
	return out, nil
}

// LookupMX returns the mail exchangers for name.
func (r *Resolver) LookupMX(ctx context.Context, name string) ([]*MX, error) {
	recs, _, err := r.Query(ctx, name, TypeMX)
	if err != nil {
		return nil, err
	}
	out := make([]*MX, len(recs))
	for i, rr := range recs {
		out[i] = rr.MX
	}
	return out, nil
}

// LookupSRV returns the SRV records for name (e.g. "_sip._tcp.example.com").
func (r *Resolver) LookupSRV(ctx context.Context, name string) ([]*SRV, error) {
	recs, _, err := r.Query(ctx, name, TypeSRV)
	if err != nil {
		return nil, err
	}
	out := make([]*SRV, len(recs))
	for i, rr := range recs {
		out[i] = rr.SRV
	}
	return out, nil
}

// LookupCAA returns the CAA records for name.
func (r *Resolver) LookupCAA(ctx context.Context, name string) ([]*CAA, error) {
	recs, _, err := r.Query(ctx, name, TypeCAA)
	if err != nil {
		return nil, err
	}
	out := make([]*CAA, len(recs))
	for i, rr := range recs {
		out[i] = rr.CAA
	}
	return out, nil
}

// LookupSOA returns the SOA record for name, or nil if there is none.
func (r *Resolver) LookupSOA(ctx context.Context, name string) (*SOA, error) {
	recs, _, err := r.Query(ctx, name, TypeSOA)
	if err != nil || len(recs) == 0 {
		return nil, err
	}
	return recs[0].SOA, nil
}

// LookupPTR returns the reverse-DNS names for ip.
func (r *Resolver) LookupPTR(ctx context.Context, ip net.IP) ([]string, error) {
	arpa, err := reverseAddr(ip)
	if err != nil {
		return nil, err
	}
	recs, _, err := r.Query(ctx, arpa, TypePTR)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(recs))
	for i, rr := range recs {
		out[i] = rr.PTR
	}
	return out, nil
}

// reverseAddr builds the in-addr.arpa (IPv4) or ip6.arpa (IPv6) name for ip.
func reverseAddr(ip net.IP) (string, error) {
	if ip4 := ip.To4(); ip4 != nil {
		return fmt.Sprintf("%d.%d.%d.%d.in-addr.arpa", ip4[3], ip4[2], ip4[1], ip4[0]), nil
	}
	ip6 := ip.To16()
	if ip6 == nil {
		return "", fmt.Errorf("dns: invalid IP %v", ip)
	}
	const hexDigit = "0123456789abcdef"
	var labels [32]byte
	for i := 0; i < 16; i++ {
		labels[i*2] = hexDigit[ip6[15-i]&0xF]
		labels[i*2+1] = hexDigit[ip6[15-i]>>4]
	}
	var b strings.Builder
	for _, c := range labels {
		b.WriteByte(c)
		b.WriteByte('.')
	}
	b.WriteString("ip6.arpa")
	return b.String(), nil
}
