package cli

import (
	"context"
	"fmt"
	"sort"

	"github.com/offsecfergali/nett/internal/module"
	"github.com/offsecfergali/nett/internal/version"
)

// commands is the registered command table. Only implemented commands appear
// here; commands whose modules do not yet exist are intentionally omitted.
var commands = []*Command{
	{
		Name:  "version",
		Short: "Print version and build information",
		Long:  "Print nett's version, VCS commit, build date, Go version, and platform.",
		Run:   runVersion,
	},
	{
		Name:  "capabilities",
		Short: "List capabilities that are actually implemented",
		Long: "Display a tree of the capabilities nett currently implements. Only " +
			"capabilities backed by implemented, tested modules are shown.",
		Run: runCapabilities,
	},
	{
		Name:  "modules",
		Short: "List modules and their implementation status, or show one module",
		Long: "With no argument, list every module in the catalog with its milestone " +
			"and status (implemented|planned). With a name, show that module's detail.",
		Args: "[name]",
		Run:  runModules,
	},
	{
		Name:  "config",
		Short: "Print the fully-resolved effective configuration",
		Long: "Print the effective configuration as JSON, after applying defaults, the " +
			"config file, NETT_* environment variables, and command-line flags.",
		Run: runConfig,
	},
}

// lookup returns the command with the given name, or nil.
func lookup(name string) *Command {
	for _, c := range commands {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func runVersion(_ context.Context, app *App, _ []string) error {
	info := version.Get()
	fmt.Fprintf(app.Out, "nett %s\n", info.Version)
	fmt.Fprintf(app.Out, "  commit:  %s\n", info.Commit)
	fmt.Fprintf(app.Out, "  built:   %s\n", info.Date)
	fmt.Fprintf(app.Out, "  go:      %s\n", info.GoVersion)
	fmt.Fprintf(app.Out, "  platform:%s\n", " "+info.Platform)
	return nil
}

func runCapabilities(_ context.Context, app *App, _ []string) error {
	caps := app.Registry.Capabilities()
	if len(caps) == 0 {
		fmt.Fprintln(app.Out, "No capabilities implemented yet.")
		return nil
	}
	fmt.Fprintln(app.Out, "Implemented capabilities:")
	fmt.Fprintln(app.Out)
	for _, c := range caps {
		renderTree(app.Out, c.Group, c.Items)
		fmt.Fprintln(app.Out)
	}
	return nil
}

func runModules(_ context.Context, app *App, args []string) error {
	if len(args) > 0 {
		return showModule(app, args[0])
	}
	return listModules(app)
}

func listModules(app *App) error {
	mods := app.Registry.List()
	// Determine column width for alignment.
	nameW := len("NAME")
	for _, m := range mods {
		if len(m.Name) > nameW {
			nameW = len(m.Name)
		}
	}
	fmt.Fprintf(app.Out, "%-*s  %-5s  %-11s  %s\n", nameW, "NAME", "MILE", "STATUS", "DESCRIPTION")
	for _, m := range mods {
		fmt.Fprintf(app.Out, "%-*s  %-5s  %-11s  %s\n", nameW, m.Name, m.Milestone, m.Status, m.Short)
	}
	fmt.Fprintln(app.Out)
	fmt.Fprintf(app.Out, "%d modules (%d implemented). Run 'nett modules <name>' for detail.\n",
		len(mods), len(app.Registry.Implemented()))
	return nil
}

func showModule(app *App, name string) error {
	m, ok := app.Registry.Get(name)
	if !ok {
		// Offer near matches to help the user.
		return fmt.Errorf("unknown module %q (run 'nett modules' to list them)", name)
	}
	fmt.Fprintf(app.Out, "Module:     %s\n", m.Name)
	fmt.Fprintf(app.Out, "Summary:    %s\n", m.Short)
	fmt.Fprintf(app.Out, "Milestone:  %s\n", m.Milestone)
	fmt.Fprintf(app.Out, "Status:     %s\n", m.Status)
	fmt.Fprintf(app.Out, "Active:     %t", m.Active)
	if m.Active {
		fmt.Fprint(app.Out, "  (makes network requests; enforces scope before contacting targets)")
	}
	fmt.Fprintln(app.Out)

	if len(m.Capabilities) == 0 {
		return nil
	}
	fmt.Fprintln(app.Out, "Capabilities:")
	// Stable order for deterministic output.
	caps := append([]module.Capability(nil), m.Capabilities...)
	sort.Slice(caps, func(i, j int) bool { return caps[i].Group < caps[j].Group })
	for _, c := range caps {
		renderTree(app.Out, c.Group, c.Items)
	}
	return nil
}

func runConfig(_ context.Context, app *App, _ []string) error {
	s, err := app.Config.JSON()
	if err != nil {
		return fmt.Errorf("render config: %w", err)
	}
	fmt.Fprintln(app.Out, s)
	return nil
}
