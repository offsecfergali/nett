// Package logging provides nett's structured logging, built on the standard
// library's log/slog. It offers configurable levels (debug/info/warn/error),
// output formats (text/json), and optional file output. Every module obtains a
// component-scoped child logger so log output can be filtered by component.
package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/offsecfergali/nett/internal/config"
)

// Logger wraps an *slog.Logger together with any resource (e.g. a log file) that
// must be closed when logging is finished.
type Logger struct {
	*slog.Logger
	closer io.Closer
}

// Close releases any underlying resource (the log file). It is safe to call on a
// Logger that writes to stderr, in which case it is a no-op.
func (l *Logger) Close() error {
	if l.closer != nil {
		return l.closer.Close()
	}
	return nil
}

// Component returns a child logger tagged with a component name. Modules use this
// so their log lines are attributable and filterable.
func (l *Logger) Component(name string) *slog.Logger {
	return l.Logger.With(slog.String("component", name))
}

// parseLevel converts a config level string to an slog.Level.
func parseLevel(level string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info", "":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("logging: unknown level %q", level)
	}
}

// New builds a Logger from the logging configuration, writing to the given
// fallback writer when no log file is configured (the cli passes os.Stderr).
// It returns an error for an unknown level or format, or if the log file cannot
// be opened.
func New(cfg config.LoggingConfig, fallback io.Writer) (*Logger, error) {
	level, err := parseLevel(cfg.Level)
	if err != nil {
		return nil, err
	}

	var (
		w      io.Writer = fallback
		closer io.Closer
	)
	if cfg.File != "" {
		f, err := os.OpenFile(cfg.File, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return nil, fmt.Errorf("logging: open log file %q: %w", cfg.File, err)
		}
		w = f
		closer = f
	}

	handler, err := newHandler(cfg.Format, w, level)
	if err != nil {
		if closer != nil {
			_ = closer.Close()
		}
		return nil, err
	}

	return &Logger{Logger: slog.New(handler), closer: closer}, nil
}

// newHandler selects a text or json slog handler at the requested level.
func newHandler(format string, w io.Writer, level slog.Level) (slog.Handler, error) {
	opts := &slog.HandlerOptions{Level: level}
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "json":
		return slog.NewJSONHandler(w, opts), nil
	case "text", "":
		return slog.NewTextHandler(w, opts), nil
	default:
		return nil, fmt.Errorf("logging: unknown format %q", format)
	}
}

// Discard returns a Logger that drops all output, useful in tests.
func Discard() *Logger {
	h := slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError + 1})
	return &Logger{Logger: slog.New(h)}
}
