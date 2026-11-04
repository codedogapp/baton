// Command baton hands a ticket to a team of Copilot CLI agents in tmux, and enforces their flow.
package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/codedogapp/baton/internal/cli"
	"github.com/codedogapp/baton/internal/keychain"
	"github.com/codedogapp/baton/internal/tmux"
)

func main() {
	// A hangup, from a terminal or a pane that closes, stops baton as Ctrl-C does, so that it
	// undoes what it was making (fact 23, decision 42). Under nohup, a hangup stays ignored.
	signals := []os.Signal{os.Interrupt, syscall.SIGTERM}
	if !signal.Ignored(syscall.SIGHUP) {
		signals = append(signals, syscall.SIGHUP)
	}

	ctx, stop := signal.NotifyContext(context.Background(), signals...)
	context.AfterFunc(ctx, stop) // After the first signal, the next one ends baton at once.

	code := cli.Run(ctx, cli.Env{
		Args:       os.Args[1:],
		Getenv:     os.Getenv,
		Stdin:      os.Stdin,
		Stdout:     os.Stdout,
		Stderr:     os.Stderr,
		Exec:       syscall.Exec,
		Executable: os.Executable,
		Tmux:       tmux.Server{},
		HTTP:       &http.Client{Timeout: time.Minute},
		Keychain:   keychain.Security.Password,
	})

	stop()
	os.Exit(code)
}
