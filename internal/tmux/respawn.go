package tmux

import (
	"context"
	"fmt"
	"strings"
)

// Respawn starts again each dead pane of the session, whose program ended and which the session
// kept (Remain), if the pane has a title of the session's panes: it runs that pane's command
// again, with its environment, in the session's folder, as NewSession did. It leaves the other
// panes as they are: the live ones, and the dead ones with other titles, such as the user's. It
// returns the titles of the panes that it started again, in the session's order, even when it
// fails partway.
func (s Server) Respawn(ctx context.Context, session Session) ([]string, error) {
	if err := session.check(); err != nil {
		return nil, err
	}

	// A title may hold spaces, but no ID does. tmux keeps the title through the respawn.
	out, err := s.run(ctx, "list-panes", "-s", "-t", "="+session.Name, "-f", "#{pane_dead}",
		"-F", "#{pane_id} #{pane_title}")
	if err != nil {
		return nil, fmt.Errorf("find the dead panes of tmux session %s: %w", session.Name, err)
	}

	dead := map[string]string{}

	for pane := range strings.Lines(out) {
		target, title, _ := strings.Cut(strings.TrimSuffix(pane, "\n"), " ")
		dead[title] = target
	}

	var started []string

	for _, pane := range session.Panes {
		target, ok := dead[pane.Title]
		if !ok {
			continue
		}

		// Without -k, tmux refuses a pane whose program runs again already.
		if _, err := s.run(ctx, session.start(pane, "respawn-pane", "-t", target)...); err != nil {
			return started, fmt.Errorf("start the %s pane of tmux session %s again: %w",
				pane.Title, session.Name, err)
		}

		started = append(started, pane.Title)
	}

	return started, nil
}
