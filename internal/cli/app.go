// Package cli implements the nett command-line interface: a small, dependency-
// free command router built on the standard library's flag package. It parses
// global flags, assembles the effective configuration (defaults → file → env →
// flags), initializes logging, builds the module registry, and dispatches to a
// subcommand.
//
// The router deliberately exposes only commands that are actually implemented.
// Commands whose modules do not yet exist (scan, pipeline) are not registered,
// so the CLI never advertises functionality it cannot perform.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/offsecfergali/nett/internal/config"
	"github.com/offsecfergali/nett/internal/logging"
	"github.com/offsecfergali/nett/internal/module"
	"github.com/offsecfergali/nett/internal/version"
)

// App carries the resolved dependencies shared by every command.
type App struct {
	Config   *config.Config
	Log      *logging.Logger
	Registry *module.Registry
	Out      io.Writer
	Err      io.Writer
}

// Command is a single CLI subcommand.
type Command struct {
	Name  string
	Short string // one-line summary for the command list
	Long  string // longer help shown by `nett <cmd> --help`
	Args  string // argument usage suffix, e.g. "[name]"
	Run   func(ctx context.Context, app *App, args []string) error
}

// Execute runs the CLI with the given arguments (excluding the program name) and
// returns a process exit code. stdout/stderr are injected so the whole CLI is
// testable without touching the real process streams.
//
// Exit codes: 0 success, 1 runtime error, 2 usage error.
func Execute(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	var (
		configPath  string
		logLevel    string
		logFormat   string
		dataDir     string
		project     string
		verbose     bool
		quiet       bool
		showVersion bool
	)

	fs := flag.NewFlagSet("nett", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&configPath, "config", "", "path to JSON config file")
	fs.StringVar(&logLevel, "log-level", "", "log level: debug|info|warn|error")
	fs.StringVar(&logFormat, "log-format", "", "log format: text|json")
	fs.StringVar(&dataDir, "data-dir", "", "data directory (SQLite database + artifacts)")
	fs.StringVar(&project, "project", "", "project name (used for resume/diff)")
	fs.BoolVar(&verbose, "v", false, "verbose output (log level debug)")
	fs.BoolVar(&verbose, "verbose", false, "verbose output (log level debug)")
	fs.BoolVar(&quiet, "q", false, "quiet output (log level error)")
	fs.BoolVar(&quiet, "quiet", false, "quiet output (log level error)")
	fs.BoolVar(&showVersion, "version", false, "print version and exit")
	// Suppress flag's automatic usage printing so we control the stream and
	// avoid printing usage twice: --help goes to stdout (exit 0), a parse error
	// goes to stderr (exit 2).
	fs.Usage = func() {}

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printRootUsage(stdout)
			return 0
		}
		// flag already wrote the specific error to stderr; add usage for context.
		fmt.Fprintln(stderr)
		printRootUsage(stderr)
		return 2
	}

	if showVersion {
		fmt.Fprintln(stdout, version.String())
		return 0
	}

	rest := fs.Args()
	if len(rest) == 0 {
		printRootUsage(stdout)
		return 0
	}
	cmdName, cmdArgs := rest[0], rest[1:]

	cmd := lookup(cmdName)
	if cmd == nil {
		fmt.Fprintf(stderr, "nett: unknown command %q\n\n", cmdName)
		printRootUsage(stderr)
		return 2
	}

	// Per-command help does not require a valid config, so handle it first.
	if hasHelpFlag(cmdArgs) {
		printCommandUsage(stdout, cmd)
		return 0
	}

	// Assemble configuration: defaults → file → env, then flag overrides.
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintln(stderr, "nett: "+err.Error())
		return 1
	}
	applyFlagOverrides(cfg, logLevel, logFormat, dataDir, project, verbose, quiet)
	if err := cfg.Validate(); err != nil {
		fmt.Fprintln(stderr, "nett: "+err.Error())
		return 1
	}

	log, err := logging.New(cfg.Logging, stderr)
	if err != nil {
		fmt.Fprintln(stderr, "nett: "+err.Error())
		return 1
	}
	defer log.Close()

	app := &App{
		Config:   cfg,
		Log:      log,
		Registry: module.DefaultRegistry(),
		Out:      stdout,
		Err:      stderr,
	}

	app.Log.Component("cli").Debug("dispatching", "command", cmd.Name, "args", cmdArgs)

	if err := cmd.Run(ctx, app, cmdArgs); err != nil {
		app.Log.Component("cli").Error("command failed", "command", cmd.Name, "err", err)
		fmt.Fprintln(stderr, "nett: "+err.Error())
		return 1
	}
	return 0
}

// applyFlagOverrides applies the highest-precedence layer (CLI flags) onto cfg.
// An explicit --log-level beats the -v/-q shortcuts; -v beats -q if both appear.
func applyFlagOverrides(cfg *config.Config, logLevel, logFormat, dataDir, project string, verbose, quiet bool) {
	switch {
	case logLevel != "":
		cfg.Logging.Level = logLevel
	case verbose:
		cfg.Logging.Level = "debug"
	case quiet:
		cfg.Logging.Level = "error"
	}
	if logFormat != "" {
		cfg.Logging.Format = logFormat
	}
	if dataDir != "" {
		cfg.DataDir = dataDir
	}
	if project != "" {
		cfg.Project = project
	}
}

// hasHelpFlag reports whether -h/--help/help appears in a command's args.
func hasHelpFlag(args []string) bool {
	for _, a := range args {
		if a == "-h" || a == "--help" || a == "help" {
			return true
		}
	}
	return false
}
