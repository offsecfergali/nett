package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// run executes the CLI with args and returns stdout, stderr, and the exit code.
func run(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	var out, errb bytes.Buffer
	code := Execute(context.Background(), args, &out, &errb)
	return out.String(), errb.String(), code
}

func TestVersionCommand(t *testing.T) {
	out, _, code := run(t, "version")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out, "nett") || !strings.Contains(out, "platform") {
		t.Errorf("unexpected version output:\n%s", out)
	}
}

func TestVersionFlag(t *testing.T) {
	out, _, code := run(t, "--version")
	if code != 0 || !strings.Contains(out, "nett") {
		t.Errorf("--version: code=%d out=%q", code, out)
	}
}

func TestCapabilitiesShowsOnlyImplemented(t *testing.T) {
	out, _, code := run(t, "capabilities")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	for _, want := range []string{"CLI", "Config", "Logging", "Scope", "Store", "DNS"} {
		if !strings.Contains(out, want) {
			t.Errorf("capabilities missing implemented group %q\n%s", want, out)
		}
	}
	// HTTP (the httpx module) is still planned and must not be advertised.
	if strings.Contains(out, "HTTP") {
		t.Errorf("capabilities must not list planned group HTTP:\n%s", out)
	}
}

func TestModulesList(t *testing.T) {
	out, _, code := run(t, "modules")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	for _, want := range []string{"core", "dns", "implemented", "planned", "M4"} {
		if !strings.Contains(out, want) {
			t.Errorf("modules list missing %q\n%s", want, out)
		}
	}
}

func TestModuleDetail(t *testing.T) {
	out, _, code := run(t, "modules", "dns")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	for _, want := range []string{"Module:", "dns", "M4", "planned"} {
		if !strings.Contains(out, want) {
			t.Errorf("module detail missing %q\n%s", want, out)
		}
	}
}

func TestModuleDetailUnknown(t *testing.T) {
	_, errb, code := run(t, "modules", "does-not-exist")
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(errb, "unknown module") {
		t.Errorf("stderr = %q", errb)
	}
}

func TestConfigCommandIsValidJSON(t *testing.T) {
	out, _, code := run(t, "config")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("config output is not valid JSON: %v\n%s", err, out)
	}
	if m["project"] != "default" {
		t.Errorf("project = %v, want default", m["project"])
	}
}

func TestUnknownCommand(t *testing.T) {
	_, errb, code := run(t, "frobnicate")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb, "unknown command") {
		t.Errorf("stderr = %q", errb)
	}
}

func TestHelpAndNoArgs(t *testing.T) {
	out, _, code := run(t, "--help")
	if code != 0 || !strings.Contains(out, "Usage:") {
		t.Errorf("--help: code=%d out=%q", code, out)
	}
	out2, _, code2 := run(t)
	if code2 != 0 || !strings.Contains(out2, "Commands:") {
		t.Errorf("no-args: code=%d out=%q", code2, out2)
	}
}

func TestHelpGoesToStdoutOnceNotStderr(t *testing.T) {
	out, errb, code := run(t, "--help")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if strings.Contains(errb, "Usage:") {
		t.Errorf("--help must not write usage to stderr:\n%s", errb)
	}
	if n := strings.Count(out, "nett — a native Go reconnaissance framework"); n != 1 {
		t.Errorf("usage header printed %d times on stdout, want 1:\n%s", n, out)
	}
}

func TestUnknownFlagShowsUsageOnStderr(t *testing.T) {
	_, errb, code := run(t, "--bogus-flag")
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if !strings.Contains(errb, "Usage:") {
		t.Errorf("parse error should include usage on stderr:\n%s", errb)
	}
}

func TestCommandHelp(t *testing.T) {
	out, _, code := run(t, "modules", "--help")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out, "nett modules") {
		t.Errorf("command help missing usage:\n%s", out)
	}
}

func TestBadLogLevelFlagFails(t *testing.T) {
	_, errb, code := run(t, "--log-level", "screaming", "version")
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(errb, "logging.level") {
		t.Errorf("stderr = %q", errb)
	}
}

func TestProjectFlagOverridesDefault(t *testing.T) {
	out, _, code := run(t, "--project", "myproj", "config")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out, `"project": "myproj"`) {
		t.Errorf("project flag not applied:\n%s", out)
	}
}

func TestFlagBeatsEnv(t *testing.T) {
	t.Setenv("NETT_PROJECT", "envproj")
	// Env alone applies.
	out, _, _ := run(t, "config")
	if !strings.Contains(out, `"project": "envproj"`) {
		t.Errorf("env not applied:\n%s", out)
	}
	// Flag beats env.
	out2, _, _ := run(t, "--project", "flagproj", "config")
	if !strings.Contains(out2, `"project": "flagproj"`) {
		t.Errorf("flag should beat env:\n%s", out2)
	}
}

func TestConfigFileFlag(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.json")
	if err := os.WriteFile(path, []byte(`{"project":"fromfile"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, code := run(t, "--config", path, "config")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out, `"project": "fromfile"`) {
		t.Errorf("config file not loaded:\n%s", out)
	}
}
