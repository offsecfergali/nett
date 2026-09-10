package subdomain

import (
	"context"
	"fmt"
	"net"
	"testing"

	"github.com/offsecfergali/nett/internal/config"
	"github.com/offsecfergali/nett/internal/ct"
	"github.com/offsecfergali/nett/internal/dns"
	"github.com/offsecfergali/nett/internal/scope"
)

type fakeProvider struct {
	name string
	hits []string
	err  error
}

func (f *fakeProvider) Name() string { return f.name }
func (f *fakeProvider) Query(ctx context.Context, domain string) ([]string, error) {
	return f.hits, f.err
}

// startFakeDNS runs a minimal UDP DNS server on loopback for these tests, so
// nothing touches the public Internet. Wildcard probe labels are 48 hex
// characters (24 random bytes); anything shorter is treated as a specific,
// deliberately provisioned name.
func startFakeDNS(t *testing.T, resolves map[string]string, wildcardIP string) *dns.Resolver {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatalf("ListenUDP: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	go func() {
		buf := make([]byte, 4096)
		for {
			n, addr, err := conn.ReadFromUDP(buf)
			if err != nil {
				return
			}
			q, err := dns.Unmarshal(buf[:n])
			if err != nil {
				continue
			}
			name := q.Questions[0].Name
			var resp *dns.Message
			if ip, ok := resolves[name]; ok {
				resp = &dns.Message{Response: true, Questions: q.Questions,
					Answers: []dns.Record{{Name: name, Type: dns.TypeA, TTL: 60, A: ip}}}
			} else if wildcardIP != "" {
				// A real wildcard catches every name not explicitly listed
				// above, regardless of label shape — including both the
				// long random labels DetectWildcard probes with and any
				// ordinary discovered candidate.
				resp = &dns.Message{Response: true, Questions: q.Questions,
					Answers: []dns.Record{{Name: name, Type: dns.TypeA, TTL: 60, A: wildcardIP}}}
			} else {
				resp = &dns.Message{Response: true, Rcode: dns.RcodeNameError, Questions: q.Questions}
			}
			resp.ID = q.ID
			b, err := resp.Marshal()
			if err != nil {
				continue
			}
			conn.WriteToUDP(b, addr)
		}
	}()

	r, err := dns.New(config.DNSConfig{
		Resolvers: []string{conn.LocalAddr().String()}, TimeoutSeconds: 1, Retries: 1, Protocol: "udp",
	})
	if err != nil {
		t.Fatalf("dns.New: %v", err)
	}
	return r
}

func mustScope(t *testing.T, cfg config.ScopeConfig) *scope.Scope {
	t.Helper()
	s, err := scope.New(cfg)
	if err != nil {
		t.Fatalf("scope.New: %v", err)
	}
	return s
}

func TestDiscoverMergesProvidersAndResolves(t *testing.T) {
	r := startFakeDNS(t, map[string]string{
		"api.example.com": "10.0.0.1",
		"www.example.com": "10.0.0.2",
	}, "")
	sc := mustScope(t, config.ScopeConfig{Include: []string{"example.com"}})

	p1 := &fakeProvider{name: "p1", hits: []string{"api.example.com", "dead.example.com"}}
	p2 := &fakeProvider{name: "p2", hits: []string{"www.example.com", "api.example.com"}}

	findings, provErrs, err := Discover(context.Background(), r, sc, providerSlice(p1, p2), "example.com", 4)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(provErrs) != 0 {
		t.Errorf("provErrs = %v, want none", provErrs)
	}
	if len(findings) != 3 {
		t.Fatalf("findings = %+v, want 3", findings)
	}

	byHost := map[string]Finding{}
	for _, f := range findings {
		byHost[f.Hostname] = f
	}

	api := byHost["api.example.com"]
	if !api.Resolved || len(api.IPs) != 1 || api.IPs[0].String() != "10.0.0.1" {
		t.Errorf("api.example.com = %+v", api)
	}
	if len(api.Sources) != 2 {
		t.Errorf("api.example.com sources = %v, want both providers merged", api.Sources)
	}

	dead := byHost["dead.example.com"]
	if dead.Resolved {
		t.Errorf("dead.example.com should not resolve, got %+v", dead)
	}
}

func TestDiscoverFiltersOutOfScope(t *testing.T) {
	r := startFakeDNS(t, map[string]string{"evil.other.org": "10.0.0.9"}, "")
	sc := mustScope(t, config.ScopeConfig{Include: []string{"example.com"}})
	p := &fakeProvider{name: "p1", hits: []string{"api.example.com", "evil.other.org"}}

	findings, _, err := Discover(context.Background(), r, sc, providerSlice(p), "example.com", 4)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	for _, f := range findings {
		if f.Hostname == "evil.other.org" {
			t.Error("out-of-scope hostname must be filtered before resolution")
		}
	}
}

func TestDiscoverRejectsOutOfScopeTarget(t *testing.T) {
	r := startFakeDNS(t, nil, "")
	sc := mustScope(t, config.ScopeConfig{}) // empty scope: nothing is in scope
	_, _, err := Discover(context.Background(), r, sc, nil, "example.com", 4)
	if err == nil {
		t.Error("Discover should refuse a target that is not itself in scope")
	}
}

func TestDiscoverFlagsWildcardMatches(t *testing.T) {
	r := startFakeDNS(t, map[string]string{"real.example.com": "10.0.0.5"}, "198.51.100.1")
	sc := mustScope(t, config.ScopeConfig{Include: []string{"example.com"}})
	p := &fakeProvider{name: "p1", hits: []string{"real.example.com", "random.example.com"}}

	findings, _, err := Discover(context.Background(), r, sc, providerSlice(p), "example.com", 4)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	byHost := map[string]Finding{}
	for _, f := range findings {
		byHost[f.Hostname] = f
	}
	if byHost["real.example.com"].WildcardMatch {
		t.Error("a distinctly-provisioned host must not be flagged as a wildcard match")
	}
	if !byHost["random.example.com"].WildcardMatch {
		t.Error("a name only the wildcard answers for must be flagged as a wildcard match")
	}
}

func TestDiscoverProviderErrorDoesNotAbort(t *testing.T) {
	r := startFakeDNS(t, map[string]string{"good.example.com": "10.0.0.1"}, "")
	sc := mustScope(t, config.ScopeConfig{Include: []string{"example.com"}})
	failing := &fakeProvider{name: "broken", err: fmt.Errorf("boom")}
	working := &fakeProvider{name: "ok", hits: []string{"good.example.com"}}

	findings, provErrs, err := Discover(context.Background(), r, sc, providerSlice(failing, working), "example.com", 4)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if provErrs["broken"] == nil {
		t.Error("provErrs should record the failing provider's error")
	}
	if len(findings) != 1 || findings[0].Hostname != "good.example.com" {
		t.Errorf("findings = %+v, want just good.example.com from the working provider", findings)
	}
}

func TestNormalizeDomain(t *testing.T) {
	if got := NormalizeDomain(" Example.COM. "); got != "example.com" {
		t.Errorf("NormalizeDomain = %q, want example.com", got)
	}
}

// providerSlice adapts *fakeProvider values into a []ct.Provider.
func providerSlice(ps ...*fakeProvider) []ct.Provider {
	out := make([]ct.Provider, len(ps))
	for i, p := range ps {
		out[i] = p
	}
	return out
}
