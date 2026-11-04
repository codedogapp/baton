package repo_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codedogapp/baton/internal/repo"
)

// writeIn writes a file at the path in the folder, with the folders above it, which holds the
// line that ignores a build folder.
func writeIn(t *testing.T, dir, path string) {
	t.Helper()

	file := filepath.Join(dir, path)
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(file, []byte("build/\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestWorktreeRemoveKeepsTheBranch removes a worktree with a file that the repo ignores, which
// goes with it, and one whose folder is gone, of which git only forgets the record (fact 46).
func TestWorktreeRemoveKeepsTheBranch(t *testing.T) {
	mainDir, linked := repos(t)
	writeIn(t, mainDir, ".git/info/exclude")
	writeIn(t, linked, "build/out")

	gone := filepath.Join(filepath.Dir(mainDir), "gone")
	run(t, mainDir, "worktree", "add", "--quiet", gone)

	if err := os.RemoveAll(gone); err != nil {
		t.Fatal(err)
	}

	for _, worktree := range []repo.Worktree{
		{Path: linked, Branch: "linked"}, {Path: gone, Branch: "gone"},
	} {
		if err := worktree.Remove(t.Context(), mainDir); err != nil {
			t.Fatal(err)
		}

		if _, err := os.Lstat(worktree.Path); !os.IsNotExist(err) {
			t.Errorf("Remove left %s: %v", worktree.Path, err)
		}
	}

	listed := run(t, mainDir, "worktree", "list", "--porcelain")
	if strings.Count(listed, "worktree ") != 1 {
		t.Errorf("git lists the worktrees %q; want the main one alone", listed)
	}

	branches := run(t, mainDir, "branch", "--format=%(refname:short)")
	if want := "gone\nlinked\n" + run(t, mainDir, "branch", "--show-current"); branches != want {
		t.Errorf("the branches are %q; want %q", branches, want)
	}
}

func TestWorktreeRemoveRefusesAndChangesNothing(t *testing.T) {
	mainDir, linked := repos(t)
	writeIn(t, linked, "notes.md")

	before := state(t, mainDir, linked)

	err := repo.Worktree{Path: linked, Branch: "linked"}.Remove(t.Context(), mainDir)
	if want := fmt.Sprintf("remove the worktree %[1]s: fatal: '%[1]s' contains modified or "+
		"untracked files, use --force to delete it", linked); fmt.Sprint(err) != want {
		t.Errorf("Remove = %v; want %s", err, want)
	}

	if after := state(t, mainDir, linked); after != before {
		t.Errorf("Remove changed\n%s\nto\n%s", before, after)
	}
}
