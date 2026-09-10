package version

import (
	"runtime"
	"strings"
	"testing"
)

func TestGetPopulatesRuntimeFields(t *testing.T) {
	i := Get()
	if i.GoVersion != runtime.Version() {
		t.Errorf("GoVersion = %q, want %q", i.GoVersion, runtime.Version())
	}
	if !strings.Contains(i.Platform, runtime.GOOS) || !strings.Contains(i.Platform, runtime.GOARCH) {
		t.Errorf("Platform = %q, want to contain %s and %s", i.Platform, runtime.GOOS, runtime.GOARCH)
	}
	if i.Version == "" {
		t.Error("Version must not be empty")
	}
}

func TestStringMentionsVersion(t *testing.T) {
	s := String()
	if !strings.HasPrefix(s, "nett ") || !strings.Contains(s, Version) {
		t.Errorf("String() = %q", s)
	}
}
