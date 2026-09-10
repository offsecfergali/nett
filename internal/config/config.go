// Package config defines nett's configuration and the strict precedence with
// which it is assembled: built-in defaults, then an optional JSON config file,
// then NETT_* environment variables, then command-line flags (applied by the
// cli package). Every configuration is validated before the tool runs; invalid
// values produce descriptive errors rather than silent fallback.
//
// The config format is JSON. This keeps M1 dependency-free (encoding/json is in
// the standard library) so `go build ./...` works in a clean environment with no
// module downloads. A YAML front-end may be added in a later milestone behind a
// general-purpose parser; the in-memory Config is the stable contract either way.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config is the complete effective configuration for a nett run. Sections that
// later milestones consume (HTTP, DNS, Scope) are defined now so the on-disk
// format is stable from the start.
type Config struct {
	// Project is the persistent project name used for resume and diff.
	Project string `json:"project"`
	// DataDir is where the SQLite database and run artifacts live.
	DataDir string `json:"data_dir"`
	// OutputDir is where exports are written.
	OutputDir string `json:"output_dir"`

	Logging     LoggingConfig     `json:"logging"`
	Concurrency ConcurrencyConfig `json:"concurrency"`
	HTTP        HTTPConfig        `json:"http"`
	DNS         DNSConfig         `json:"dns"`
	Scope       ScopeConfig       `json:"scope"`

	// Modules enables/disables individual modules by name. Absent means enabled.
	Modules map[string]bool `json:"modules"`
}

// LoggingConfig controls structured logging.
type LoggingConfig struct {
	// Level is one of: debug, info, warn, error.
	Level string `json:"level"`
	// Format is one of: text, json.
	Format string `json:"format"`
	// File, when non-empty, sends logs to this path instead of stderr.
	File string `json:"file"`
}

// ConcurrencyConfig bounds worker pools. Zero is never used at runtime; the
// defaults are always positive and validation rejects non-positive values.
type ConcurrencyConfig struct {
	Global int `json:"global"`
	DNS    int `json:"dns"`
	HTTP   int `json:"http"`
	Port   int `json:"port"`
}

// HTTPConfig configures the native HTTP probing client (M7).
type HTTPConfig struct {
	TimeoutSeconds  int     `json:"timeout_seconds"`
	Retries         int     `json:"retries"`
	RateLimitPerSec float64 `json:"rate_limit_per_sec"`
	UserAgent       string  `json:"user_agent"`
	FollowRedirects bool    `json:"follow_redirects"`
	MaxRedirects    int     `json:"max_redirects"`
}

// DNSConfig configures the native resolver (M4).
type DNSConfig struct {
	Resolvers       []string `json:"resolvers"`
	TimeoutSeconds  int      `json:"timeout_seconds"`
	Retries         int      `json:"retries"`
	Protocol        string   `json:"protocol"` // udp, tcp, doh, dot
	RateLimitPerSec float64  `json:"rate_limit_per_sec"`
}

// ScopeConfig holds the raw scope inputs consumed by the scope engine (M3).
type ScopeConfig struct {
	Include   []string `json:"include"`
	Exclude   []string `json:"exclude"`
	Wildcards []string `json:"wildcards"`
}

// valid enumerations, kept here so config and cli agree on the accepted values.
var (
	validLogLevels  = map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	validLogFormats = map[string]bool{"text": true, "json": true}
	validProtocols  = map[string]bool{"udp": true, "tcp": true, "doh": true, "dot": true}
)

// Default returns a fully-populated, valid configuration. Default().Validate()
// always returns nil; this is asserted by tests.
func Default() *Config {
	return &Config{
		Project:   "default",
		DataDir:   "./data",
		OutputDir: "./output",
		Logging: LoggingConfig{
			Level:  "info",
			Format: "text",
			File:   "",
		},
		Concurrency: ConcurrencyConfig{
			Global: 50,
			DNS:    25,
			HTTP:   25,
			Port:   100,
		},
		HTTP: HTTPConfig{
			TimeoutSeconds:  10,
			Retries:         2,
			RateLimitPerSec: 20,
			UserAgent:       "nett/0.1 (+https://github.com/alieddine/nett)",
			FollowRedirects: true,
			MaxRedirects:    10,
		},
		DNS: DNSConfig{
			Resolvers:       []string{"1.1.1.1:53", "8.8.8.8:53"},
			TimeoutSeconds:  5,
			Retries:         2,
			Protocol:        "udp",
			RateLimitPerSec: 50,
		},
		Scope: ScopeConfig{
			Include:   nil,
			Exclude:   nil,
			Wildcards: nil,
		},
		Modules: map[string]bool{},
	}
}

// Load returns a configuration assembled from defaults overlaid with the JSON
// file at path (if path is non-empty), then overlaid with NETT_* environment
// variables. Flag overrides are applied by the caller afterward. The result is
// validated before return.
//
// A path that does not exist is an error; an empty path means "defaults + env".
func Load(path string) (*Config, error) {
	cfg := Default()
	if path != "" {
		if err := cfg.mergeFile(path); err != nil {
			return nil, err
		}
	}
	if err := cfg.ApplyEnv(os.Getenv); err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// mergeFile decodes the JSON file at path into the config. Only keys present in
// the file override the defaults, because we decode into the already-populated
// struct. Unknown fields are rejected so typos are caught early.
func (c *Config) mergeFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("config: open %q: %w", path, err)
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	if err := dec.Decode(c); err != nil {
		return fmt.Errorf("config: parse %q: %w", path, err)
	}
	return nil
}

// getenv matches os.Getenv's signature so tests can inject a fake environment.
type getenv func(string) string

// ApplyEnv overlays NETT_* environment variables onto the config. It returns a
// descriptive error if a numeric or boolean variable cannot be parsed.
func (c *Config) ApplyEnv(env getenv) error {
	setStr := func(key string, dst *string) {
		if v := env(key); v != "" {
			*dst = v
		}
	}
	setInt := func(key string, dst *int) error {
		v := env(key)
		if v == "" {
			return nil
		}
		n, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("config: %s=%q: not an integer", key, v)
		}
		*dst = n
		return nil
	}
	setFloat := func(key string, dst *float64) error {
		v := env(key)
		if v == "" {
			return nil
		}
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return fmt.Errorf("config: %s=%q: not a number", key, v)
		}
		*dst = f
		return nil
	}
	setBool := func(key string, dst *bool) error {
		v := env(key)
		if v == "" {
			return nil
		}
		b, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("config: %s=%q: not a boolean", key, v)
		}
		*dst = b
		return nil
	}
	setCSV := func(key string, dst *[]string) {
		if v := env(key); v != "" {
			*dst = splitCSV(v)
		}
	}

	setStr("NETT_PROJECT", &c.Project)
	setStr("NETT_DATA_DIR", &c.DataDir)
	setStr("NETT_OUTPUT_DIR", &c.OutputDir)

	setStr("NETT_LOG_LEVEL", &c.Logging.Level)
	setStr("NETT_LOG_FORMAT", &c.Logging.Format)
	setStr("NETT_LOG_FILE", &c.Logging.File)

	if err := setInt("NETT_CONCURRENCY", &c.Concurrency.Global); err != nil {
		return err
	}
	if err := setInt("NETT_HTTP_TIMEOUT", &c.HTTP.TimeoutSeconds); err != nil {
		return err
	}
	if err := setInt("NETT_HTTP_RETRIES", &c.HTTP.Retries); err != nil {
		return err
	}
	if err := setFloat("NETT_HTTP_RATELIMIT", &c.HTTP.RateLimitPerSec); err != nil {
		return err
	}
	setStr("NETT_HTTP_USERAGENT", &c.HTTP.UserAgent)
	if err := setBool("NETT_HTTP_FOLLOW_REDIRECTS", &c.HTTP.FollowRedirects); err != nil {
		return err
	}

	setCSV("NETT_DNS_RESOLVERS", &c.DNS.Resolvers)
	if err := setInt("NETT_DNS_TIMEOUT", &c.DNS.TimeoutSeconds); err != nil {
		return err
	}
	setStr("NETT_DNS_PROTOCOL", &c.DNS.Protocol)

	setCSV("NETT_SCOPE_INCLUDE", &c.Scope.Include)
	setCSV("NETT_SCOPE_EXCLUDE", &c.Scope.Exclude)
	setCSV("NETT_SCOPE_WILDCARDS", &c.Scope.Wildcards)

	return nil
}

// Validate checks that the configuration is internally consistent and safe to
// run with. It accumulates all problems into a single error so the user sees
// every issue at once.
func (c *Config) Validate() error {
	var problems []string

	if strings.TrimSpace(c.Project) == "" {
		problems = append(problems, "project must not be empty")
	}
	if strings.TrimSpace(c.DataDir) == "" {
		problems = append(problems, "data_dir must not be empty")
	}

	if !validLogLevels[c.Logging.Level] {
		problems = append(problems, fmt.Sprintf("logging.level %q invalid (want debug|info|warn|error)", c.Logging.Level))
	}
	if !validLogFormats[c.Logging.Format] {
		problems = append(problems, fmt.Sprintf("logging.format %q invalid (want text|json)", c.Logging.Format))
	}

	checkPositive := func(name string, v int) {
		if v <= 0 {
			problems = append(problems, fmt.Sprintf("%s must be > 0 (got %d)", name, v))
		}
	}
	checkPositive("concurrency.global", c.Concurrency.Global)
	checkPositive("concurrency.dns", c.Concurrency.DNS)
	checkPositive("concurrency.http", c.Concurrency.HTTP)
	checkPositive("concurrency.port", c.Concurrency.Port)
	checkPositive("http.timeout_seconds", c.HTTP.TimeoutSeconds)
	checkPositive("dns.timeout_seconds", c.DNS.TimeoutSeconds)

	if c.HTTP.Retries < 0 {
		problems = append(problems, fmt.Sprintf("http.retries must be >= 0 (got %d)", c.HTTP.Retries))
	}
	if c.DNS.Retries < 0 {
		problems = append(problems, fmt.Sprintf("dns.retries must be >= 0 (got %d)", c.DNS.Retries))
	}
	if c.HTTP.RateLimitPerSec < 0 {
		problems = append(problems, fmt.Sprintf("http.rate_limit_per_sec must be >= 0 (got %g)", c.HTTP.RateLimitPerSec))
	}
	if c.DNS.RateLimitPerSec < 0 {
		problems = append(problems, fmt.Sprintf("dns.rate_limit_per_sec must be >= 0 (got %g)", c.DNS.RateLimitPerSec))
	}
	if !validProtocols[c.DNS.Protocol] {
		problems = append(problems, fmt.Sprintf("dns.protocol %q invalid (want udp|tcp|doh|dot)", c.DNS.Protocol))
	}
	if len(c.DNS.Resolvers) == 0 {
		problems = append(problems, "dns.resolvers must not be empty")
	}

	if len(problems) > 0 {
		return fmt.Errorf("invalid configuration:\n  - %s", strings.Join(problems, "\n  - "))
	}
	return nil
}

// JSON returns the configuration as indented JSON, used by `nett config`.
func (c *Config) JSON() (string, error) {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// splitCSV splits a comma-separated list, trimming spaces and dropping empties.
func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
