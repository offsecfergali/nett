package cli

import (
	"fmt"
	"io"
)

// printRootUsage prints the top-level help, generated from the command table so
// it never drifts from the registered commands.
func printRootUsage(w io.Writer) {
	fmt.Fprintln(w, "nett — a native Go reconnaissance framework")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  nett [global flags] <command> [arguments]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Commands:")
	nameW := len("version")
	for _, c := range commands {
		if len(c.Name) > nameW {
			nameW = len(c.Name)
		}
	}
	for _, c := range commands {
		fmt.Fprintf(w, "  %-*s  %s\n", nameW, c.Name, c.Short)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Global flags:")
	fmt.Fprintln(w, "  --config <path>       JSON config file")
	fmt.Fprintln(w, "  --log-level <level>   debug|info|warn|error")
	fmt.Fprintln(w, "  --log-format <fmt>    text|json")
	fmt.Fprintln(w, "  --data-dir <path>     data directory (SQLite database + artifacts)")
	fmt.Fprintln(w, "  --project <name>      project name (used for resume/diff)")
	fmt.Fprintln(w, "  -v, --verbose         verbose output (log level debug)")
	fmt.Fprintln(w, "  -q, --quiet           quiet output (log level error)")
	fmt.Fprintln(w, "  --version             print version and exit")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Run 'nett <command> --help' for command-specific help.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Note: scan and pipeline commands are added in later milestones as their")
	fmt.Fprintln(w, "modules are implemented; nett never advertises a command it cannot run.")
}

// printCommandUsage prints help for a single command.
func printCommandUsage(w io.Writer, c *Command) {
	args := c.Args
	if args != "" {
		args = " " + args
	}
	fmt.Fprintf(w, "Usage:\n  nett %s%s\n\n", c.Name, args)
	if c.Long != "" {
		fmt.Fprintln(w, c.Long)
	} else {
		fmt.Fprintln(w, c.Short)
	}
}
