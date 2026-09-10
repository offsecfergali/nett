// Command nett is a native Go reconnaissance framework. This entrypoint is
// intentionally thin: it wires up signal-based cancellation and hands control to
// the cli package, which owns all argument parsing, configuration, and dispatch.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/alieddine/nett/internal/cli"
)

func main() {
	// A SIGINT/SIGTERM cancels the root context so in-flight work (later
	// milestones) can unwind cleanly rather than being killed mid-write.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	os.Exit(cli.Execute(ctx, os.Args[1:], os.Stdout, os.Stderr))
}
