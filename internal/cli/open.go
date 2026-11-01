package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/codedogapp/baton/internal/flow"
	"github.com/codedogapp/baton/internal/repo"
	"github.com/codedogapp/baton/internal/tmux"
)

// envTmux is the variable that tmux sets in its panes, which names their server.
const envTmux = "TMUX"

// open opens the repo's orchestrator, in the user's own shell: it makes the orchestrator's tmux
// session in the repo's main worktree, unless the session is there already, then shows it in the
// user's terminal (decision 45).
func open(ctx context.Context, env Env, args []string) error {
	if err := parse(newFlags(env, "start"), args); err != nil {
		return err
	}

	if role := env.Getenv(envRole); role != "" {
		return fmt.Errorf("run it in your own shell, not in a pane that baton started: %s "+
			"names the %s", envRole, role)
	}

	mainDir, err := repo.MainWorktree(ctx, env.Dir)
	if err != nil {
		return err
	}

	name := tmux.Name("baton-" + filepath.Base(mainDir))

	switch dir, found := env.Tmux.SessionDir(ctx, name); {
	case !found:
		err = makeOrchestrator(ctx, env, name, mainDir)
	case dir != mainDir:
		err = fmt.Errorf("tmux session %s runs in %s, not in this repo, %s: baton names a repo's "+
			"session after its folder", name, dir, mainDir)
	}

	if err != nil {
		return err
	}

	return show(ctx, env, name)
}

// makeOrchestrator makes the orchestrator's session in the main worktree, with one pane, which runs
// this baton's pane command as the orchestrator (decisions 42 and 44).
func makeOrchestrator(ctx context.Context, env Env, name, mainDir string) error {
	self, err := env.Executable()
	if err != nil {
		return fmt.Errorf("find baton's own program: %w", err)
	}

	err = env.Tmux.NewSession(ctx, tmux.Session{Name: name, Dir: mainDir, Panes: []tmux.Pane{{
		Title:   string(flow.RoleOrchestrator),
		Env:     []string{envRole + "=" + string(flow.RoleOrchestrator)},
		Command: []string{self, "pane"},
	}}})
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(env.Stdout, "Started the orchestrator in tmux session %s.\n", name)

	return nil
}

// show shows the session in the user's terminal. Inside tmux, it switches the user's client to
// the session. Outside, it attaches the terminal to the session, with tmux in baton's place, so
// that tmux keeps the terminal until the user detaches.
func show(ctx context.Context, env Env, name string) error {
	if env.Getenv(envTmux) != "" {
		return env.Tmux.SwitchClient(ctx, name)
	}

	program, err := exec.LookPath("tmux")
	if err == nil {
		err = env.Exec(program, env.Tmux.AttachCommand(name), os.Environ())
	}

	if err != nil {
		return fmt.Errorf("attach to tmux session %s: %w", name, err)
	}

	return nil
}
