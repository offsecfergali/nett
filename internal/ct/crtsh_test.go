package ct

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
)

func newTestServer(t *testing.T, body string, status int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("q"); got != "%.example.com" {
			t.Errorf("query = %q, want %%.example.com", got)
		}
		if r.URL.Query().Get("output") != "json" {
			t.Error("expected output=json in query")
		}
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestCrtShParsesMultiSANEntries(t *testing.T) {
	body := `[
		{"common_name":"api.example.com","name_value":"api.example.com\nwww.api.example.com"},
		{"common_name":"example.com","name_value":"*.example.com\nexample.com"}
	]`
	srv := newTestServer(t, body, http.StatusOK)
	c := newCrtSh(srv.URL, srv.Client())

	got, err := c.Query(context.Background(), "example.com")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	sort.Strings(got)
	want := []string{"api.example.com", "example.com", "www.api.example.com"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got %v, want %v", got, want)
			break
		}
	}
}

func TestCrtShEmptyResponse(t *testing.T) {
	srv := newTestServer(t, "", http.StatusOK)
	c := newCrtSh(srv.URL, srv.Client())
	got, err := c.Query(context.Background(), "example.com")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %v, want empty", got)
	}
}

func TestCrtShRejectsNonEmailCN(t *testing.T) {
	// A CN containing an email address or spaces must not leak through as a
	// bogus "hostname".
	body := `[{"common_name":"Acme Corp Root CA","name_value":"admin@example.com\nvalid.example.com"}]`
	srv := newTestServer(t, body, http.StatusOK)
	c := newCrtSh(srv.URL, srv.Client())
	got, err := c.Query(context.Background(), "example.com")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(got) != 1 || got[0] != "valid.example.com" {
		t.Errorf("got %v, want only [valid.example.com]", got)
	}
}

func TestCrtShHTTPError(t *testing.T) {
	srv := newTestServer(t, "internal error", http.StatusInternalServerError)
	c := newCrtSh(srv.URL, srv.Client())
	if _, err := c.Query(context.Background(), "example.com"); err == nil {
		t.Error("Query should fail on a non-200 response")
	}
}

func TestCrtShMalformedJSON(t *testing.T) {
	srv := newTestServer(t, "{not json", http.StatusOK)
	c := newCrtSh(srv.URL, srv.Client())
	if _, err := c.Query(context.Background(), "example.com"); err == nil {
		t.Error("Query should fail on malformed JSON")
	}
}

func TestNormalizeHostname(t *testing.T) {
	cases := map[string]string{
		"Example.COM.":      "example.com",
		"*.example.com":     "example.com",
		"  spaced.com  ":    "spaced.com",
		"admin@example.com": "",
		"":                  "",
		"has space.com":     "",
	}
	for in, want := range cases {
		if got := normalizeHostname(in); got != want {
			t.Errorf("normalizeHostname(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestName(t *testing.T) {
	c := NewCrtSh(nil)
	if c.Name() != "crt.sh" {
		t.Errorf("Name() = %q, want crt.sh", c.Name())
	}
}
