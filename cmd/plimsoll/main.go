// Command plimsoll reports how a repository's shape changed since a base revision.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Stdin, os.Stdout, os.Stderr, os.Environ()) //nolint:forbidigo // composition root owns the process streams
	stop()
	os.Exit(code)
}
