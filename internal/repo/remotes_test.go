package repo_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/codedogapp/baton/internal/repo"
)

func TestRemoteURLsAreAsTheConfigHoldsThem(t *testing.T) {
	mainDir, linked := repos(t)
	run(t, mainDir, "config", "url.https://example.invalid/.insteadOf", "demo:")
	run(t, mainDir, "remote", "add", "origin", "demo:origin.git")
	run(t, mainDir, "remote", "set-url", "--add", "--push", "origin", "git@example.invalid:one.git")
	run(t, mainDir, "remote", "set-url", "--add", "--push", "origin", "git@example.invalid:two.git")
	run(t, mainDir, "remote", "add", "My.Fork", "../fork.git")

	want := []repo.RemoteURL{
		{Remote: "origin", URL: "demo:origin.git"},
		{Remote: "origin", URL: "git@example.invalid:one.git", Push: true},
		{Remote: "origin", URL: "git@example.invalid:two.git", Push: true},
		{Remote: "My.Fork", URL: "../fork.git"},
	}

	for _, dir := range []string{mainDir, linked} {
		got, err := repo.RemoteURLs(t.Context(), dir)
		if err != nil || !slices.Equal(got, want) {
			t.Errorf("RemoteURLs(%s) = %+v, %v; want %+v", dir, got, err, want)
		}
	}
}

func TestRemoteURLsAreNoneWithoutRemotes(t *testing.T) {
	mainDir, _ := repos(t)

	if got, err := repo.RemoteURLs(t.Context(), mainDir); got != nil || err != nil {
		t.Errorf("RemoteURLs = %+v, %v; want none", got, err)
	}
}

func TestRemoteURLsRefuseABrokenConfig(t *testing.T) {
	mainDir, _ := repos(t)
	run(t, mainDir, "config", "include.path", "broken")

	err := os.WriteFile(filepath.Join(mainDir, ".git", "broken"), []byte("[remote\n"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	_, err = repo.RemoteURLs(t.Context(), mainDir)
	if want := "read the remotes' URLs: fatal: bad config line 1"; err == nil ||
		!strings.HasPrefix(err.Error(), want) {
		t.Errorf("RemoteURLs = %v; want %q, then the rest of git's reason", err, want)
	}
}

func TestRemoteURLsReportAGitThatFailsWithoutAReason(t *testing.T) {
	t.Setenv("PATH", script(t, "git", "exit 3\n"))

	_, err := repo.RemoteURLs(t.Context(), t.TempDir())
	if err == nil || err.Error() != "read the remotes' URLs: exit status 3" {
		t.Errorf("RemoteURLs = %v; want git's exit status", err)
	}
}

func TestClearsPushURLsFromGit246(t *testing.T) {
	cases := []struct {
		version string
		want    bool
	}{
		{version: "git version 2.45.2", want: false},
		{version: "git version 2.46.0", want: true},
		{version: "git version 2.50.1 (Apple Git-155)", want: true},
		{version: "git version 3.0.0", want: true},
		{version: "git version 1.99.9", want: false},
		{version: "git version 2.9.5", want: false},
		{version: "git version 3", want: false},
		{version: "nonsense", want: false},
	}

	for _, test := range cases {
		t.Run(test.version, func(t *testing.T) {
			t.Setenv("PATH", script(t, "git", "echo '"+test.version+"'\n"))

			if got, err := repo.ClearsPushURLs(t.Context()); got != test.want || err != nil {
				t.Errorf("ClearsPushURLs = %t, %v; want %t", got, err, test.want)
			}
		})
	}
}

func TestClearsPushURLsReportsAGitThatFails(t *testing.T) {
	t.Setenv("PATH", script(t, "git", "exit 3\n"))

	_, err := repo.ClearsPushURLs(t.Context())
	if err == nil || err.Error() != "find git's version: exit status 3" {
		t.Errorf("ClearsPushURLs = %v; want git's exit status", err)
	}
}
