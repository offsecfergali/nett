package module

import (
	"reflect"
	"testing"
)

func TestRegisterAndGet(t *testing.T) {
	r := NewRegistry()
	d := &Descriptor{Name: "dns", Short: "resolver", Milestone: "M4", Status: StatusImplemented}
	if err := r.Register(d); err != nil {
		t.Fatalf("Register: %v", err)
	}
	got, ok := r.Get("dns")
	if !ok || got != d {
		t.Fatalf("Get(dns) = %v, %v", got, ok)
	}
	if _, ok := r.Get("missing"); ok {
		t.Error("Get(missing) should be false")
	}
}

func TestRegisterRejectsBadInput(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(nil); err == nil {
		t.Error("nil descriptor should error")
	}
	if err := r.Register(&Descriptor{Name: "", Status: StatusPlanned}); err == nil {
		t.Error("empty name should error")
	}
	if err := r.Register(&Descriptor{Name: "x", Status: "bogus"}); err == nil {
		t.Error("bad status should error")
	}
	if err := r.Register(&Descriptor{Name: "dup", Status: StatusPlanned}); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(&Descriptor{Name: "dup", Status: StatusPlanned}); err == nil {
		t.Error("duplicate name should error")
	}
}

func TestListPreservesOrder(t *testing.T) {
	r := NewRegistry()
	for _, n := range []string{"c", "a", "b"} {
		if err := r.Register(&Descriptor{Name: n, Status: StatusPlanned}); err != nil {
			t.Fatal(err)
		}
	}
	var names []string
	for _, d := range r.List() {
		names = append(names, d.Name)
	}
	if want := []string{"c", "a", "b"}; !reflect.DeepEqual(names, want) {
		t.Errorf("order = %v, want %v", names, want)
	}
}

func TestImplementedFilter(t *testing.T) {
	r := NewRegistry()
	r.MustRegister(&Descriptor{Name: "a", Status: StatusImplemented})
	r.MustRegister(&Descriptor{Name: "b", Status: StatusPlanned})
	r.MustRegister(&Descriptor{Name: "c", Status: StatusImplemented})
	impl := r.Implemented()
	if len(impl) != 2 || impl[0].Name != "a" || impl[1].Name != "c" {
		t.Errorf("Implemented = %+v", impl)
	}
}

func TestCapabilitiesOnlyFromImplementedAndMerged(t *testing.T) {
	r := NewRegistry()
	r.MustRegister(&Descriptor{
		Name: "impl1", Status: StatusImplemented,
		Capabilities: []Capability{{Group: "DNS", Items: []string{"A", "CNAME"}}},
	})
	r.MustRegister(&Descriptor{
		Name: "impl2", Status: StatusImplemented,
		Capabilities: []Capability{{Group: "DNS", Items: []string{"AAAA", "A"}}}, // overlaps "A"
	})
	r.MustRegister(&Descriptor{
		Name: "planned", Status: StatusPlanned,
		Capabilities: []Capability{{Group: "HTTP", Items: []string{"probing"}}},
	})

	caps := r.Capabilities()
	if len(caps) != 1 {
		t.Fatalf("want 1 group (planned excluded), got %d: %+v", len(caps), caps)
	}
	if caps[0].Group != "DNS" {
		t.Fatalf("group = %q, want DNS", caps[0].Group)
	}
	// Merged, deduplicated, sorted.
	if want := []string{"A", "AAAA", "CNAME"}; !reflect.DeepEqual(caps[0].Items, want) {
		t.Errorf("items = %v, want %v", caps[0].Items, want)
	}
}

func TestDefaultRegistryShape(t *testing.T) {
	r := DefaultRegistry()

	core, ok := r.Get("core")
	if !ok || !core.Implemented() {
		t.Fatal("core module must exist and be implemented")
	}

	dns, ok := r.Get("dns")
	if !ok {
		t.Fatal("dns module must exist")
	}
	if !dns.Implemented() {
		t.Fatalf("dns module must be implemented, got status %q", dns.Status)
	}
	if dns.Milestone != "M4" {
		t.Fatalf("dns module milestone = %q, want M4", dns.Milestone)
	}

	foundDNS := false
	for _, c := range r.Capabilities() {
		if c.Group == "DNS" {
			foundDNS = true
			break
		}
	}
	if !foundDNS {
		t.Error("DNS must appear in capabilities after implementation")
	}

	if len(r.Capabilities()) == 0 {
		t.Error("core capabilities should be present")
	}
}
