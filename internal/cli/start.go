package cli

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/codedogapp/baton/internal/flow"
	"github.com/codedogapp/baton/internal/repo"
	"github.com/codedogapp/baton/internal/state"
	"github.com/codedogapp/baton/internal/tmux"
)

// Every feature gets these until the config file sets them (decisions 13, 16 and 41).
const (
	roundCap     = 3          // The engineer's passes that a brief gets.
	branchPrefix = "feature/" // The feature's branch is its slug after this.
)

// team is the roles that work on a feature, in the order of their panes.
var team = []flow.Role{flow.RoleLead, flow.RoleEngineer, flow.RoleQA, flow.RoleReviewer}

// runFeature runs the feature command's own command: start, for now.
func runFeature(ctx context.Context, env Env, args []string) int {
	if len(args) > 0 && args[0] == "start" {
		return exit(env.Stderr, "feature start", startFeature(ctx, env, args[1:]))
	}

	_, _ = fmt.Fprintf(env.Stderr, "baton feature: want start\n\n%s", usage)

	return exitUsage
}

// startFeature starts a feature, with the brief as the lead's work: its state, its branch and
// worktree next to the repo's main worktree, and its tmux session, with a pane per role that runs
// baton pane (decisions 40 to 42). Only the orchestrator starts features, so it refuses a role's
// pane.
func startFeature(ctx context.Context, env Env, args []string) error {
	slug, briefPath, err := parseStart(env, args)
	if err != nil {
		return err
	}

	if role := flow.Role(env.Getenv(envRole)); role != "" && role != flow.RoleOrchestrator {
		return fmt.Errorf("only the orchestrator starts a feature, and %s names the %s", envRole,
			role)
	}

	brief, err := readNote(env.Dir, "brief", briefPath)
	if err != nil {
		return err
	}

	start, err := newLaunch(ctx, env, slug)
	if err != nil {
		return err
	}

	start.brief = brief
	if err := start.run(ctx, env.Tmux); err != nil {
		return err
	}

	_, _ = fmt.Fprintf(env.Stdout, "Started %s: branch %s, worktree %s, tmux session %s. "+
		"The lead has the brief.\n", slug, start.worktree.Branch, start.worktree.Path,
		start.session.Name)

	return nil
}

// newLaunch returns where the feature's parts go: its state in the repo's baton folder, and its
// worktree next to the repo's main worktree, with names after the main worktree's folder
// (decision 41).
func newLaunch(ctx context.Context, env Env, slug string) (launch, error) {
	self, err := env.Executable()
	if err != nil {
		return launch{}, fmt.Errorf("find baton's own program: %w", err)
	}

	// Both look for the repo from the working folder, so they fail together, as outside a repo.
	store, err := openStore(ctx, env.Dir)

	var mainDir string
	if err == nil {
		mainDir, err = repo.MainWorktree(ctx, env.Dir)
	}

	if err != nil {
		return launch{}, err
	}

	name := filepath.Base(mainDir) + "-" + slug
	worktree := repo.Worktree{
		Path:   filepath.Join(filepath.Dir(mainDir), name),
		Branch: branchPrefix + slug,
		Base:   "HEAD",
	}

	return launch{
		store: store, slug: slug, mainDir: mainDir, worktree: worktree,
		session: tmux.Session{
			Name:  tmux.Name("baton-" + name),
			Dir:   worktree.Path,
			Panes: panes(self, slug),
		},
	}, nil
}

// parseStart reads baton feature start's command line: the slug, and the brief's file, whose flag
// may come before or after the slug.
func parseStart(env Env, args []string) (slug, brief string, err error) {
	flags := newFlags(env, "feature start")
	briefPath := flags.String("brief", "", "the `file` with the brief for the lead (required)")

	if err := flags.Parse(args); err != nil {
		return "", "", usageError{err}
	}

	if flags.NArg() == 0 {
		return "", "", errors.New(
			"want the feature's slug: baton feature start <slug> --brief <file>")
	}

	slug = flags.Arg(0)
	if err := parse(flags, flags.Args()[1:]); err != nil {
		return "", "", err
	}

	return slug, *briefPath, nil
}

// launch is what baton feature start makes, in this order: the feature, its branch and worktree,
// and its tmux session.
type launch struct {
	store    state.Store
	slug     string
	brief    []byte
	mainDir  string
	worktree repo.Worktree
	session  tmux.Session
}

// run makes the feature, which claims its slug, with the brief as the lead's work, then the
// branch and the worktree, then the tmux session. If a part fails, it undoes the parts it made,
// even after Ctrl-C (decision 40).
func (start launch) run(ctx context.Context, sessions SessionMaker) error {
	now, err := flow.Start(roundCap)
	if err == nil {
		err = start.store.Create(state.Feature{
			Slug:     start.slug,
			Branch:   start.worktree.Branch,
			Worktree: start.worktree.Path,
			Flow:     now,
		}, state.Entry{
			Event: state.Event{Kind: state.EventStart, Role: flow.RoleOrchestrator},
			Note:  start.brief,
		})
	}

	if err != nil {
		return err
	}

	if err := start.worktree.Add(ctx, start.mainDir); err != nil {
		return errors.Join(err, start.store.Remove(start.slug))
	}

	if err := sessions.NewSession(ctx, start.session); err != nil {
		undo := context.WithoutCancel(ctx)

		return errors.Join(err, start.worktree.Undo(undo, start.mainDir),
			start.store.Remove(start.slug))
	}

	return nil
}

// panes returns the feature's panes, one per role, titled with the role, each running this baton's
// pane command with the role and the feature in its environment (decisions 29 and 42).
func panes(self, slug string) []tmux.Pane {
	panes := make([]tmux.Pane, 0, len(team))

	for _, role := range team {
		panes = append(panes, tmux.Pane{
			Title:   string(role),
			Env:     []string{envRole + "=" + string(role), envFeature + "=" + slug},
			Command: []string{self, "pane"},
		})
	}

	return panes
}
