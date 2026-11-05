package cli_test

import (
	"cmp"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codedogapp/baton/internal/cli"
	"github.com/codedogapp/baton/internal/flow"
	"github.com/codedogapp/baton/internal/state"
)

// TestFeatureCloseRefusesAndChangesNothing checks that a refusal ends no tmux session, removes no
// worktree, and leaves baton's folder as it was.
func TestFeatureCloseRefusesAndChangesNothing(t *testing.T) {
	demo := newTeam(t)

	cases := []struct {
		name    string
		args    []string          // After baton feature close.
		env     map[string]string // The pane's environment.
		session string            // The folder of tmux session baton-linked, if not demo-1's.
		want    string            // The reason, after "baton feature close: ".
	}{
		{
			name: "without a slug", args: []string{"--remove-worktree"},
			want: "want the feature's slug: baton feature close <slug> [--remove-worktree]",
		},
		{
			name: "with an argument after the slug", args: []string{slug, "now"},
			want: `unexpected argument "now"`,
		},
		{
			name: "in a role's pane", args: []string{slug}, env: paneOf(flow.RoleLead),
			want: "only the orchestrator closes a feature, and BATON_ROLE names the lead",
		},
		{
			name: "for a feature that isn't there", args: []string{"demo-9"},
			want: "no feature demo-9",
		},
		{
			name: "with a session of its name in another folder",
			args: []string{slug, "--remove-worktree"}, env: orchestratorPane(),
			session: demo.mainDir,
			want: "tmux session baton-linked runs in " + demo.mainDir + ", not in the worktree " +
				"of demo-1, " + demo.linked + ": baton changes only the sessions that it made",
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			refused := demo
			refused.sessions = &fakeTmux{
				dirs: map[string]string{"baton-linked": cmp.Or(test.session, demo.linked)},
			}
			before := snapshot(t, demo.store.Dir)

			got, want := refused.closeFrom(t, test.env, test.args...),
				result{code: 1, stderr: "baton feature close: " + test.want + "\n"}
			if got != want {
				t.Errorf("got %+v; want %+v", got, want)
			}

			if len(refused.sessions.killed) > 0 {
				t.Errorf("killed %q; want none", refused.sessions.killed)
			}

			if _, err := os.Stat(demo.linked); err != nil {
				t.Errorf("the worktree: %v", err)
			}

			if after := snapshot(t, demo.store.Dir); !maps.Equal(after, before) {
				t.Errorf("baton's folder changed:\n%v\nwant:\n%v", after, before)
			}
		})
	}
}

// TestFeatureCloseRefusesOutsideARepo refuses a close that has no git repo, and so no feature.
func TestFeatureCloseRefusesOutsideARepo(t *testing.T) {
	outside := t.TempDir()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(outside))

	demo := team{mainDir: outside, sessions: &fakeTmux{}}
	got := demo.closeFrom(t, orchestratorPane(), slug)

	want := "baton feature close: find the git repo: fatal: not a git repository"
	if got.code != 1 || got.stdout != "" || !strings.HasPrefix(got.stderr, want) {
		t.Errorf("got %+v; want exit code 1 and %q", got, want)
	}
}

// TestFeatureCloseFailsOnceAnotherCloseCameFirst has another close close demo-1 while this one
// looks for the feature's tmux session: this one fails, and leaves the session to the other.
func TestFeatureCloseFailsOnceAnotherCloseCameFirst(t *testing.T) {
	demo := newTeam(t)
	demo.sessions.looking = func() {
		edit(t, demo.store, func(feature *state.Feature) {
			feature.Flow.Holder, feature.Flow.Status = flow.RoleOrchestrator, flow.StatusClosed
		})
	}

	got, want := demo.closeFrom(t, orchestratorPane(), slug), result{
		code: 1, stderr: "baton feature close: demo-1: the feature is closed already\n",
	}
	if got != want {
		t.Errorf("got %+v; want %+v", got, want)
	}

	if len(demo.sessions.killed) > 0 {
		t.Errorf("killed %q; want none", demo.sessions.killed)
	}
}

// TestFeatureCloseReportsAnOutputItCantWrite fails a close that can't print what it did. The
// feature stays closed, and its tmux session ended.
func TestFeatureCloseReportsAnOutputItCantWrite(t *testing.T) {
	demo := newTeam(t)

	var stderr strings.Builder

	code := cli.Run(t.Context(), cli.Env{
		Args:   []string{"feature", "close", slug},
		Getenv: func(name string) string { return orchestratorPane()[name] },
		Dir:    demo.mainDir,
		Stdout: failing{},
		Stderr: &stderr,
		Tmux:   demo.sessions,
	})

	if got := stderr.String(); code != 1 || got != "baton feature close: output closed\n" {
		t.Errorf("exit code %d and %q; want 1 and the output's error", code, got)
	}

	demo.wantClosed(t, []string{"baton-linked"})
}
