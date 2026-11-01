package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// asBaton is the variable that has this test binary run baton's main in place of the tests. The
// skeleton test sets it for the baton commands that it runs and for its tmux server, so for the
// panes and their stand-in copilot too, which run baton as a link to this binary.
const asBaton = "BATON_TEST_AS_BATON"

// TestMain runs baton, when the skeleton test runs this binary as baton. For the tests, it keeps
// git and tmux away from the user's: git without the user's config, in the C locale, with a test
// author, and without the variables that git sets for its hooks, such as GIT_DIR; tmux without
// TMUX and TMUX_PANE, which name the user's server; and no XDG folders, so that baton's cache and
// tmux's config are in the test's home folder. It also clears BATON_ROLE and BATON_FEATURE, which
// name the pane, if a pane of baton's runs the tests.
func TestMain(m *testing.M) {
	if os.Getenv(asBaton) == "1" {
		main()
	}

	var errs []error

	for _, env := range os.Environ() {
		if name, _, _ := strings.Cut(env, "="); strings.HasPrefix(name, "GIT_") {
			errs = append(errs, os.Unsetenv(name))
		}
	}

	for _, name := range []string{
		"TMUX", "TMUX_PANE", "XDG_CACHE_HOME", "XDG_CONFIG_HOME", "BATON_ROLE", "BATON_FEATURE",
	} {
		errs = append(errs, os.Unsetenv(name))
	}

	for _, env := range []string{
		"GIT_CONFIG_GLOBAL=" + os.DevNull, "GIT_CONFIG_NOSYSTEM=1", "LC_ALL=C",
		"GIT_AUTHOR_NAME=baton", "GIT_AUTHOR_EMAIL=baton@example.com",
		"GIT_COMMITTER_NAME=baton", "GIT_COMMITTER_EMAIL=baton@example.com",
	} {
		name, value, _ := strings.Cut(env, "=")
		errs = append(errs, os.Setenv(name, value))
	}

	if err := errors.Join(errs...); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

// TestSkeleton walks a feature through one round, end to end: baton as the user runs it, in a
// throwaway repo, on a tmux server of the test's own, with the stand-in copilot in each pane
// (testdata/copilot). The test, as the orchestrator, starts the feature with its brief, and its
// panes run this baton. The lead briefs the engineer, the engineer adds demo.txt, QA and the
// reviewer approve it, and the lead commits it and hands the final report to the orchestrator,
// whose baton wait picks it up.
func TestSkeleton(t *testing.T) {
	root := skeleton(t)
	feature := filepath.Join(root, "repo", ".git", "baton", "features", "demo-1")

	wantOut(t, exec.CommandContext(t.Context(), "baton", "feature", "start", "demo-1",
		"--brief", "../brief.md"), "Started demo-1: branch feature/demo-1, worktree "+
		filepath.Join(root, "repo-demo-1")+", tmux session baton-repo-demo-1. "+
		"The lead has the brief.\n")
	wantPanes(t)

	// The round takes a few seconds: each pane's baton wait looks for its work every second.
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()

	wait := exec.CommandContext(ctx, "baton", "wait")

	wait.Env = append(os.Environ(), "BATON_ROLE=orchestrator")
	wantOut(t, wait, "Picked up demo-1 as orchestrator: ready, round 1 of 3, from lead.\nNote: "+
		filepath.Join(feature, "notes", "06-lead-to-orchestrator.md")+"\n")

	wantOut(t, exec.CommandContext(t.Context(), "baton", "status"),
		"FEATURE  HOLDER        ROUND   STATUS\ndemo-1   orchestrator  1 of 3  ready\n")
	wantJournal(t, feature, []logged{
		{"start orchestrator  01-orchestrator-to-lead.md", "Add demo.txt.\n"},
		{"pickup lead  ", ""},
		{"handover lead  02-lead-to-engineer.md", "Step brief: add demo.txt.\n"},
		{"pickup engineer  ", ""},
		{"handover engineer  03-engineer-to-qa.md", "Added demo.txt.\n"},
		{"pickup qa  ", ""},
		{"handover qa approve 04-qa-to-reviewer.md", "Checked demo.txt.\n"},
		{"pickup reviewer  ", ""},
		{"handover reviewer approve 05-reviewer-to-lead.md", "Reviewed demo.txt.\n"},
		{"pickup lead  ", ""},
		{"handover lead  06-lead-to-orchestrator.md", "Final report: committed demo.txt.\n"},
		{"pickup orchestrator  ", ""},
	})
	wantFile(t, filepath.Join(root, "copilot.log"),
		"baton:lead 1: Picked up demo-1 as lead: round 0 of 3, from orchestrator.\n"+
			"baton:engineer 1: Picked up demo-1 as engineer: round 1 of 3, from lead.\n"+
			"baton:qa 1: Picked up demo-1 as qa: round 1 of 3, from engineer.\n"+
			"baton:reviewer 1: Picked up demo-1 as reviewer: round 1 of 3, from qa, "+
			"verdict approve.\n"+
			"baton:lead 2: Picked up demo-1 as lead: round 1 of 3, from reviewer, "+
			"verdict approve.\n")

	wantOut(t, exec.CommandContext(t.Context(), "git", "log", "--format=%s",
		"main..feature/demo-1"), "Add demo.txt\n")
	wantOut(t, exec.CommandContext(t.Context(), "git", "show", "feature/demo-1:demo.txt"),
		"Demo.\n")
	wantOut(t, exec.CommandContext(t.Context(), "git", "-C", "../repo-demo-1", "status",
		"--porcelain"), "")
}
