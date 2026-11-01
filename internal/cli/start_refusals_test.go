package cli_test

import (
	"cmp"
	"errors"
	"maps"
	"path/filepath"
	"strings"
	"testing"
)

// TestStartFeatureRefuses tries to start feature demo-2 where demo-1 has started, and checks that
// a refusal changes nothing: baton's folder, the repo's worktrees and branches, and tmux.
func TestStartFeatureRefuses(t *testing.T) {
	mainDir, linked := repos(t)
	store := started(t, mainDir, fresh())
	brief := write(t, linked, "brief.md", "Build the demo.\n")
	blank := write(t, linked, "blank.md", " \n\t\n")

	outside := t.TempDir()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(outside))

	cases := []struct {
		name string
		dir  string   // The working folder, if not the linked worktree.
		args []string // After baton feature start.
		role string   // BATON_ROLE, if any.
		self error    // Why baton can't find its own program, if it can't.
		want string   // The start of the reason, after "baton feature start: ".
	}{
		{
			name: "without a slug", args: []string{"--brief", "brief.md"},
			want: "want the feature's slug: baton feature start <slug> --brief <file>\n",
		},
		{
			name: "with an argument after the slug",
			args: []string{"demo-2", "now", "--brief", "brief.md"},
			want: "unexpected argument \"now\"\n",
		},
		{
			name: "without a brief", args: []string{"demo-2"},
			want: "want --brief <file>: the brief for the next role\n",
		},
		{
			name: "with a blank brief", args: []string{"demo-2", "--brief", "blank.md"},
			want: "brief " + blank + " is blank: it's the next role's work\n",
		},
		{
			name: "with a missing brief", args: []string{"demo-2", "--brief", "missing.md"},
			want: "read the brief: open " + filepath.Join(linked, "missing.md") +
				": no such file or directory\n",
		},
		{
			name: "in a role's pane", args: []string{"demo-2", "--brief", "brief.md"}, role: "lead",
			want: "only the orchestrator starts a feature, and BATON_ROLE names the lead\n",
		},
		{
			name: "without its own program", args: []string{"demo-2", "--brief", "brief.md"},
			self: errors.New("no such program"),
			want: "find baton's own program: no such program\n",
		},
		{
			name: "outside a git repo", dir: outside, args: []string{"demo-2", "--brief", brief},
			want: "find the git repo: fatal: not a git repository",
		},
		{
			name: "with a bad slug", args: []string{"Demo-2", "--brief", "brief.md"},
			want: "slug \"Demo-2\": want words of lowercase letters and digits joined by " +
				"hyphens, such as demo-1\n",
		},
		{
			name: "with a taken slug", args: []string{slug, "--brief", "brief.md"},
			want: "feature demo-1 exists\n",
		},
	}

	files, repo := snapshot(t, store.Dir), repoState(t, mainDir)

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			fake := &fakeTmux{}
			env := startIn(cmp.Or(test.dir, linked), fake, test.args...)
			env.Getenv = func(name string) string {
				return map[string]string{"BATON_ROLE": test.role}[name]
			}

			if test.self != nil {
				env.Executable = func() (string, error) { return "", test.self }
			}

			got := runEnv(t.Context(), env)

			want := "baton feature start: " + test.want
			if got.code != 1 || got.stdout != "" || !strings.HasPrefix(got.stderr, want) {
				t.Errorf("baton feature start = %+v, want code 1 and stderr %q", got, want)
			}

			if len(fake.sessions) > 0 {
				t.Errorf("tmux sessions: %+v; want none", fake.sessions)
			}

			if after := snapshot(t, store.Dir); !maps.Equal(after, files) {
				t.Errorf("baton's folder holds %q; want it unchanged: %q", after, files)
			}

			wantSameRepo(t, mainDir, repo)
		})
	}
}
