// Package version exposes build and release metadata for nett.
//
// The values are overridable at build time with -ldflags, e.g.:
//
//	go build -ldflags "-X github.com/alieddine/nett/internal/version.Version=1.2.3 \
//	  -X github.com/alieddine/nett/internal/version.Commit=$(git rev-parse --short HEAD) \
//	  -X github.com/alieddine/nett/internal/version.Date=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
//	  ./cmd/nett
package version

import (
	"fmt"
	"runtime"
)

// These are set via -ldflags at build time. The defaults are used for plain
// `go build ./...` and for tests.
var (
	// Version is the semantic version of the build.
	Version = "0.1.0-dev"
	// Commit is the VCS revision the binary was built from.
	Commit = "unknown"
	// Date is the RFC3339 build timestamp.
	Date = "unknown"
)

// Info is a structured view of the build metadata.
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	Date      string `json:"date"`
	GoVersion string `json:"go_version"`
	Platform  string `json:"platform"`
}

// Get returns the current build metadata.
func Get() Info {
	return Info{
		Version:   Version,
		Commit:    Commit,
		Date:      Date,
		GoVersion: runtime.Version(),
		Platform:  fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}
}

// String returns a one-line human-readable version string.
func (i Info) String() string {
	return fmt.Sprintf("nett %s (commit %s, built %s, %s, %s)",
		i.Version, i.Commit, i.Date, i.GoVersion, i.Platform)
}

// String is a package-level convenience for the default build info.
func String() string { return Get().String() }
