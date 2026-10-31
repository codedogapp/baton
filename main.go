// Command baton hands a ticket to a team of Copilot CLI agents in tmux, and enforces their flow.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/codedogapp/baton/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	context.AfterFunc(ctx, stop) // After the first signal, the next one ends baton at once.

	code := cli.Run(ctx, cli.Env{
		Args:   os.Args[1:],
		Getenv: os.Getenv,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	})

	stop()
	os.Exit(code)
}
