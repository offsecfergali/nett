package dns

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alieddine/nett/internal/config"
)

// startFakeServer runs a minimal UDP DNS server on loopback driven entirely
// by handler, so tests never touch the public Internet. It returns the
// server's address and stops the goroutine on test cleanup.
func startFakeServer(t *testing.T, handler func(q *Message) *Message) string {
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
			q, err := Unmarshal(buf[:n])
			if err != nil {
				continue
			}
			resp := handler(q)
			if resp == nil {
				continue
			}
			resp.ID = q.ID
			respBuf, err := resp.Marshal()
			if err != nil {
				continue
			}
			conn.WriteToUDP(respBuf, addr)
		}
	}()
	return conn.LocalAddr().String()
}

// deadUDPAddr returns an address nothing is listening on, for exercising the
// retry/failover path deterministically and quickly.
func deadUDPAddr(t *testing.T) string {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatalf("ListenUDP: %v", err)
	}
	addr := conn.LocalAddr().String()
	conn.Close()
	return addr
}

func testDNSConfig(resolvers ...string) config.DNSConfig {
	return config.DNSConfig{
		Resolvers:       resolvers,
		TimeoutSeconds:  1,
		Retries:         1,
		Protocol:        "udp",
		RateLimitPerSec: 0,
	}
}

func answerA(q *Message, ip string, ttl uint32) *Message {
	return &Message{
		Response: true, RecursionAvailable: true,
		Questions: q.Questions,
		Answers:   []Record{{Name: q.Questions[0].Name, Type: TypeA, TTL: ttl, A: ip}},
	}
}

func nxdomain(q *Message) *Message {
	return &Message{Response: true, Rcode: RcodeNameError, Questions: q.Questions}
}

func TestLookupA_Basic(t *testing.T) {
	addr := startFakeServer(t, func(q *Message) *Message {
		if q.Questions[0].Name == "example.com" && q.Questions[0].Type == TypeA {
			return answerA(q, "93.184.216.34", 300)
		}
		return nxdomain(q)
	})
	r, err := New(testDNSConfig(addr))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ips, err := r.LookupA(context.Background(), "example.com")
	if err != nil {
		t.Fatalf("LookupA: %v", err)
	}
	if len(ips) != 1 || ips[0].String() != "93.184.216.34" {
		t.Errorf("LookupA = %v, want [93.184.216.34]", ips)
	}
}

func TestLookupA_NXDOMAIN(t *testing.T) {
	addr := startFakeServer(t, func(q *Message) *Message { return nxdomain(q) })
	r, err := New(testDNSConfig(addr))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ips, err := r.LookupA(context.Background(), "nosuchhost.example.com")
	if err != nil {
		t.Fatalf("LookupA should not error on NXDOMAIN: %v", err)
	}
	if len(ips) != 0 {
		t.Errorf("LookupA(NXDOMAIN) = %v, want empty", ips)
	}
}

func TestLookupA_ChasesCNAME(t *testing.T) {
	addr := startFakeServer(t, func(q *Message) *Message {
		name, qtype := q.Questions[0].Name, q.Questions[0].Type
		switch {
		case name == "alias.example.com" && qtype == TypeA:
			return &Message{Response: true, Questions: q.Questions} // NOERROR, no answers
		case name == "alias.example.com" && qtype == TypeCNAME:
			return &Message{Response: true, Questions: q.Questions,
				Answers: []Record{{Name: name, Type: TypeCNAME, TTL: 300, CNAME: "target.example.com"}}}
		case name == "target.example.com" && qtype == TypeA:
			return answerA(q, "203.0.113.9", 300)
		}
		return nxdomain(q)
	})
	r, err := New(testDNSConfig(addr))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ips, err := r.LookupA(context.Background(), "alias.example.com")
	if err != nil {
		t.Fatalf("LookupA: %v", err)
	}
	if len(ips) != 1 || ips[0].String() != "203.0.113.9" {
		t.Errorf("LookupA (via CNAME) = %v, want [203.0.113.9]", ips)
	}
}

func TestQueryIsCached(t *testing.T) {
	var calls int32
	addr := startFakeServer(t, func(q *Message) *Message {
		atomic.AddInt32(&calls, 1)
		return answerA(q, "93.184.216.34", 300)
	})
	r, err := New(testDNSConfig(addr))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	if _, err := r.LookupA(ctx, "example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.LookupA(ctx, "example.com"); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("server received %d queries, want 1 (second lookup should hit cache)", got)
	}
}

func TestQueryRetriesAcrossResolvers(t *testing.T) {
	dead := deadUDPAddr(t)
	live := startFakeServer(t, func(q *Message) *Message {
		return answerA(q, "93.184.216.34", 300)
	})
	r, err := New(testDNSConfig(dead, live))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ips, err := r.LookupA(context.Background(), "example.com")
	if err != nil {
		t.Fatalf("LookupA should succeed via the second resolver: %v", err)
	}
	if len(ips) != 1 {
		t.Errorf("LookupA = %v, want one address", ips)
	}
}

func TestQueryFailsAfterExhaustingResolvers(t *testing.T) {
	dead1, dead2 := deadUDPAddr(t), deadUDPAddr(t)
	cfg := testDNSConfig(dead1, dead2)
	cfg.Retries = 0
	r, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := r.LookupA(context.Background(), "example.com"); err == nil {
		t.Error("LookupA should fail when every resolver is unreachable")
	}
}

func TestLookupPTR(t *testing.T) {
	addr := startFakeServer(t, func(q *Message) *Message {
		if q.Questions[0].Name == "34.216.184.93.in-addr.arpa" && q.Questions[0].Type == TypePTR {
			return &Message{Response: true, Questions: q.Questions,
				Answers: []Record{{Name: q.Questions[0].Name, Type: TypePTR, TTL: 300, PTR: "example.com"}}}
		}
		return nxdomain(q)
	})
	r, err := New(testDNSConfig(addr))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	names, err := r.LookupPTR(context.Background(), net.ParseIP("93.184.216.34"))
	if err != nil {
		t.Fatalf("LookupPTR: %v", err)
	}
	if len(names) != 1 || names[0] != "example.com" {
		t.Errorf("LookupPTR = %v, want [example.com]", names)
	}
}

func TestLookupTXTAndMX(t *testing.T) {
	addr := startFakeServer(t, func(q *Message) *Message {
		switch q.Questions[0].Type {
		case TypeTXT:
			return &Message{Response: true, Questions: q.Questions,
				Answers: []Record{{Name: q.Questions[0].Name, Type: TypeTXT, TTL: 300, TXT: []string{"v=spf1 -all"}}}}
		case TypeMX:
			return &Message{Response: true, Questions: q.Questions,
				Answers: []Record{{Name: q.Questions[0].Name, Type: TypeMX, TTL: 300, MX: &MX{Preference: 10, Exchange: "mail.example.com"}}}}
		}
		return nxdomain(q)
	})
	r, err := New(testDNSConfig(addr))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	txt, err := r.LookupTXT(context.Background(), "example.com")
	if err != nil || len(txt) != 1 || txt[0] != "v=spf1 -all" {
		t.Errorf("LookupTXT = %v, %v", txt, err)
	}
	mx, err := r.LookupMX(context.Background(), "example.com")
	if err != nil || len(mx) != 1 || mx[0].Exchange != "mail.example.com" {
		t.Errorf("LookupMX = %v, %v", mx, err)
	}
}

func TestNewRejectsNoResolvers(t *testing.T) {
	if _, err := New(config.DNSConfig{Protocol: "udp"}); err == nil {
		t.Error("New should reject a config with no resolvers")
	}
}

func TestNewRejectsUnknownProtocol(t *testing.T) {
	if _, err := New(config.DNSConfig{Resolvers: []string{"1.1.1.1:53"}, Protocol: "carrier-pigeon"}); err == nil {
		t.Error("New should reject an unknown protocol")
	}
}

func TestReverseAddrIPv4(t *testing.T) {
	got, err := reverseAddr(net.ParseIP("93.184.216.34"))
	if err != nil {
		t.Fatal(err)
	}
	want := "34.216.184.93.in-addr.arpa"
	if got != want {
		t.Errorf("reverseAddr = %q, want %q", got, want)
	}
}

func TestDetectWildcardPresent(t *testing.T) {
	addr := startFakeServer(t, func(q *Message) *Message {
		name := q.Questions[0].Name
		if len(name) >= 48 { // random probe labels are 48 hex chars
			return answerA(q, "198.51.100.7", 300)
		}
		return nxdomain(q)
	})
	r, err := New(testDNSConfig(addr))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	w, err := DetectWildcard(context.Background(), r, "example.com")
	if err != nil {
		t.Fatalf("DetectWildcard: %v", err)
	}
	if !w.Detected {
		t.Fatal("expected wildcard to be detected")
	}
	if !w.Is([]net.IP{net.ParseIP("198.51.100.7")}) {
		t.Error("Is() should match the wildcard's own IP")
	}
	if w.Is([]net.IP{net.ParseIP("203.0.113.1")}) {
		t.Error("Is() should not match an unrelated IP")
	}
}

func TestDetectWildcardAbsent(t *testing.T) {
	addr := startFakeServer(t, func(q *Message) *Message { return nxdomain(q) })
	r, err := New(testDNSConfig(addr))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	w, err := DetectWildcard(context.Background(), r, "example.com")
	if err != nil {
		t.Fatalf("DetectWildcard: %v", err)
	}
	if w.Detected {
		t.Error("expected no wildcard to be detected")
	}
	if w.Is([]net.IP{net.ParseIP("198.51.100.7")}) {
		t.Error("Is() on a not-detected result should always be false")
	}
}

func TestNegativeCacheUsesSOAMinimum(t *testing.T) {
	var calls int32
	addr := startFakeServer(t, func(q *Message) *Message {
		atomic.AddInt32(&calls, 1)
		return &Message{
			Response: true, Rcode: RcodeNameError, Questions: q.Questions,
			Authority: []Record{{Name: "example.com", Type: TypeSOA, TTL: 300, SOA: &SOA{
				MName: "ns1.example.com", RName: "hostmaster.example.com",
				Serial: 1, Refresh: 1, Retry: 1, Expire: 1, Minimum: 60,
			}}},
		}
	})
	r, err := New(testDNSConfig(addr))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	if _, err := r.LookupA(ctx, "nosuchhost.example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.LookupA(ctx, "nosuchhost.example.com"); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("server received %d queries, want 1 (negative result should be cached)", got)
	}
}

func TestResolverRespectsContextCancellation(t *testing.T) {
	addr := deadUDPAddr(t)
	cfg := testDNSConfig(addr)
	cfg.TimeoutSeconds = 5
	cfg.Retries = 5
	r, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err = r.LookupA(ctx, "example.com")
	if err == nil {
		t.Fatal("expected an error from a cancelled context")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("query took %v after context cancellation, want it to abort quickly", elapsed)
	}
}
