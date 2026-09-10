package module

// DefaultRegistry returns a registry populated with the nett module catalog.
//
// The catalog is the single source of truth for `nett modules` and
// `nett capabilities`. Modules delivered by the current milestone are marked
// StatusImplemented and contribute real capabilities; modules from future
// milestones are marked StatusPlanned so users can see the roadmap without the
// tool ever claiming a capability it does not have.
//
// As each milestone lands, its descriptor's Status flips to implemented and its
// Capabilities become visible in `nett capabilities`. This function is the
// only place that mapping changes.
func DefaultRegistry() *Registry {
	r := NewRegistry()
	for _, d := range catalog() {
		r.MustRegister(d)
	}
	return r
}

func catalog() []*Descriptor {
	return []*Descriptor{
		// --- Implemented (Milestone 1) ---
		{
			Name:      "core",
			Short:     "CLI, layered configuration, and structured logging",
			Milestone: "M1",
			Status:    StatusImplemented,
			Active:    false,
			Capabilities: []Capability{
				{Group: "CLI", Items: []string{"help", "version", "capabilities", "modules", "config"}},
				{Group: "Config", Items: []string{"defaults", "json-file", "env-overrides", "flag-overrides", "validation"}},
				{Group: "Logging", Items: []string{"levels", "text", "json", "file-output", "component-scoping"}},
			},
		},
		{
			Name:      "store",
			Short:     "SQLite asset-graph store: assets, edges, provenance, events, module runs",
			Milestone: "M2",
			Status:    StatusImplemented,
			Active:    false,
			Capabilities: []Capability{
				{Group: "Store", Items: []string{"projects", "assets", "edges", "provenance", "events", "module-runs", "additive-merge", "event-dedup"}},
			},
		},
		{
			Name:      "scope",
			Short:     "Scope engine (domain/host/IP/CIDR/URL/path, allow/deny, fail-closed)",
			Milestone: "M3",
			Status:    StatusImplemented,
			Active:    false,
			Capabilities: []Capability{
				{Group: "Scope", Items: []string{"domain", "wildcard", "hostname", "ip", "cidr", "url", "path", "allowlist", "denylist", "dynamic-ip-learning"}},
			},
		},

		{
			Name:      "dns",
			Short:     "Native DNS resolver with caching and wildcard detection",
			Milestone: "M4",
			Status:    StatusImplemented,
			Active:    true,
			Capabilities: []Capability{
				{Group: "DNS", Items: []string{"A", "AAAA", "CNAME", "MX", "NS", "TXT", "SRV", "CAA", "PTR", "SOA"}},
				{Group: "DNS-Transport", Items: []string{"UDP", "TCP", "DoH", "DoT"}},
				{Group: "DNS-Features", Items: []string{"positive-cache", "negative-cache", "ttl", "retries", "resolver-rotation", "wildcard-detection"}},
			},
		},

		{
			Name:      "ct",
			Short:     "Certificate Transparency discovery via crt.sh",
			Milestone: "M5",
			Status:    StatusImplemented,
			Active:    true,
			Capabilities: []Capability{
				{Group: "CT", Items: []string{"crt.sh"}},
			},
		},
		{
			Name:      "subdomain",
			Short:     "Passive subdomain discovery: CT + DNS validation + wildcard filtering",
			Milestone: "M5",
			Status:    StatusImplemented,
			Active:    true,
			Capabilities: []Capability{
				{Group: "Subdomain", Items: []string{"ct-merge", "scope-filter", "dns-validation", "wildcard-flagging"}},
			},
		},
		{
			Name:      "permute",
			Short:     "DNS brute force / permutation engine for candidate hostnames",
			Milestone: "M7",
			Status:    StatusImplemented,
			Active:    true,
			Capabilities: []Capability{
				{Group: "Permute", Items: []string{"wordlist", "common-affix-permutations", "wildcard-filtering"}},
			},
		},
		{
			Name:      "portscan",
			Short:     "Native TCP connect scanner (explicit activation only)",
			Milestone: "M9",
			Status:    StatusImplemented,
			Active:    true,
			Capabilities: []Capability{
				{Group: "Portscan", Items: []string{"single-port", "port-range", "port-list", "common-ports", "all-ports", "rate-limited", "bounded-concurrency"}},
			},
		},

		// --- Planned (future milestones) ---
		{
			Name: "asn", Short: "ASN/IP intelligence and netblock relationships",
			Milestone: "M6", Status: StatusPlanned, Active: true,
		},
		{
			Name: "http", Short: "Native HTTP probing engine and reusable client",
			Milestone: "M7", Status: StatusPlanned, Active: true,
			Capabilities: []Capability{
				{Group: "HTTP", Items: []string{"probing", "redirects", "headers", "cookies", "tls", "timing", "http-version"}},
			},
		},
		{
			Name: "tls", Short: "Native TLS inspection (cert/SAN/cipher/ALPN)",
			Milestone: "M8", Status: StatusPlanned, Active: true,
		},
		{
			Name: "servicefp", Short: "Service/banner fingerprinting via safe probes",
			Milestone: "M10", Status: StatusPlanned, Active: true,
		},
		{
			Name: "fingerprint", Short: "HTTP technology fingerprinting (external definitions)",
			Milestone: "M10", Status: StatusPlanned, Active: false,
		},
		{
			Name: "crawler", Short: "Native concurrent web crawler",
			Milestone: "M11", Status: StatusPlanned, Active: true,
			Capabilities: []Capability{
				{Group: "Web", Items: []string{"crawler", "forms", "robots", "sitemap"}},
			},
		},
		{
			Name: "jsintel", Short: "JavaScript intelligence engine",
			Milestone: "M12", Status: StatusPlanned, Active: true,
		},
		{
			Name: "urldisc", Short: "Historical URL providers, robots and sitemap intelligence",
			Milestone: "M13", Status: StatusPlanned, Active: true,
		},
		{
			Name: "content", Short: "Optional content discovery (bring-your-own wordlist)",
			Milestone: "M13", Status: StatusPlanned, Active: true,
		},
		{
			Name: "apidisc", Short: "REST/OpenAPI/Swagger discovery and parsing",
			Milestone: "M14", Status: StatusPlanned, Active: true,
		},
		{
			Name: "graphql", Short: "GraphQL reconnaissance",
			Milestone: "M15", Status: StatusPlanned, Active: true,
		},
		{
			Name: "param", Short: "Parameter discovery across URLs/forms/JS/OpenAPI",
			Milestone: "M15", Status: StatusPlanned, Active: false,
		},
		{
			Name: "repo", Short: "Public repository intelligence",
			Milestone: "M16", Status: StatusPlanned, Active: true,
		},
		{
			Name: "cloud", Short: "Cloud/CDN attribution from DNS/TLS/HTTP signals",
			Milestone: "M17", Status: StatusPlanned, Active: false,
		},
		{
			Name: "classify", Short: "Asset classification engine",
			Milestone: "M18", Status: StatusPlanned, Active: false,
		},
		{
			Name: "score", Short: "Deterministic asset scoring and prioritization",
			Milestone: "M19", Status: StatusPlanned, Active: false,
		},
		{
			Name: "graph", Short: "Relationship-graph query layer",
			Milestone: "M20", Status: StatusPlanned, Active: false,
		},
		{
			Name: "event", Short: "Event bus and pivot scheduler with loop guards",
			Milestone: "M21", Status: StatusPlanned, Active: false,
		},
		{
			Name: "diff", Short: "Differential reconnaissance between project snapshots",
			Milestone: "M22", Status: StatusPlanned, Active: false,
		},
		{
			Name: "resume", Short: "Resume/incremental engine over persisted module state",
			Milestone: "M23", Status: StatusPlanned, Active: false,
		},
	}
}
