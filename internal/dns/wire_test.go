package dns

import (
	"reflect"
	"testing"
)

func TestQueryRoundTrip(t *testing.T) {
	q := NewQuery(0x1234, "example.com", TypeA)
	buf, err := q.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got, err := Unmarshal(buf)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.ID != 0x1234 {
		t.Errorf("ID = %#x, want 0x1234", got.ID)
	}
	if !got.RecursionDesired {
		t.Error("RecursionDesired should round-trip true")
	}
	if len(got.Questions) != 1 || got.Questions[0].Name != "example.com" || got.Questions[0].Type != TypeA {
		t.Errorf("Questions = %+v", got.Questions)
	}
}

func TestRecordRoundTrip(t *testing.T) {
	cases := []Record{
		{Name: "example.com", Type: TypeA, TTL: 300, A: "93.184.216.34"},
		{Name: "example.com", Type: TypeAAAA, TTL: 300, AAAA: "2606:2800:220:1:248:1893:25c8:1946"},
		{Name: "example.com", Type: TypeCNAME, TTL: 300, CNAME: "target.example.net"},
		{Name: "example.com", Type: TypeNS, TTL: 300, NS: "ns1.example.com"},
		{Name: "4.3.2.1.in-addr.arpa", Type: TypePTR, TTL: 300, PTR: "host.example.com"},
		{Name: "example.com", Type: TypeMX, TTL: 300, MX: &MX{Preference: 10, Exchange: "mail.example.com"}},
		{Name: "example.com", Type: TypeTXT, TTL: 300, TXT: []string{"v=spf1 -all", "second chunk"}},
		{Name: "_sip._tcp.example.com", Type: TypeSRV, TTL: 300, SRV: &SRV{Priority: 1, Weight: 2, Port: 5060, Target: "sip.example.com"}},
		{Name: "example.com", Type: TypeCAA, TTL: 300, CAA: &CAA{Flag: 0, Tag: "issue", Value: "letsencrypt.org"}},
		{Name: "example.com", Type: TypeSOA, TTL: 300, SOA: &SOA{
			MName: "ns1.example.com", RName: "hostmaster.example.com",
			Serial: 2024010100, Refresh: 3600, Retry: 900, Expire: 604800, Minimum: 300,
		}},
	}

	for _, rr := range cases {
		t.Run(rr.Type.String(), func(t *testing.T) {
			m := &Message{Response: true, Questions: []Question{{Name: rr.Name, Type: rr.Type}}, Answers: []Record{rr}}
			buf, err := m.Marshal()
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			got, err := Unmarshal(buf)
			if err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if len(got.Answers) != 1 {
				t.Fatalf("Answers = %+v, want 1 record", got.Answers)
			}
			if !reflect.DeepEqual(got.Answers[0], rr) {
				t.Errorf("round-trip mismatch:\n got  %+v\n want %+v", got.Answers[0], rr)
			}
		})
	}
}

func TestUnmarshalRejectsShortMessage(t *testing.T) {
	if _, err := Unmarshal([]byte{1, 2, 3}); err == nil {
		t.Error("Unmarshal should reject a message shorter than the header")
	}
}

// buildHeader returns a 12-byte DNS header with the given section counts.
func buildHeader(id uint16, qd, an, ns, ar int) []byte {
	buf := make([]byte, 12)
	buf[0], buf[1] = byte(id>>8), byte(id)
	buf[2] = 0x81 // QR=1, RD=1
	buf[3] = 0x80 // RA=1
	putU16 := func(off int, v int) { buf[off] = byte(v >> 8); buf[off+1] = byte(v) }
	putU16(4, qd)
	putU16(6, an)
	putU16(8, ns)
	putU16(10, ar)
	return buf
}

// TestNameCompressionPointer hand-builds a response where the second
// answer's owner name is a compression pointer back to the first answer's
// name, and the first answer's name is itself a pointer to the question —
// exercising the exact shape real-world resolvers send and that Go's own
// encoder never produces, since Marshal does not compress on write.
func TestNameCompressionPointer(t *testing.T) {
	buf := buildHeader(1, 1, 2, 0, 0)
	// Question: "example.com" A, starts at offset 12.
	qNameOff := len(buf)
	buf = append(buf, encodeNameMust(t, "example.com")...)
	buf = append(buf, 0, 1) // TYPE A
	buf = append(buf, 0, 1) // CLASS IN

	// Answer 1: name = pointer to question name.
	ans1Off := len(buf)
	buf = append(buf, 0xC0, byte(qNameOff))
	buf = append(buf, 0, 1) // TYPE A
	buf = append(buf, 0, 1) // CLASS IN
	buf = appendUint32(buf, 300)
	buf = appendUint16(buf, 4)
	buf = append(buf, 93, 184, 216, 34)

	// Answer 2: name = pointer to answer 1's name (which is itself a
	// pointer) — verifies chained pointer-following.
	buf = append(buf, 0xC0, byte(ans1Off))
	buf = append(buf, 0, 1)
	buf = append(buf, 0, 1)
	buf = appendUint32(buf, 300)
	buf = appendUint16(buf, 4)
	buf = append(buf, 93, 184, 216, 35)

	m, err := Unmarshal(buf)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(m.Answers) != 2 {
		t.Fatalf("Answers = %+v, want 2", m.Answers)
	}
	for i, want := range []string{"93.184.216.34", "93.184.216.35"} {
		if m.Answers[i].Name != "example.com" {
			t.Errorf("Answers[%d].Name = %q, want example.com (via pointer)", i, m.Answers[i].Name)
		}
		if m.Answers[i].A != want {
			t.Errorf("Answers[%d].A = %q, want %q", i, m.Answers[i].A, want)
		}
	}
}

func TestNameCompressionLoopIsRejected(t *testing.T) {
	buf := buildHeader(1, 0, 1, 0, 0)
	// A record whose name is a pointer to itself: infinite loop if
	// unguarded. readName must return an error instead of hanging.
	selfOff := len(buf)
	buf = append(buf, 0xC0, byte(selfOff))
	buf = append(buf, 0, 1)
	buf = append(buf, 0, 1)
	buf = appendUint32(buf, 300)
	buf = appendUint16(buf, 4)
	buf = append(buf, 1, 2, 3, 4)

	if _, err := Unmarshal(buf); err == nil {
		t.Error("Unmarshal should reject a self-referential compression pointer")
	}
}

func encodeNameMust(t *testing.T, name string) []byte {
	t.Helper()
	b, err := encodeName(name)
	if err != nil {
		t.Fatalf("encodeName(%q): %v", name, err)
	}
	return b
}
