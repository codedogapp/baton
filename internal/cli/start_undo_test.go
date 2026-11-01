package cli_test

import (
	"context"
	"errors"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestStartFeatureUndoesWhatItMade starts feature demo-1 where a part of it fails, and checks that
// baton undoes the parts that it made: git lists the worktrees and branches that it listed before,
// and baton's folder holds only the feature's lock, which stays (decision 40).
func TestStartFeatureUndoesWhatItMade(t *testing.T) {
	cases := []struct {
		name   string
		branch bool                                // Whether the feature's branch exists before.
		making func(stop context.CancelFunc) error // What tmux does, if anything.
		want   string                              // The reason, after "baton feature start: ".
	}{
		{
			name: "when the branch exists", branch: true,
			want: "make the branch feature/demo-1: fatal: a branch named 'feature/demo-1' " +
				"already exists\n",
		},
		{
			name:   "when tmux fails",
			making: func(context.CancelFunc) error { return errors.New("duplicate session") },
			want:   "duplicate session\n",
		},
		{
			name: "when stopped while tmux makes the session",
			making: func(stop context.CancelFunc) error {
				stop()

				return context.Canceled
			},
			want: "context canceled\n",
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			mainDir := repoNamed(t, "main")
			brief := write(t, t.TempDir(), "brief.md", "Build the demo.\n")

			if test.branch {
				gitIn(t, mainDir, exec.CommandContext(t.Context(), "git", "branch",
					"feature/demo-1"))
			}

			ctx, stop := context.WithCancel(t.Context())
			defer stop()

			fake := &fakeTmux{}
			if test.making != nil {
				fake.making = func(context.Context) error { return test.making(stop) }
			}

			repo := repoState(t, mainDir)
			got := runEnv(ctx, startIn(mainDir, fake, slug, "--brief", brief))

			if want := (result{code: 1, stderr: "baton feature start: " + test.want}); got != want {
				t.Errorf("baton feature start = %+v, want %+v", got, want)
			}

			wantSameRepo(t, mainDir, repo)

			files := snapshot(t, filepath.Join(mainDir, ".git", "baton"))
			if want := map[string]string{"features/demo-1/lock": ""}; !maps.Equal(files, want) {
				t.Errorf("baton's folder holds %q; want %q", files, want)
			}
		})
	}
}

// TestStartFeatureReportsWhatItCantUndo makes a part of the feature fail, and then its undo, and
// checks that baton reports both.
func TestStartFeatureReportsWhatItCantUndo(t *testing.T) {
	private, readOnly := os.FileMode(0o700), os.FileMode(0o500)

	// locks makes the folders read-only until the test ends.
	locks := func(t *testing.T, folders ...string) error {
		t.Helper()

		for _, folder := range folders {
			t.Cleanup(func() { _ = os.Chmod(folder, private) })

			if err := os.Chmod(folder, readOnly); err != nil {
				return err
			}
		}

		return nil
	}

	t.Run("after the worktree fails", func(t *testing.T) {
		mainDir := repoNamed(t, "main")
		feature := filepath.Join(mainDir, ".git", "baton", "features", slug)
		hooks := filepath.Join(mainDir, ".git", "hooks")
		hook := "#!/bin/sh\nchmod 500 '" + feature + "'\necho 'no checkout here' >&2\nexit 1\n"

		if err := os.MkdirAll(hooks, private); err != nil {
			t.Fatal(err)
		}

		err := os.WriteFile(filepath.Join(hooks, "post-checkout"), []byte(hook), private)
		if err != nil {
			t.Fatal(err)
		}

		t.Cleanup(func() { _ = os.Chmod(feature, private) })

		wantReported(t, mainDir, &fakeTmux{},
			"make the worktree "+mainDir+"-demo-1: no checkout here\n",
			"\nremove feature demo-1: ")
	})

	t.Run("after tmux fails", func(t *testing.T) {
		mainDir := repoNamed(t, "main")
		fake := &fakeTmux{making: func(context.Context) error {
			return errors.Join(errors.New("no server"), locks(t, filepath.Dir(mainDir),
				filepath.Join(mainDir, ".git", "baton", "features", slug)))
		}}

		wantReported(t, mainDir, fake, "no server\n",
			"\nerror: failed to delete '"+mainDir+"-demo-1': ", "\nremove feature demo-1: ")
	})
}

// wantReported starts feature demo-1 in the repo, with the fake as tmux, and checks that baton
// fails and reports each of the reasons, the first one first.
func wantReported(t *testing.T, mainDir string, fake *fakeTmux, reasons ...string) {
	t.Helper()

	brief := write(t, t.TempDir(), "brief.md", "Build the demo.\n")
	got := runEnv(t.Context(), startIn(mainDir, fake, slug, "--brief", brief))

	if got.code != 1 || got.stdout != "" ||
		!strings.HasPrefix(got.stderr, "baton feature start: "+reasons[0]) {
		t.Errorf("baton feature start = %+v, want code 1 and stderr %q", got, reasons[0])
	}

	for _, reason := range reasons[1:] {
		if !strings.Contains(got.stderr, reason) {
			t.Errorf("stderr: %q; want it to hold %q", got.stderr, reason)
		}
	}
}
