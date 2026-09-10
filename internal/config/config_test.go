package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultIsValid(t *testing.T) {
	if err := Default().Validate(); err != nil {
		t.Fatalf("Default() must be valid, got: %v", err)
	}
}

func TestValidateCatchesProblems(t *testing.T) {
	c := Default()
	c.Project = ""
	c.Logging.Level = "loud"
	c.Logging.Format = "xml"
	c.Concurrency.Global = 0
	c.HTTP.TimeoutSeconds = -1
	c.DNS.Protocol = "carrier-pigeon"
	c.DNS.Resolvers = nil

	err := c.Validate()
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	for _, want := range []string{
		"project must not be empty",
		`logging.level "loud" invalid`,
		`logging.format "xml" invalid`,
		"concurrency.global must be > 0",
		"http.timeout_seconds must be > 0",
		`dns.protocol "carrier-pigeon" invalid`,
		"dns.resolvers must not be empty",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error missing %q\nfull error:\n%s", want, err)
		}
	}
}

func TestApplyEnvOverrides(t *testing.T) {
	env := map[string]string{
		"NETT_PROJECT":       "acme",
		"NETT_LOG_LEVEL":     "debug",
		"NETT_LOG_FORMAT":    "json",
		"NETT_CONCURRENCY":   "7",
		"NETT_HTTP_TIMEOUT":  "3",
		"NETT_DNS_RESOLVERS": "9.9.9.9:53, 8.8.4.4:53",
		"NETT_DNS_PROTOCOL":  "tcp",
		"NETT_SCOPE_INCLUDE": "example.com,*.example.com",
	}
	c := Default()
	if err := c.ApplyEnv(func(k string) string { return env[k] }); err != nil {
		t.Fatalf("ApplyEnv: %v", err)
	}
	if c.Project != "acme" {
		t.Errorf("project = %q, want acme", c.Project)
	}
	if c.Logging.Level != "debug" || c.Logging.Format != "json" {
		t.Errorf("logging = %+v", c.Logging)
	}
	if c.Concurrency.Global != 7 {
		t.Errorf("concurrency.global = %d, want 7", c.Concurrency.Global)
	}
	if c.HTTP.TimeoutSeconds != 3 {
		t.Errorf("http.timeout = %d, want 3", c.HTTP.TimeoutSeconds)
	}
	if got := c.DNS.Resolvers; len(got) != 2 || got[0] != "9.9.9.9:53" || got[1] != "8.8.4.4:53" {
		t.Errorf("dns.resolvers = %v", got)
	}
	if c.DNS.Protocol != "tcp" {
		t.Errorf("dns.protocol = %q, want tcp", c.DNS.Protocol)
	}
	if got := c.Scope.Include; len(got) != 2 || got[0] != "example.com" || got[1] != "*.example.com" {
		t.Errorf("scope.include = %v", got)
	}
}

func TestApplyEnvRejectsBadNumbers(t *testing.T) {
	env := map[string]string{"NETT_HTTP_TIMEOUT": "soon"}
	err := Default().ApplyEnv(func(k string) string { return env[k] })
	if err == nil || !strings.Contains(err.Error(), "NETT_HTTP_TIMEOUT") {
		t.Fatalf("expected timeout parse error, got: %v", err)
	}
}

func TestLoadMergesFileOverDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.json")
	body := `{
	  "project": "from-file",
	  "logging": {"level": "warn", "format": "json"},
	  "dns": {"protocol": "tcp"}
	}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Project != "from-file" {
		t.Errorf("project = %q, want from-file", c.Project)
	}
	if c.Logging.Level != "warn" || c.Logging.Format != "json" {
		t.Errorf("logging = %+v", c.Logging)
	}
	if c.DNS.Protocol != "tcp" {
		t.Errorf("dns.protocol = %q, want tcp", c.DNS.Protocol)
	}
	// Fields not in the file keep their defaults.
	if len(c.DNS.Resolvers) == 0 {
		t.Error("dns.resolvers should retain defaults when absent from file")
	}
}

func TestLoadRejectsUnknownField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.json")
	if err := os.WriteFile(path, []byte(`{"projekt":"typo"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected error for unknown field, got nil")
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "nope.json")); err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestJSONRoundTripsProject(t *testing.T) {
	s, err := Default().JSON()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s, `"project": "default"`) {
		t.Errorf("JSON missing project field:\n%s", s)
	}
}
