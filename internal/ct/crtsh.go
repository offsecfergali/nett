// Package ct implements Certificate Transparency log discovery: querying
// public CT log aggregators for certificates issued to a domain, and
// extracting the hostnames they cover. This is passive discovery — it never
// contacts the target itself, only a public third-party CT aggregator.
package ct

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// Provider is a source of Certificate-Transparency-derived hostnames.
type Provider interface {
	Name() string
	Query(ctx context.Context, domain string) ([]string, error)
}

// CrtSh queries crt.sh (https://crt.sh), the most widely used public CT log
// aggregator, via its JSON output mode.
type CrtSh struct {
	baseURL string
	client  *http.Client
}

// NewCrtSh returns a CrtSh provider using client (a *http.Client with a
// sensible timeout; New from internal/httpclient in later milestones, a
// plain &http.Client{Timeout: ...} for now).
func NewCrtSh(client *http.Client) *CrtSh {
	return newCrtSh("https://crt.sh", client)
}

func newCrtSh(baseURL string, client *http.Client) *CrtSh {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &CrtSh{baseURL: baseURL, client: client}
}

// Name identifies this provider in provenance records.
func (c *CrtSh) Name() string { return "crt.sh" }

// crtShEntry is the shape of one row in crt.sh's JSON output. name_value can
// contain multiple newline-separated names when a certificate covers several
// SANs; common_name is the certificate's subject CN.
type crtShEntry struct {
	CommonName string `json:"common_name"`
	NameValue  string `json:"name_value"`
}

// Query returns every distinct, normalized hostname crt.sh has observed on a
// certificate covering domain or any of its subdomains. Wildcard names
// ("*.example.com") are reported with the leading "*." stripped, since the
// wildcard marker itself is not a resolvable hostname.
func (c *CrtSh) Query(ctx context.Context, domain string) ([]string, error) {
	reqURL := fmt.Sprintf("%s/?q=%s&output=json", c.baseURL, url.QueryEscape("%."+domain))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("ct: build crt.sh request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ct: crt.sh request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ct: crt.sh returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, fmt.Errorf("ct: read crt.sh response: %w", err)
	}
	// An empty result set is returned by crt.sh as an empty body, not `[]`.
	if len(strings.TrimSpace(string(body))) == 0 {
		return nil, nil
	}

	var entries []crtShEntry
	if err := json.Unmarshal(body, &entries); err != nil {
		return nil, fmt.Errorf("ct: parse crt.sh response: %w", err)
	}

	seen := make(map[string]struct{})
	for _, e := range entries {
		for _, raw := range strings.Split(e.NameValue, "\n") {
			if h := normalizeHostname(raw); h != "" {
				seen[h] = struct{}{}
			}
		}
		if h := normalizeHostname(e.CommonName); h != "" {
			seen[h] = struct{}{}
		}
	}

	out := make([]string, 0, len(seen))
	for h := range seen {
		out = append(out, h)
	}
	sort.Strings(out)
	return out, nil
}

func normalizeHostname(raw string) string {
	h := strings.ToLower(strings.TrimSpace(raw))
	h = strings.TrimSuffix(h, ".")
	h = strings.TrimPrefix(h, "*.")
	if h == "" || strings.ContainsAny(h, " \t\"'@/\\") {
		return "" // not a plausible hostname (e.g. an email in a CN, a blank)
	}
	return h
}
