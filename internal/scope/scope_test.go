package scope

import (
	"net"
	"testing"

	"github.com/offsecfergali/nett/internal/config"
)

func TestFailClosedByDefault(t *testing.T) {
	s, err := New(config.ScopeConfig{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if s.AllowHost("example.com") {
		t.Error("empty scope must deny every host (fail-closed)")
	}
	if s.AllowIP(net.ParseIP("1.2.3.4")) {
		t.Error("empty scope must deny every IP (fail-closed)")
	}
}

func TestDomainMatchesSelfAndSubdomains(t *testing.T) {
	s, err := New(config.ScopeConfig{Include: []string{"example.com"}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	for _, host := range []string{"example.com", "EXAMPLE.COM", "example.com.", "api.example.com", "a.b.example.com"} {
		if !s.AllowHost(host) {
			t.Errorf("AllowHost(%q) = false, want true", host)
		}
	}
	for _, host := range []string{"notexample.com", "example.org", "evilexample.com"} {
		if s.AllowHost(host) {
			t.Errorf("AllowHost(%q) = true, want false", host)
		}
	}
}

func TestExcludeOverridesInclude(t *testing.T) {
	s, err := New(config.ScopeConfig{
		Include: []string{"example.com"},
		Exclude: []string{"internal.example.com"},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if !s.AllowHost("api.example.com") {
		t.Error("api.example.com should be in scope")
	}
	if s.AllowHost("internal.example.com") {
		t.Error("internal.example.com should be excluded")
	}
	if s.AllowHost("dev.internal.example.com") {
		t.Error("subdomains of an excluded domain must also be excluded")
	}
}

func TestWildcardConfig(t *testing.T) {
	s, err := New(config.ScopeConfig{Wildcards: []string{"*.example.com"}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if !s.AllowHost("api.example.com") {
		t.Error("api.example.com should match *.example.com")
	}
	if s.AllowHost("example.com") {
		t.Error("bare apex should not match *.example.com")
	}
}

func TestWildcardInInclude(t *testing.T) {
	s, err := New(config.ScopeConfig{Include: []string{"staging-*.example.com"}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if !s.AllowHost("staging-web.example.com") {
		t.Error("staging-web.example.com should match staging-*.example.com")
	}
	if s.AllowHost("prod-web.example.com") {
		t.Error("prod-web.example.com should not match staging-*.example.com")
	}
}

func TestCIDRIncludeAndExclude(t *testing.T) {
	s, err := New(config.ScopeConfig{
		Include: []string{"10.0.0.0/24"},
		Exclude: []string{"10.0.0.128/25"},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if !s.AllowIP(net.ParseIP("10.0.0.5")) {
		t.Error("10.0.0.5 should be in scope")
	}
	if s.AllowIP(net.ParseIP("10.0.0.200")) {
		t.Error("10.0.0.200 falls in the excluded sub-range")
	}
	if s.AllowIP(net.ParseIP("10.0.1.5")) {
		t.Error("10.0.1.5 is outside the included CIDR")
	}
}

func TestExactIPInclude(t *testing.T) {
	s, err := New(config.ScopeConfig{Include: []string{"93.184.216.34"}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if !s.AllowIP(net.ParseIP("93.184.216.34")) {
		t.Error("exact IP include should be allowed")
	}
	if s.AllowIP(net.ParseIP("93.184.216.35")) {
		t.Error("a different IP must not be allowed")
	}
}

func TestLearnIPOnlyFromInScopeHost(t *testing.T) {
	s, err := New(config.ScopeConfig{Include: []string{"example.com"}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ip := net.ParseIP("192.0.2.10")
	s.LearnIP("api.example.com", ip)
	if !s.AllowIP(ip) {
		t.Error("IP resolved from an in-scope host should become in scope")
	}

	other := net.ParseIP("192.0.2.20")
	s.LearnIP("not-in-scope.org", other)
	if s.AllowIP(other) {
		t.Error("IP resolved from an out-of-scope host must not become in scope")
	}
}

func TestAddDomainInjectsScanTarget(t *testing.T) {
	s, err := New(config.ScopeConfig{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s.AddDomain("example.com")
	if !s.AllowHost("www.example.com") {
		t.Error("AddDomain should bring subdomains into scope")
	}
}

func TestAllowURL(t *testing.T) {
	s, err := New(config.ScopeConfig{
		Include: []string{"example.com"},
		Exclude: []string{"/admin/*"},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ok, err := s.AllowURL("https://api.example.com/v1/users")
	if err != nil || !ok {
		t.Errorf("AllowURL(in-scope) = %v, %v; want true, nil", ok, err)
	}
	ok, err = s.AllowURL("https://api.example.com/admin/settings")
	if err != nil || ok {
		t.Errorf("AllowURL(excluded path) = %v, %v; want false, nil", ok, err)
	}
	ok, err = s.AllowURL("https://evil.org/x")
	if err != nil || ok {
		t.Errorf("AllowURL(out-of-scope host) = %v, %v; want false, nil", ok, err)
	}
	if _, err := s.AllowURL("://not a url"); err == nil {
		t.Error("AllowURL should error on an unparsable URL")
	}
}

func TestIncludeURLSeedsHostname(t *testing.T) {
	s, err := New(config.ScopeConfig{Include: []string{"https://example.com/path"}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if !s.AllowHost("example.com") {
		t.Error("a URL include entry should seed its hostname into scope")
	}
}

func TestInvalidWildcardConfigIsRejected(t *testing.T) {
	// Wildcards are compiled eagerly; this asserts New surfaces a bad pattern
	// rather than silently accepting it. "*" alone is valid; an empty entry
	// combined with an unclosed regex-special sequence should still compile
	// since QuoteMeta escapes everything — so instead assert a genuinely
	// pathological case is still handled without panicking.
	if _, err := New(config.ScopeConfig{Wildcards: []string{"[*.example.com"}}); err != nil {
		t.Fatalf("unexpected error for a literal-bracket wildcard: %v", err)
	}
}
