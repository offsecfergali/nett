package portscan

import (
	"reflect"
	"testing"
)

func TestParsePortsEmptyReturnsCommon(t *testing.T) {
	got, err := ParsePorts("")
	if err != nil {
		t.Fatalf("ParsePorts(\"\"): %v", err)
	}
	if !reflect.DeepEqual(got, CommonPorts) {
		t.Error("ParsePorts(\"\") should return CommonPorts")
	}
}

func TestParsePortsSingle(t *testing.T) {
	got, err := ParsePorts("443")
	if err != nil {
		t.Fatalf("ParsePorts: %v", err)
	}
	if !reflect.DeepEqual(got, []int{443}) {
		t.Errorf("got %v, want [443]", got)
	}
}

func TestParsePortsList(t *testing.T) {
	got, err := ParsePorts("443,22,80")
	if err != nil {
		t.Fatalf("ParsePorts: %v", err)
	}
	if !reflect.DeepEqual(got, []int{22, 80, 443}) {
		t.Errorf("got %v, want sorted [22 80 443]", got)
	}
}

func TestParsePortsRange(t *testing.T) {
	got, err := ParsePorts("1-5")
	if err != nil {
		t.Fatalf("ParsePorts: %v", err)
	}
	if !reflect.DeepEqual(got, []int{1, 2, 3, 4, 5}) {
		t.Errorf("got %v, want [1 2 3 4 5]", got)
	}
}

func TestParsePortsMixedAndDeduped(t *testing.T) {
	got, err := ParsePorts("80,1-3,3,2")
	if err != nil {
		t.Fatalf("ParsePorts: %v", err)
	}
	if !reflect.DeepEqual(got, []int{1, 2, 3, 80}) {
		t.Errorf("got %v, want deduped sorted [1 2 3 80]", got)
	}
}

func TestParsePortsAll(t *testing.T) {
	got, err := ParsePorts("-")
	if err != nil {
		t.Fatalf("ParsePorts: %v", err)
	}
	if len(got) != MaxPort || got[0] != 1 || got[len(got)-1] != MaxPort {
		t.Errorf("ParsePorts(\"-\") = %d ports [%d..%d], want %d [1..%d]", len(got), got[0], got[len(got)-1], MaxPort, MaxPort)
	}
}

func TestParsePortsRejectsInvalid(t *testing.T) {
	cases := []string{"0", "65536", "abc", "1-", "-5-10", "70000", "5-1"}
	for _, c := range cases {
		if _, err := ParsePorts(c); err == nil {
			t.Errorf("ParsePorts(%q) should have failed", c)
		}
	}
}
