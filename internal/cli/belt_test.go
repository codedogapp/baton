package cli_test

import (
	"cmp"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/codedogapp/baton/internal/flow"
	"github.com/codedogapp/baton/internal/state"
)

// The belt tests run baton pane in the lead's pane, then git with the environment that baton gave
// the lead's copilot, as a command or a script there would (decision 55). Each push that would
// leave the machine goes to a host under .invalid, which no resolver knows, and git runs ssh as
// false, so a push that the belt misses fails too, without reaching a host, but for another reason.

// TestRolePaneBlocksPushesThatLeave has the lead's copilot's git, in a repo with remotes, refuse
// each push that would leave the machine, with no network: to a remote's url, ahead of the user's
// own shorter rule for it; to a remote's pushurl, in this git, and in a git before 2.46, which
// can't clear pushurls; and to URLs typed in full. Pushes to a local remote, whose path holds a
// colon, a file:// remote and a path go on, and so do fetches (fact 38).
func TestRolePaneBlocksPushesThatLeave(t *testing.T) {
	for _, version := range []string{"", "git version 2.45.2"} {
		t.Run(cmp.Or(version, "this git"), func(t *testing.T) {
			standIn(t)

			mainDir, linked := withRemotes(t)
			if version != "" {
				gitBut(t, "version) echo '"+version+"'; exit ;;")
			}

			started(t, mainDir, fresh())

			var vars []string

			synctest.Test(t, func(t *testing.T) {
				env, _ := inPane(t, linked, paneOf(flow.RoleLead))
				vars = copilotsEnv(t, paneFor(t, env))
			})

			ctx := t.Context()

			for _, cmd := range []*exec.Cmd{
				exec.CommandContext(ctx, "git", "push", "origin", "HEAD:refs/heads/demo"),
				exec.CommandContext(ctx, "git", "push", "mirror", "HEAD:refs/heads/demo"),
				exec.CommandContext(ctx, "git", "push", "git@example.invalid:typed.git",
					"HEAD:refs/heads/demo"),
				exec.CommandContext(ctx, "git", "push", "HTTPS://example.invalid/typed.git",
					"HEAD:refs/heads/demo"),
				exec.CommandContext(ctx, "git", "push", "[git@example.invalid]:typed.git",
					"HEAD:refs/heads/demo"),
			} {
				wantBlocked(t, cmd, linked, vars)
			}

			for _, cmd := range []*exec.Cmd{
				exec.CommandContext(ctx, "git", "push", "local", "HEAD:refs/heads/local"),
				exec.CommandContext(ctx, "git", "push", "files", "HEAD:refs/heads/files"),
				exec.CommandContext(ctx, "git", "push", "../local:bare.git", "HEAD:refs/heads/path"),
				exec.CommandContext(ctx, "git", "fetch", "local", "refs/heads/local"),
			} {
				wantDone(t, cmd, linked, vars)
			}
		})
	}
}

// withRemotes makes a repo with a linked worktree, as repos does, and a bare repo next to them,
// whose path holds a colon, and gives the repo remotes, in a config file that its own includes:
// origin, which leaves the machine, under a shorter rule of the user's that pushes over ssh;
// mirror, whose pushurl leaves too; and local and files, the bare repo by its path and by a
// file:// URL.
func withRemotes(t *testing.T) (mainDir, linked string) {
	t.Helper()

	mainDir, linked = repos(t)
	gitIn(t, mainDir, exec.CommandContext(t.Context(), "git", "init", "--quiet", "--bare",
		"../local:bare.git"))

	bare := filepath.Join(filepath.Dir(mainDir), "local:bare.git")
	write(t, filepath.Join(mainDir, ".git"), "remotes", `[url "ssh://git@example.invalid/"]
	pushInsteadOf = https://example.invalid/
[remote "origin"]
	url = https://example.invalid/demo.git
[remote "mirror"]
	url = https://example.invalid/mirror.git
	pushurl = git@example.invalid:mirror.git
[remote "local"]
	url = `+bare+`
[remote "files"]
	url = file://`+bare+"\n")
	gitIn(t, mainDir, exec.CommandContext(t.Context(), "git", "config", "include.path",
		"remotes"))

	return mainDir, linked
}

// copilotsEnv returns the environment that baton pane gave copilot, with ssh as false for git.
func copilotsEnv(t *testing.T, got ran) []string {
	t.Helper()

	if len(got.execs) != 1 {
		t.Fatalf("got %+v; want copilot started", got)
	}

	return append(slices.Clone(got.execs[0].env), "GIT_SSH_COMMAND=false")
}

// wantBlocked checks that git's command, run in the folder with the environment, fails at baton's
// missing remote helper.
func wantBlocked(t *testing.T, cmd *exec.Cmd, dir string, vars []string) {
	t.Helper()

	cmd.Dir, cmd.Env = dir, vars
	out, err := cmd.CombinedOutput()

	if err == nil || !strings.Contains(string(out), "'remote-baton-blocks-push' is not a git") {
		t.Errorf("%s: %v, %s; want baton's missing remote helper", cmd, err, out)
	}
}

// wantDone checks that git's command, run in the folder with the environment, succeeds.
func wantDone(t *testing.T, cmd *exec.Cmd, dir string, vars []string) {
	t.Helper()

	cmd.Dir, cmd.Env = dir, vars
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("%s: %v, %s; want it done", cmd, err, out)
	}
}

// TestRolePaneRefusesWithoutItsBelt has the lead's pane refuse before it waits, so that the lead's
// work stays where it is, when it can't make the belt: when git fails as it reads the remotes'
// URLs, or git's version, or when baton can't read the count of git's settings in its environment.
func TestRolePaneRefusesWithoutItsBelt(t *testing.T) {
	cases := []struct {
		name   string
		fails  string // git's command that fails, if any.
		count  string // GIT_CONFIG_COUNT, if any.
		reason string
	}{
		{name: "remotes", fails: "config", reason: "read the remotes' URLs: exit status 3"},
		{name: "version", fails: "version", reason: "find git's version: exit status 3"},
		{
			name:   "count",
			count:  " 0",
			reason: `GIT_CONFIG_COUNT=" 0" isn't a count of git's settings`,
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			standIn(t)

			mainDir, linked := repos(t)
			store := started(t, mainDir, fresh())

			if test.fails != "" {
				gitBut(t, test.fails+") exit 3 ;;")
			}

			if test.count != "" {
				t.Setenv("GIT_CONFIG_COUNT", test.count)
			}

			synctest.Test(t, func(t *testing.T) {
				env, _ := inPane(t, linked, paneOf(flow.RoleLead))
				wantRefused(t, paneFor(t, env), "", test.reason+"\n")
			})

			wantWork(t, store, state.Work{
				Note: "01-orchestrator-to-lead.md", From: flow.RoleOrchestrator,
			})
		})
	}
}

// gitBut puts a git first on the PATH that runs the shell's case for git's first argument, which
// ends the script, and the real git for every other.
func gitBut(t *testing.T, cases string) {
	t.Helper()

	program, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}

	dir, executable := t.TempDir(), os.FileMode(0o700)
	text := fmt.Sprintf("#!/bin/sh\ncase $1 in\n%s\nesac\nexec '%s' \"$@\"\n", cases, program)

	if err := os.WriteFile(filepath.Join(dir, "git"), []byte(text), executable); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", dir+string(filepath.ListSeparator)+os.Getenv("PATH"))
}
