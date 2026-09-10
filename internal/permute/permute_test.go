package permute

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/offsecfergali/nett/internal/config"
	"github.com/offsecfergali/nett/internal/dns"
	"github.com/offsecfergali/nett/internal/scope"
)

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
				// ordinary brute-force candidate.
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

func writeWordlist(t *testing.T, lines ...string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "words.txt")
	content := ""
	for _, l := range lines {
		content += l + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func TestLoadWordlist(t *testing.T) {
	path := writeWordlist(t, "api", "# comment", "", "  www  ", "API")
	words, err := LoadWordlist(path)
	if err != nil {
		t.Fatalf("LoadWordlist: %v", err)
	}
	if len(words) != 2 || words[0] != "api" || words[1] != "www" {
		t.Errorf("words = %v, want [api www] (deduped, trimmed, lowercased, comments/blanks skipped)", words)
	}
}

func TestLoadWordlistMissingFile(t *testing.T) {
	if _, err := LoadWordlist("/nonexistent/path/words.txt"); err == nil {
		t.Error("LoadWordlist should fail for a missing file")
	}
}

func TestLoadWordlistEmpty(t *testing.T) {
	path := writeWordlist(t, "# only a comment", "")
	if _, err := LoadWordlist(path); err == nil {
		t.Error("LoadWordlist should fail for a wordlist with no usable entries")
	}
}

func TestBruteForceFindsLiveHosts(t *testing.T) {
	r := startFakeDNS(t, map[string]string{
		"api.example.com": "10.0.0.1",
		"www.example.com": "10.0.0.2",
	}, "")
	sc := mustScope(t, config.ScopeConfig{Include: []string{"example.com"}})

	findings, err := BruteForce(context.Background(), r, sc, "example.com", []string{"api", "www", "dead"}, false, 4)
	if err != nil {
		t.Fatalf("BruteForce: %v", err)
	}
	if len(findings) != 2 {
		t.Fatalf("findings = %+v, want 2 live hosts", findings)
	}
	hosts := map[string]bool{}
	for _, f := range findings {
		hosts[f.Hostname] = true
		if !f.Resolved {
			t.Errorf("%s should be marked Resolved", f.Hostname)
		}
	}
	if !hosts["api.example.com"] || !hosts["www.example.com"] {
		t.Errorf("hosts = %v, want api and www", hosts)
	}
}

func TestBruteForceRejectsOutOfScopeTarget(t *testing.T) {
	r := startFakeDNS(t, nil, "")
	sc := mustScope(t, config.ScopeConfig{})
	if _, err := BruteForce(context.Background(), r, sc, "example.com", []string{"api"}, false, 4); err == nil {
		t.Error("BruteForce should refuse an out-of-scope target")
	}
}

func TestBruteForceWithPermutations(t *testing.T) {
	r := startFakeDNS(t, map[string]string{
		"dev-api.example.com": "10.0.0.1",
	}, "")
	sc := mustScope(t, config.ScopeConfig{Include: []string{"example.com"}})

	findings, err := BruteForce(context.Background(), r, sc, "example.com", []string{"api"}, true, 4)
	if err != nil {
		t.Fatalf("BruteForce: %v", err)
	}
	found := false
	for _, f := range findings {
		if f.Hostname == "dev-api.example.com" {
			found = true
		}
	}
	if !found {
		t.Errorf("findings = %+v, want dev-api.example.com found via permutation", findings)
	}
}

func TestBruteForceFlagsWildcardMatches(t *testing.T) {
	r := startFakeDNS(t, nil, "198.51.100.1")
	sc := mustScope(t, config.ScopeConfig{Include: []string{"example.com"}})

	findings, err := BruteForce(context.Background(), r, sc, "example.com", []string{"anything"}, false, 4)
	if err != nil {
		t.Fatalf("BruteForce: %v", err)
	}
	if len(findings) != 1 || !findings[0].WildcardMatch {
		t.Errorf("findings = %+v, want anything.example.com resolved and flagged as a wildcard match", findings)
	}
}

func TestGeneratePermutationsIncludesBaseWord(t *testing.T) {
	perms := GeneratePermutations("api")
	found := false
	for _, p := range perms {
		if p == "api" {
			found = true
		}
	}
	if !found {
		t.Error("GeneratePermutations must include the base word itself")
	}
	if len(perms) < 2 {
		t.Error("GeneratePermutations should produce more than just the base word")
	}
}
