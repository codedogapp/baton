package repo_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codedogapp/baton/internal/repo"
)

// TestRemoteIsOriginsURL finds the origin remote's URL from every worktree of a repo with two
// remotes, as git gives it: with the repo's insteadOf rules applied.
func TestRemoteIsOriginsURL(t *testing.T) {
	cases := []struct {
		name   string
		config []string // git config's arguments, if any, in the main worktree.
		url    string   // origin's URL in the repo's config.
		want   string
	}{
		{
			name: "as it is",
			url:  "git@github.com:codedogapp/baton.git",
			want: "git@github.com:codedogapp/baton.git",
		},
		{
			name:   "rewritten",
			config: []string{"url.https://github.com/.insteadOf", "gh:"},
			url:    "gh:codedogapp/baton",
			want:   "https://github.com/codedogapp/baton",
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			mainDir, linked := repos(t)
			run(t, mainDir, "remote", "add", "upstream", "https://example.com/upstream.git")
			run(t, mainDir, "remote", "add", "origin", test.url)

			if test.config != nil {
				run(t, mainDir, append([]string{"config"}, test.config...)...)
			}

			sub := filepath.Join(linked, "docs")
			if err := os.Mkdir(sub, 0o700); err != nil {
				t.Fatal(err)
			}

			for _, dir := range []string{mainDir, linked, sub} {
				got, err := repo.Remote(t.Context(), dir)
				if err != nil || got != test.want {
					t.Errorf("Remote(%s) = %q, %v; want %q", dir, got, err, test.want)
				}
			}
		})
	}
}

func TestRemoteIsEmptyWithoutOrigin(t *testing.T) {
	mainDir, _ := repos(t)
	run(t, mainDir, "remote", "add", "upstream", "https://example.com/upstream.git")

	if got, err := repo.Remote(t.Context(), mainDir); got != "" || err != nil {
		t.Errorf("Remote = %q, %v; want no URL and no error", got, err)
	}
}

func TestRemoteRefusesAFolderOutsideARepo(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))

	got, err := repo.Remote(t.Context(), dir)
	if want := "find the origin remote: fatal: not a git repository"; got != "" || err == nil ||
		!strings.HasPrefix(err.Error(), want) {
		t.Errorf("Remote = %q, %v; want %q, then the rest of git's reason", got, err, want)
	}
}
