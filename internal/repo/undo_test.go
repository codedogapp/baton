package repo_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

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

// TestWorktreeAddUndoesAStoppedCheckout stops Add while a filter holds up the checkout.
func TestWorktreeAddUndoesAStoppedCheckout(t *testing.T) {
	mainDir, _ := repos(t)
	marker := filepath.Join(t.TempDir(), "checking-out")

	// The filter holds up git reset, which checks the worktree out, until git worktree add, which
	// runs it, has ended and left it to another parent (fact 23). It prints first: Add reports the
	// stop, not what git printed.
	filters := script(t, "slow", fmt.Sprintf(`up=$(ps -o ppid= -p $PPID)
echo 'holding up the checkout' >&2
touch %q
i=0
while [ "$(ps -o ppid= -p $PPID)" = "$up" ] && [ $i -lt 200 ]; do sleep 0.05; i=$((i + 1)); done
cat
`, marker))
	run(t, mainDir, "config", "filter.slow.smudge", filepath.Join(filters, "slow"))

	for name, text := range map[string]string{".gitattributes": "f filter=slow\n", "f": "hello\n"} {
		err := os.WriteFile(filepath.Join(mainDir, name), []byte(text), 0o600)
		if err != nil {
			t.Fatal(err)
		}
	}

	run(t, mainDir, "add", ".gitattributes", "f")
	run(t, mainDir, "commit", "--quiet", "--message", "Filter")

	path := filepath.Join(filepath.Dir(mainDir), "main-demo-1")
	before := state(t, mainDir, path)
	errStop := errors.New("stopped")
	ctx, stop := context.WithCancelCause(t.Context())

	var checkout sync.WaitGroup

	checkout.Go(func() {
		for ctx.Err() == nil {
			if _, err := os.Stat(marker); err == nil {
				stop(errStop)
			}

			time.Sleep(10 * time.Millisecond)
		}
	})

	err := repo.Worktree{Path: path, Branch: "demo-1", Base: "HEAD"}.Add(ctx, mainDir)

	stop(nil)
	checkout.Wait()

	if want := "make the worktree " + path + ": stopped"; !errors.Is(err, errStop) ||
		fmt.Sprint(err) != want {
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
