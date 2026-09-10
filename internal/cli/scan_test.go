package cli

import (
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestParseScanArgsExamplesFromSpec(t *testing.T) {
	cases := []struct {
		args []string
	}{
		{[]string{"example.com", "-d"}},
		{[]string{"example.com", "-brute", "words.txt"}},
		{[]string{"example.com", "-tcp"}},
		{[]string{"example.com", "-tcp", "1-1000"}},
		{[]string{"example.com", "-tcp", "22,80,443"}},
		{[]string{"example.com", "-p"}},
		{[]string{"example.com", "-d", "-brute", "words.txt", "-p"}},
	}
	for _, c := range cases {
		t.Run(strings.Join(c.args, "_"), func(t *testing.T) {
			f, err := parseScanArgs(c.args)
			if err != nil {
				t.Fatalf("parseScanArgs(%v): %v", c.args, err)
			}
			if f.target != "example.com" {
				t.Errorf("target = %q, want example.com", f.target)
			}
		})
	}
}

func TestParseScanArgsTCPOptionalValue(t *testing.T) {
	f, err := parseScanArgs([]string{"example.com", "-tcp", "1-1000"})
	if err != nil {
		t.Fatalf("parseScanArgs: %v", err)
	}
	if !f.tcpRequested || f.tcpSpec != "1-1000" {
		t.Errorf("f = %+v, want tcpRequested=true tcpSpec=1-1000", f)
	}

	f, err = parseScanArgs([]string{"example.com", "-tcp", "-p"})
	if err != nil {
		t.Fatalf("parseScanArgs: %v", err)
	}
	if !f.tcpRequested || f.tcpSpec != "" || !f.allPorts {
		t.Errorf("f = %+v, want tcpRequested=true tcpSpec=\"\" allPorts=true (-tcp must not swallow the next flag)", f)
	}
}

func TestParseScanArgsUnimplementedFlagsRejected(t *testing.T) {
	for _, args := range [][]string{
		{"example.com", "-udp"},
		{"example.com", "-f"},
		{"example.com", "-en"},
		{"example.com", "-dir", "words.txt"},
	} {
		if _, err := parseScanArgs(args); err == nil {
			t.Errorf("parseScanArgs(%v) should fail: flag not implemented yet", args)
		}
	}
}

func TestParseScanArgsRequiresTarget(t *testing.T) {
	if _, err := parseScanArgs([]string{"-d"}); err == nil {
		t.Error("parseScanArgs should require a target")
	}
}

func TestParseScanArgsRequiresAction(t *testing.T) {
	if _, err := parseScanArgs([]string{"example.com"}); err == nil {
		t.Error("parseScanArgs should require at least one action flag")
	}
}

func TestParseScanArgsRejectsMultipleTargets(t *testing.T) {
	if _, err := parseScanArgs([]string{"example.com", "other.com", "-d"}); err == nil {
		t.Error("parseScanArgs should reject more than one positional target")
	}
}

func TestParseScanArgsRejectsUnknownFlag(t *testing.T) {
	if _, err := parseScanArgs([]string{"example.com", "-bogus"}); err == nil {
		t.Error("parseScanArgs should reject an unknown flag")
	}
}

func TestParseScanArgsBruteRequiresValue(t *testing.T) {
	if _, err := parseScanArgs([]string{"example.com", "-brute"}); err == nil {
		t.Error("-brute should require a wordlist argument")
	}
}

// TestScanTCPEndToEnd exercises the full `nett scan <ip> -tcp <port>` path
// through Execute against a real local TCP listener, verifying the scanner
// finds the open port and the run completes successfully end-to-end
// (scope auto-inclusion of the literal IP target, store creation, output).
func TestScanTCPEndToEnd(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	_, portStr, _ := net.SplitHostPort(ln.Addr().String())
	port, _ := strconv.Atoi(portStr)

	dataDir := t.TempDir()
	out, errOut, code := run(t, "--data-dir", dataDir, "--project", "t1", "scan", "127.0.0.1", "-tcp", portStr)
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, errOut)
	}
	if !strings.Contains(out, strconv.Itoa(port)+"/tcp   open") {
		t.Errorf("output missing open port line:\n%s", out)
	}

	if _, err := os.Stat(filepath.Join(dataDir, "t1.db")); err != nil {
		t.Errorf("expected a SQLite database to be created at %s: %v", filepath.Join(dataDir, "t1.db"), err)
	}
}

func TestScanRejectsOutOfScopeIP(t *testing.T) {
	// A closed/unrelated port on an address we did not ask to scan is not
	// the point here — this checks that requesting a scan is otherwise
	// well-formed; scope auto-inclusion of the literal target is what makes
	// TestScanTCPEndToEnd work, so this documents the negative: config-level
	// exclude wins even over the CLI-target auto-include.
	dataDir := t.TempDir()
	cfgPath := filepath.Join(dataDir, "nett.json")
	if err := os.WriteFile(cfgPath, []byte(`{"scope":{"exclude":["127.0.0.1"]}}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, errOut, code := run(t, "--config", cfgPath, "--data-dir", dataDir, "--project", "t1", "scan", "127.0.0.1", "-tcp", "1")
	if code == 0 {
		t.Errorf("expected a non-zero exit when the target is explicitly excluded, stderr=%s", errOut)
	}
	if !strings.Contains(errOut, "not in scope") {
		t.Errorf("stderr = %q, want a scope-related error", errOut)
	}
}

func TestScanMissingWordlistFile(t *testing.T) {
	dataDir := t.TempDir()
	_, errOut, code := run(t, "--data-dir", dataDir, "--project", "t1", "scan", "example.com", "-brute", filepath.Join(dataDir, "nope.txt"))
	if code == 0 {
		t.Error("expected a non-zero exit for a missing wordlist")
	}
	if !strings.Contains(errOut, "-brute") {
		t.Errorf("stderr = %q, want it to reference -brute", errOut)
	}
}
