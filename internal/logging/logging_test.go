package logging

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alieddine/nett/internal/config"
)

func TestJSONOutputContainsFields(t *testing.T) {
	var buf bytes.Buffer
	log, err := New(config.LoggingConfig{Level: "info", Format: "json"}, &buf)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer log.Close()

	log.Info("hello", "k", "v")
	out := buf.String()
	for _, want := range []string{`"msg":"hello"`, `"k":"v"`, `"level":"INFO"`} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\ngot: %s", want, out)
		}
	}
}

func TestLevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	log, err := New(config.LoggingConfig{Level: "warn", Format: "text"}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()

	log.Info("suppressed-info")
	log.Warn("visible-warn")
	out := buf.String()
	if strings.Contains(out, "suppressed-info") {
		t.Error("info line should be filtered at warn level")
	}
	if !strings.Contains(out, "visible-warn") {
		t.Error("warn line should be present")
	}
}

func TestComponentTag(t *testing.T) {
	var buf bytes.Buffer
	log, err := New(config.LoggingConfig{Level: "debug", Format: "json"}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()

	log.Component("dns").Info("resolving")
	if !strings.Contains(buf.String(), `"component":"dns"`) {
		t.Errorf("missing component tag: %s", buf.String())
	}
}

func TestInvalidLevelAndFormat(t *testing.T) {
	if _, err := New(config.LoggingConfig{Level: "loud", Format: "text"}, &bytes.Buffer{}); err == nil {
		t.Error("expected error for invalid level")
	}
	if _, err := New(config.LoggingConfig{Level: "info", Format: "yaml"}, &bytes.Buffer{}); err == nil {
		t.Error("expected error for invalid format")
	}
}

func TestFileOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nett.log")
	log, err := New(config.LoggingConfig{Level: "info", Format: "text", File: path}, os.Stderr)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	log.Info("to-file", "run", 1)
	if err := log.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !strings.Contains(string(data), "to-file") {
		t.Errorf("log file missing message: %s", data)
	}
}

func TestDiscardNeverPanics(t *testing.T) {
	log := Discard()
	log.Info("nothing")
	if err := log.Close(); err != nil {
		t.Errorf("Discard Close: %v", err)
	}
}
