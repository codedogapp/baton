package tmux

import (
	"context"
	"slices"
)

// SessionDir returns the folder of the session with the exact name, which the session's new panes
// start in. It returns false if tmux finds no such session, or no server: tmux tells those apart
// from its other failures only in its messages, so a caller that then makes the session gets any
// other failure from that.
func (s Server) SessionDir(ctx context.Context, name string) (string, bool) {
	// Each of the session's windows prints the session's folder. display-message would print an
	// empty line for a session that isn't there (fact 22).
	out, err := s.run(ctx, "list-windows", "-t", "="+name, "-F", "#{session_path}")

	return line(out), err == nil
}

// SwitchClient shows the session in the user's tmux client: the one that shows the pane that runs
// baton, or tmux's latest client outside a pane (fact 22).
func (s Server) SwitchClient(ctx context.Context, name string) error {
	_, err := s.run(ctx, "switch-client", "-t", "="+name)

	return err
}

// AttachCommand returns tmux's command line that attaches the terminal to the session. The caller
// runs it in its own place, as execve does, so that tmux keeps the terminal until the user
// detaches.
func (s Server) AttachCommand(name string) []string {
	return slices.Concat([]string{"tmux"}, s.global(), []string{"attach-session", "-t", "=" + name})
}
