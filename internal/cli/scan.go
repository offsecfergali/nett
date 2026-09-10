package cli

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/offsecfergali/nett/internal/ct"
	"github.com/offsecfergali/nett/internal/dns"
	"github.com/offsecfergali/nett/internal/model"
	"github.com/offsecfergali/nett/internal/permute"
	"github.com/offsecfergali/nett/internal/portscan"
	"github.com/offsecfergali/nett/internal/scope"
	"github.com/offsecfergali/nett/internal/store"
	"github.com/offsecfergali/nett/internal/subdomain"
)

// scanFlags holds the parsed form of `nett scan <target> [flags]`. Several
// flags (-tcp, -udp) take an optional trailing value, which the standard
// library's flag package cannot express (it always either requires a value
// or never takes one) — so scan arguments are parsed by hand in
// parseScanArgs rather than with flag.FlagSet.
type scanFlags struct {
	target string

	discover bool // -d

	bruteRequested bool
	bruteWordlist  string // -brute <path>

	tcpRequested bool
	tcpSpec      string // -tcp [spec]; empty spec means CommonPorts

	allPorts bool // -p: every port, 1-65535

	udpRequested bool // accepted for forward-compat but not yet implemented
	fingerprint  bool // -f, not yet implemented
	enumerate    bool // -en, not yet implemented

	dirRequested bool // -dir <wordlist>, not yet implemented
	dirWordlist  string
}

// parseScanArgs hand-parses `nett scan` arguments. It returns a usage error
// (not a panic or silent default) for anything it does not recognize, so a
// typo never silently does nothing.
func parseScanArgs(args []string) (*scanFlags, error) {
	var f scanFlags
	var positional []string

	takesOptionalValue := func(i int) (string, int) {
		if i+1 < len(args) && !looksLikeFlag(args[i+1]) {
			return args[i+1], i + 2
		}
		return "", i + 1
	}
	requireValue := func(name string, i int) (string, int, error) {
		if i+1 >= len(args) {
			return "", 0, fmt.Errorf("%s requires a value", name)
		}
		return args[i+1], i + 2, nil
	}

	i := 0
	for i < len(args) {
		a := args[i]
		switch a {
		case "-d":
			f.discover = true
			i++
		case "-p":
			f.allPorts = true
			i++
		case "-f":
			f.fingerprint = true
			i++
		case "-en":
			f.enumerate = true
			i++
		case "-tcp":
			f.tcpRequested = true
			f.tcpSpec, i = takesOptionalValue(i)
		case "-udp":
			f.udpRequested = true
			_, i = takesOptionalValue(i)
		case "-brute":
			f.bruteRequested = true
			var err error
			f.bruteWordlist, i, err = requireValue("-brute", i)
			if err != nil {
				return nil, err
			}
		case "-dir":
			f.dirRequested = true
			var err error
			f.dirWordlist, i, err = requireValue("-dir", i)
			if err != nil {
				return nil, err
			}
		default:
			if looksLikeFlag(a) {
				return nil, fmt.Errorf("unknown flag %q", a)
			}
			positional = append(positional, a)
			i++
		}
	}

	if len(positional) == 0 {
		return nil, fmt.Errorf("missing target (usage: nett scan <target> [flags])")
	}
	if len(positional) > 1 {
		return nil, fmt.Errorf("expected exactly one target, got %d: %v", len(positional), positional)
	}
	f.target = positional[0]

	for _, unimplemented := range []struct {
		requested bool
		flag      string
		milestone string
	}{
		{f.udpRequested, "-udp", "M9"},
		{f.fingerprint, "-f", "M10"},
		{f.enumerate, "-en", "M21"},
		{f.dirRequested, "-dir", "M13"},
	} {
		if unimplemented.requested {
			return nil, fmt.Errorf("%s is not implemented yet (planned %s; run 'nett modules' to see status)", unimplemented.flag, unimplemented.milestone)
		}
	}
	if !f.discover && !f.bruteRequested && !f.tcpRequested && !f.allPorts {
		return nil, fmt.Errorf("no action requested: pass at least one of -d, -brute <wordlist>, -tcp, -p")
	}
	return &f, nil
}

// looksLikeFlag reports whether s should be parsed as a flag name rather
// than a value: a "-" prefix that isn't itself a bare port range like
// "-1000" (which can't happen validly anyway, since ports never start a spec
// with a bare dash) or a negative number.
func looksLikeFlag(s string) bool {
	if !strings.HasPrefix(s, "-") || s == "-" {
		return false
	}
	return true
}

func runScan(ctx context.Context, app *App, args []string) error {
	f, err := parseScanArgs(args)
	if err != nil {
		return err
	}

	target := subdomain.NormalizeDomain(f.target)
	targetIP := net.ParseIP(target)

	sc, err := scope.New(app.Config.Scope)
	if err != nil {
		return fmt.Errorf("build scope: %w", err)
	}
	if targetIP != nil {
		sc.AddIP(targetIP)
	} else {
		sc.AddDomain(target)
	}

	resolver, err := dns.New(app.Config.DNS)
	if err != nil {
		return fmt.Errorf("build resolver: %w", err)
	}

	if err := os.MkdirAll(app.Config.DataDir, 0o755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	dbPath := filepath.Join(app.Config.DataDir, app.Config.Project+".db")
	st, err := store.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer st.Close()

	cfgJSON, _ := app.Config.JSON()
	if err := st.CreateProject(ctx, app.Config.Project, app.Config.Project, cfgJSON); err != nil {
		return fmt.Errorf("create project: %w", err)
	}

	fmt.Fprintf(app.Out, "Target: %s\n", target)

	if f.discover {
		if err := runDiscover(ctx, app, st, resolver, sc, target); err != nil {
			return fmt.Errorf("-d: %w", err)
		}
	}
	if f.bruteRequested {
		if err := runBrute(ctx, app, st, resolver, sc, target, f.bruteWordlist); err != nil {
			return fmt.Errorf("-brute: %w", err)
		}
	}
	if f.tcpRequested || f.allPorts {
		if err := runPortScan(ctx, app, st, resolver, sc, target, targetIP, f); err != nil {
			return fmt.Errorf("port scan: %w", err)
		}
	}
	return nil
}

func runDiscover(ctx context.Context, app *App, st *store.Store, resolver *dns.Resolver, sc *scope.Scope, target string) error {
	httpClient := &http.Client{Timeout: time.Duration(app.Config.HTTP.TimeoutSeconds) * time.Second}
	providers := []ct.Provider{ct.NewCrtSh(httpClient)}

	findings, provErrs, err := subdomain.Discover(ctx, resolver, sc, providers, target, app.Config.Concurrency.DNS)
	if err != nil {
		return err
	}
	for name, perr := range provErrs {
		app.Log.Component("subdomain").Warn("provider failed", "provider", name, "err", perr)
	}

	fmt.Fprintf(app.Out, "\n[-d] Subdomain discovery for %s (%d found)\n", target, len(findings))
	domainAsset := &model.Asset{ProjectID: app.Config.Project, Type: model.AssetDomain, Key: target, InScope: true, Confidence: 1.0}
	if err := st.UpsertAsset(ctx, domainAsset); err != nil {
		return err
	}

	for _, fnd := range findings {
		status := "no resolution"
		if fnd.Resolved {
			ips := make([]string, len(fnd.IPs))
			for i, ip := range fnd.IPs {
				ips[i] = ip.String()
			}
			status = strings.Join(ips, ", ")
		}
		note := ""
		if fnd.WildcardMatch {
			note = "  (wildcard match)"
		}
		fmt.Fprintf(app.Out, "  %-40s %-24s [%s]%s\n", fnd.Hostname, status, strings.Join(fnd.Sources, ","), note)

		if err := persistHostname(ctx, st, app.Config.Project, domainAsset.ID, fnd.Hostname, fnd.IPs, "subdomain", strings.Join(fnd.Sources, ",")); err != nil {
			return err
		}
	}
	return nil
}

func runBrute(ctx context.Context, app *App, st *store.Store, resolver *dns.Resolver, sc *scope.Scope, target, wordlistPath string) error {
	words, err := permute.LoadWordlist(wordlistPath)
	if err != nil {
		return err
	}
	findings, err := permute.BruteForce(ctx, resolver, sc, target, words, true, app.Config.Concurrency.DNS)
	if err != nil {
		return err
	}

	fmt.Fprintf(app.Out, "\n[-brute] DNS brute force for %s (%d words, %d live)\n", target, len(words), len(findings))
	domainAsset := &model.Asset{ProjectID: app.Config.Project, Type: model.AssetDomain, Key: target, InScope: true, Confidence: 1.0}
	if err := st.UpsertAsset(ctx, domainAsset); err != nil {
		return err
	}

	for _, fnd := range findings {
		ips := make([]string, len(fnd.IPs))
		for i, ip := range fnd.IPs {
			ips[i] = ip.String()
		}
		note := ""
		if fnd.WildcardMatch {
			note = "  (wildcard match)"
		}
		fmt.Fprintf(app.Out, "  %-40s %-24s%s\n", fnd.Hostname, strings.Join(ips, ", "), note)

		if err := persistHostname(ctx, st, app.Config.Project, domainAsset.ID, fnd.Hostname, fnd.IPs, "permute", "brute-force"); err != nil {
			return err
		}
	}
	return nil
}

func runPortScan(ctx context.Context, app *App, st *store.Store, resolver *dns.Resolver, sc *scope.Scope, target string, targetIP net.IP, f *scanFlags) error {
	ip := targetIP
	if ip == nil {
		ips, err := resolver.LookupA(ctx, target)
		if err != nil {
			return fmt.Errorf("resolve %s: %w", target, err)
		}
		if len(ips) == 0 {
			return fmt.Errorf("%s does not resolve to an A record; cannot port-scan it", target)
		}
		ip = ips[0]
		sc.LearnIP(target, ip)
	}
	if !sc.AllowIP(ip) {
		return fmt.Errorf("%s is not in scope for active scanning", ip)
	}

	var ports []int
	var err error
	if f.allPorts {
		ports = portscan.AllPorts()
	} else {
		ports, err = portscan.ParsePorts(f.tcpSpec)
	}
	if err != nil {
		return err
	}

	scanner := portscan.New(portscan.Config{
		Concurrency:     app.Config.Concurrency.Port,
		Timeout:         time.Duration(app.Config.HTTP.TimeoutSeconds) * time.Second,
		RateLimitPerSec: app.Config.HTTP.RateLimitPerSec,
	})
	fmt.Fprintf(app.Out, "\n[-tcp] Scanning %s (%s, %d ports)\n", target, ip, len(ports))

	results := scanner.Scan(ctx, ip, ports)
	openCount := 0
	for _, r := range results {
		if r.State != portscan.StateOpen {
			continue
		}
		openCount++
		fmt.Fprintf(app.Out, "  %d/tcp   open\n", r.Port)
		if err := persistPort(ctx, st, app.Config.Project, ip, r.Port); err != nil {
			return err
		}
	}
	if openCount == 0 {
		fmt.Fprintln(app.Out, "  (no open ports found)")
	}
	return nil
}

// persistHostname records host as a hostname asset, links it to its parent
// domain (HAS_SUBDOMAIN), records provenance, and (if it resolved) links it
// to each IP it resolved to (RESOLVES_TO), also recording each IP as its own
// asset — matching the relationship vocabulary in docs/DATA_MODEL.md §3.
func persistHostname(ctx context.Context, st *store.Store, projectID, domainAssetID, host string, ips []net.IP, module, source string) error {
	hostAsset := &model.Asset{
		ProjectID: projectID, Type: model.AssetHostname, Key: host, InScope: true, Confidence: 1.0,
		Value: map[string]any{"fqdn": host, "resolved": len(ips) > 0},
	}
	if err := st.UpsertAsset(ctx, hostAsset); err != nil {
		return err
	}
	if err := st.AddProvenance(ctx, &model.Provenance{
		ProjectID: projectID, SubjectID: hostAsset.ID, SubjectKind: "asset",
		Module: module, Source: source, Confidence: 1.0,
	}); err != nil {
		return err
	}
	if err := st.UpsertEdge(ctx, &model.Edge{
		ProjectID: projectID, SrcID: domainAssetID, DstID: hostAsset.ID, Rel: model.RelHasSubdomain, Confidence: 1.0,
	}); err != nil {
		return err
	}
	for _, ip := range ips {
		ipAsset := &model.Asset{
			ProjectID: projectID, Type: model.AssetIP, Key: ip.String(), InScope: true, Confidence: 1.0,
			Value: map[string]any{"ip": ip.String()},
		}
		if err := st.UpsertAsset(ctx, ipAsset); err != nil {
			return err
		}
		if err := st.UpsertEdge(ctx, &model.Edge{
			ProjectID: projectID, SrcID: hostAsset.ID, DstID: ipAsset.ID, Rel: model.RelResolvesTo, Confidence: 1.0,
		}); err != nil {
			return err
		}
	}
	return nil
}

// persistPort records an open port as its own asset, keyed by ip:port/tcp so
// it stays stable across scans. It is intentionally not linked to the IP via
// an edge yet: docs/DATA_MODEL.md §3's vocabulary defines SERVICE -> ON_PORT
// -> PORT, not IP -> PORT directly, and service identification does not
// exist until M10 — inventing a relationship not in that vocabulary would
// make the graph harder to reason about later, not easier.
func persistPort(ctx context.Context, st *store.Store, projectID string, ip net.IP, port int) error {
	key := fmt.Sprintf("%s:%d/tcp", ip.String(), port)
	portAsset := &model.Asset{
		ProjectID: projectID, Type: model.AssetPort, Key: key, InScope: true, Confidence: 1.0,
		Value: map[string]any{"ip": ip.String(), "number": float64(port), "transport": "tcp", "state": "open"},
	}
	return st.UpsertAsset(ctx, portAsset)
}
