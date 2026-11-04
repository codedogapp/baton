package repo_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/codedogapp/baton/internal/repo"
)

func TestPushSendsTheBranchToOriginAndTracksIt(t *testing.T) {
	mainDir, linked := repos(t)
	origin := filepath.Join(t.TempDir(), "origin.git")
	run(t, mainDir, "init", "--quiet", "--bare", origin)
	run(t, mainDir, "remote", "add", "origin", origin)
	run(t, linked, "commit", "--quiet", "--allow-empty", "--message", "Work")

	if err := repo.Push(t.Context(), mainDir, "linked"); err != nil {
		t.Fatal(err)
	}

	got, want := run(t, origin, "rev-parse", "linked"), run(t, linked, "rev-parse", "HEAD")
	if got != want {
		t.Errorf("origin's linked is at %s, want the branch's %s", got, want)
	}

	if got := run(t, linked, "rev-parse", "--abbrev-ref", "@{upstream}"); got != "origin/linked" {
		t.Errorf("the branch's upstream is %q, want origin/linked", got)
	}
}

func TestPushRefusesARepoWithoutOrigin(t *testing.T) {
	mainDir, _ := repos(t)

	err := repo.Push(t.Context(), mainDir, "linked")
	if want := "push linked to origin: fatal: 'origin' does not appear to be a git repository"; err == nil ||
		!strings.HasPrefix(err.Error(), want) {
		t.Errorf("Push = %v; want %q, then the rest of git's reason", err, want)
	}
}
