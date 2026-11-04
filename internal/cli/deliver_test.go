package cli_test

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codedogapp/baton/internal/cli"
	"github.com/codedogapp/baton/internal/flow"
	"github.com/codedogapp/baton/internal/state"
)

// pullRequest is the URL of the pull request that the stand-in gh opens.
const pullRequest = "https://github.com/example/demo/pull/7"

// delivery is a repo where demo-1 is ready to deliver.
type delivery struct {
	mainDir string
	origin  string // The bare repo that is the repo's origin.
	store   state.Store
	report  string // The path of the lead's final report.
}

// readyToDeliver makes a repo with a bare repo of the test's as its origin, and demo-1 on the
// linked worktree's branch, which the lead hands to the orchestrator with its final report.
func readyToDeliver(t *testing.T) delivery {
	t.Helper()
	withGate(t, "true")

	mainDir, linked := repos(t)
	gitIn(t, mainDir, exec.CommandContext(t.Context(), "git", "init", "--quiet", "--bare",
		"../origin.git"))
	gitIn(t, mainDir, exec.CommandContext(t.Context(), "git", "remote", "add", "origin",
		"../origin.git"))

	store := started(t, mainDir, flow.State{
		Holder: flow.RoleLead, Status: flow.StatusRunning, Round: 1, Cap: 3,
	})
	edit(t, store, func(feature *state.Feature) {
		feature.Branch, feature.Worktree = "linked", linked
	})
	handOver(t, mainDir, flow.RoleLead, flow.VerdictNone)

	origin := filepath.Join(filepath.Dir(mainDir), "origin.git")

	return delivery{mainDir, origin, store, notePath(store, "02-lead-to-orchestrator.md")}
}

// standInGh puts a stand-in gh first on PATH, which writes its arguments, a line each, to the file
// that it returns, then runs the shell script.
func standInGh(t *testing.T, script string) (calls string) {
	t.Helper()

	bin, executable := t.TempDir(), os.FileMode(0o700)
	calls = filepath.Join(bin, "calls")
	text := fmt.Sprintf("#!/bin/sh\nfor arg; do printf '%%s\\n' \"$arg\"; done > %q\n%s", calls,
		script)

	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(text), executable); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", bin+string(filepath.ListSeparator)+os.Getenv("PATH"))

	return calls
}

// wantGh checks the stand-in gh's arguments, a line each, or that gh didn't run if want is "".
func wantGh(t *testing.T, calls, want string) {
	t.Helper()

	got, err := os.ReadFile(filepath.Clean(calls))
	if want == "" && !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("gh ran: %q, %v", got, err)
	}

	if want != "" && string(got) != want {
		t.Errorf("gh ran with %q, %v; want %q", got, err, want)
	}
}

// wantPushed checks the commit of the branch linked in the origin, or that origin has no such
// branch if want is "".
func wantPushed(t *testing.T, origin, want string) {
	t.Helper()

	out := gitIn(t, origin, exec.CommandContext(t.Context(), "git", "for-each-ref",
		"--format=%(objectname)", "refs/heads/linked"))
	if got := strings.TrimSpace(out); got != want {
		t.Errorf("origin's linked is at %q, want %q", got, want)
	}
}

// head returns the commit of the repo's branch linked.
func head(t *testing.T, mainDir string) string {
	t.Helper()

	out := gitIn(t, mainDir, exec.CommandContext(t.Context(), "git", "rev-parse", "linked"))

	return strings.TrimSpace(out)
}

func TestDeliverPushesTheBranchAndOpensItsPullRequest(t *testing.T) {
	ready := readyToDeliver(t)
	calls := standInGh(t, "echo "+pullRequest+"\n")

	got := run(t, ready.mainDir, orchestratorPane(), "deliver", slug, "--title", " Add the demo \n")
	if want := (result{stdout: "Delivered demo-1: pushed linked to origin and opened its pull " +
		"request, " + pullRequest + ".\n"}); got != want {
		t.Errorf("got %+v; want %+v", got, want)
	}

	wantPushed(t, ready.origin, head(t, ready.mainDir))
	wantGh(t, calls, "pr\ncreate\n--head=linked\n--title=Add the demo\n--body-file="+
		ready.report+"\n")

	feature, err := ready.store.Read(slug)
	if want := (flow.State{
		Holder: flow.RoleOrchestrator, Status: flow.StatusDelivered, Round: 1, Cap: 3,
	}); err != nil || feature.Flow != want || feature.PullRequest != pullRequest {
		t.Errorf("Read = %+v, %v; want %+v, with the pull request", feature, err, want)
	}

	wantJournal(t, ready.store, []logged{
		{"start orchestrator  01-orchestrator-to-lead.md", "Build the demo.\n"},
		{"edit orchestrator  ", ""},
		{"handover lead  02-lead-to-orchestrator.md (gate: true)", "From the lead.\n"},
		{"deliver orchestrator  ", pullRequest},
	})
}

// TestDeliverCommentsOnThePullRequestOfAnEarlierDelivery delivers a feature that came back ready,
// after a delivery and changes, from the user's own shell.
func TestDeliverCommentsOnThePullRequestOfAnEarlierDelivery(t *testing.T) {
	ready := readyToDeliver(t)
	edit(t, ready.store, func(feature *state.Feature) { feature.PullRequest = pullRequest })

	calls := standInGh(t, "echo "+pullRequest+"#issuecomment-1\n")

	got := run(t, ready.mainDir, nil, "deliver", slug)
	if want := (result{stdout: "Delivered demo-1: pushed linked to origin and commented the " +
		"lead's report on its pull request, " + pullRequest + ".\n"}); got != want {
		t.Errorf("got %+v; want %+v", got, want)
	}

	wantPushed(t, ready.origin, head(t, ready.mainDir))
	wantGh(t, calls, "pr\ncomment\n--body-file="+ready.report+"\n--\n"+pullRequest+"\n")

	if feature, err := ready.store.Read(slug); err != nil ||
		feature.Flow.Status != flow.StatusDelivered || feature.PullRequest != pullRequest {
		t.Errorf("Read = %+v, %v; want it delivered, with the pull request", feature, err)
	}
}

// TestDeliverReportsAnOutputItCantWrite fails a delivery that can't print what it did. The
// delivery stays recorded, so the next one comments on its pull request.
func TestDeliverReportsAnOutputItCantWrite(t *testing.T) {
	ready := readyToDeliver(t)
	standInGh(t, "echo "+pullRequest+"\n")

	var stderr strings.Builder

	code := cli.Run(t.Context(), cli.Env{
		Args:   []string{"deliver", slug, "--title", "Add the demo"},
		Getenv: func(name string) string { return orchestratorPane()[name] },
		Dir:    ready.mainDir,
		Stdout: failing{},
		Stderr: &stderr,
	})

	if got := stderr.String(); code != 1 || got != "baton deliver: output closed\n" {
		t.Errorf("exit code %d and %q; want 1 and the output's error", code, got)
	}

	if feature, err := ready.store.Read(slug); err != nil ||
		feature.Flow.Status != flow.StatusDelivered || feature.PullRequest != pullRequest {
		t.Errorf("Read = %+v, %v; want it delivered, with the pull request", feature, err)
	}
}
