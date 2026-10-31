package repo_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/codedogapp/baton/internal/repo"
)

// holdUp makes the repo check f out through a filter that runs the shell text first. git worktree
// add checks a worktree out in a child, git reset --hard, which runs the filter; the text finds
// the PID of git worktree add in $add, and the PID of its parent's parent from grandparent
// (fact 23).
func holdUp(t *testing.T, mainDir, text string) {
	t.Helper()

	filters := script(t, "hold", "grandparent() { ps -o ppid= -p $PPID | tr -d ' '; }\n"+
		"add=$(grandparent)\n"+text+"cat\n")
	run(t, mainDir, "config", "filter.hold.smudge", filepath.Join(filters, "hold"))

	for name, text := range map[string]string{".gitattributes": "f filter=hold\n", "f": "hello\n"} {
		err := os.WriteFile(filepath.Join(mainDir, name), []byte(text), 0o600)
		if err != nil {
			t.Fatal(err)
		}
	}

	run(t, mainDir, "add", ".gitattributes", "f")
	run(t, mainDir, "commit", "--quiet", "--message", "Filter")
}

// once runs the action with the name of a file in the folder once there is one, unless the
// context is done first. It returns a function that waits for it.
func once(ctx context.Context, dir string, action func(name string)) (wait func()) {
	var watch sync.WaitGroup

	watch.Go(func() {
		for ctx.Err() == nil {
			if files, err := os.ReadDir(dir); err == nil && len(files) > 0 {
				action(files[0].Name())

				return
			}

			time.Sleep(10 * time.Millisecond)
		}
	})

	return watch.Wait
}

// TestWorktreeAddUndoesACheckoutStoppedByCtrlC stops Add while a filter holds up the checkout, and
// interrupts git as well, as Ctrl-C does: git cleans up after itself while Add stops.
func TestWorktreeAddUndoesACheckoutStoppedByCtrlC(t *testing.T) {
	mainDir, _ := repos(t)
	checkingOut := t.TempDir()

	// The filter holds up the checkout until git worktree add has ended and left it to another
	// parent (fact 23). It prints first: Add reports the stop, not what git printed.
	holdUp(t, mainDir, fmt.Sprintf(`echo 'holding up the checkout' >&2
touch %q/$add
i=0
while [ "$(grandparent)" = "$add" ] && [ $i -lt 200 ]; do sleep 0.05; i=$((i + 1)); done
`, checkingOut))

	path := filepath.Join(filepath.Dir(mainDir), "main-demo-1")
	before := state(t, mainDir, path)
	errStop := errors.New("stopped")
	ctx, stop := context.WithCancelCause(t.Context())

	wait := once(ctx, checkingOut, func(add string) {
		pid, err := strconv.Atoi(add)
		if err == nil {
			err = syscall.Kill(pid, syscall.SIGINT)
		}

		if err != nil {
			t.Error(err)
		}

		stop(errStop)
	})

	err := repo.Worktree{Path: path, Branch: "demo-1", Base: "HEAD"}.Add(ctx, mainDir)

	stop(nil)
	wait()

	if want := "make the worktree " + path + ": stopped"; !errors.Is(err, errStop) ||
		fmt.Sprint(err) != want {
		t.Errorf("Add = %v; want %s", err, want)
	}

	if after := state(t, mainDir, path); after != before {
		t.Errorf("Add changed\n%s\nto\n%s", before, after)
	}
}

// TestWorktreeAddInterruptsGitWhenStopped stops Add while a stand-in git makes the worktree.
// Killed instead, a git that Ctrl-C had set cleaning up would leave its worktree half removed
// (fact 23).
func TestWorktreeAddInterruptsGitWhenStopped(t *testing.T) {
	dir, adding := t.TempDir(), t.TempDir()
	interrupted := filepath.Join(dir, "interrupted")
	fake := script(t, "git", fmt.Sprintf(`[ "$1 $2" = "worktree add" ] || exit 0
trap 'touch %q; exit 1' INT
touch %q/git
i=0
while [ $i -lt 200 ]; do sleep 0.05; i=$((i + 1)); done
exit 1
`, interrupted, adding))
	t.Setenv("PATH", fake+string(os.PathListSeparator)+os.Getenv("PATH"))

	errStop := errors.New("stopped")
	ctx, stop := context.WithCancelCause(t.Context())
	wait := once(ctx, adding, func(string) { stop(errStop) })

	path := filepath.Join(dir, "main-demo-1")
	err := repo.Worktree{Path: path, Branch: "demo-1", Base: "HEAD"}.Add(ctx, dir)

	stop(nil)
	wait()

	if want := "make the worktree " + path + ": stopped"; fmt.Sprint(err) != want {
		t.Errorf("Add = %v; want %s", err, want)
	}

	if _, err := os.Stat(interrupted); err != nil {
		t.Errorf("git wasn't interrupted: %v", err)
	}
}
