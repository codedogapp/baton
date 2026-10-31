package repo_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/codedogapp/baton/internal/repo"
)

func TestWorktreeAddUndoesAWorktreeThatAHookFails(t *testing.T) {
	mainDir, _ := repos(t)
	hooks := script(t, "post-checkout", "echo 'no worktrees here' >&2\nexit 1\n")
	run(t, mainDir, "config", "core.hooksPath", hooks)
	// HEAD hasn't merged the base, so only a forced delete deletes the branch.
	ahead := run(t, mainDir, "commit-tree", "-p", "HEAD", "-m", "Ahead", "HEAD^{tree}")

	path := filepath.Join(filepath.Dir(mainDir), "main-demo-1")
	before := state(t, mainDir, path)

	err := repo.Worktree{Path: path, Branch: "demo-1", Base: ahead}.Add(t.Context(), mainDir)
	if want := "make the worktree " + path + ": no worktrees here"; fmt.Sprint(err) != want {
		t.Errorf("Add = %v; want %s", err, want)
	}

	if after := state(t, mainDir, path); after != before {
		t.Errorf("Add changed\n%s\nto\n%s", before, after)
	}
}

func TestWorktreeAddDeletesTheBranchWhenGitMakesNoFolder(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can make a folder in a read-only one")
	}

	mainDir, _ := repos(t)

	locked := filepath.Join(t.TempDir(), "locked")
	if err := os.Mkdir(locked, 0o500); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(locked, "main-demo-1")
	before := state(t, mainDir, path)

	err := repo.Worktree{Path: path, Branch: "demo-1", Base: "HEAD"}.Add(t.Context(), mainDir)
	if want := fmt.Sprintf("make the worktree %[1]s: fatal: could not create leading "+
		"directories of '%[1]s/.git': Permission denied", path); fmt.Sprint(err) != want {
		t.Errorf("Add = %v; want %s", err, want)
	}

	if after := state(t, mainDir, path); after != before {
		t.Errorf("Add changed\n%s\nto\n%s", before, after)
	}
}

// TestWorktreeAddUndoesAKilledCheckout kills git while it checks out, which leaves the worktree
// locked (fact 23).
func TestWorktreeAddUndoesAKilledCheckout(t *testing.T) {
	mainDir, _ := repos(t)
	holdUp(t, mainDir, "kill -KILL $add\n")

	path := filepath.Join(filepath.Dir(mainDir), "main-demo-1")
	before := state(t, mainDir, path)

	err := repo.Worktree{Path: path, Branch: "demo-1", Base: "HEAD"}.Add(t.Context(), mainDir)
	if want := "make the worktree " + path + ": signal: killed"; fmt.Sprint(err) != want {
		t.Errorf("Add = %v; want %s", err, want)
	}

	if after := state(t, mainDir, path); after != before {
		t.Errorf("Add changed\n%s\nto\n%s", before, after)
	}
}

// TestWorktreeAddReportsWhatItCannotUndo runs a stand-in git that fails to make the worktree, to
// remove it and to delete the branch.
func TestWorktreeAddReportsWhatItCannotUndo(t *testing.T) {
	path := filepath.Join(t.TempDir(), "main-demo-1")
	fake := script(t, "git", fmt.Sprintf(`case "$1 $2" in
"branch --no-track") ;;
"worktree add") mkdir %q; echo 'no worktrees here' >&2; exit 1 ;;
"worktree remove") echo 'cannot remove it' >&2; exit 1 ;;
*) echo 'cannot delete it' >&2; exit 1 ;;
esac
`, path))
	t.Setenv("PATH", fake+string(os.PathListSeparator)+os.Getenv("PATH"))

	err := repo.Worktree{Path: path, Branch: "demo-1", Base: "HEAD"}.Add(t.Context(), t.TempDir())

	want := "make the worktree " + path + ": no worktrees here\ncannot remove it\ncannot delete it"
	if got := fmt.Sprint(err); got != want {
		t.Errorf("Add = %q; want %q", got, want)
	}
}
