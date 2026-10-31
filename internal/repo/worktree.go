package repo

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// endOfOptions makes git take what follows as names, even one that starts with a dash (fact 23).
const endOfOptions = "--end-of-options"

// Worktree is a feature's worktree to make.
type Worktree struct {
	// Path is the absolute path of the worktree's folder, which mustn't exist yet.
	Path string
	// Branch is the new branch that the worktree checks out.
	Branch string
	// Base is where the branch starts: a branch, a tag or a commit.
	Base string
}

// Add makes the branch at its base, without an upstream, and checks it out in the new worktree. It
// makes both or neither: if the worktree fails, a hook of the user's included, or the context
// stops it, Add removes the worktree and deletes the branch, and reports what it couldn't undo
// (decision 35).
func (wt Worktree) Add(ctx context.Context, dir string) error {
	if err := wt.check(); err != nil {
		return err
	}

	_, err := git(ctx, dir, "branch", "--no-track", endOfOptions, wt.Branch, wt.Base)
	if err != nil {
		return fmt.Errorf("make the branch %s: %w", wt.Branch, err)
	}

	_, err = git(ctx, dir, "worktree", "add", "--quiet", endOfOptions, wt.Path, wt.Branch)
	if err != nil {
		// The undo runs even after a stop, once git's child that checks out has ended: until
		// then, it holds git's stderr (fact 23).
		err = errors.Join(err, wt.Undo(context.WithoutCancel(ctx), dir))

		return fmt.Errorf("make the worktree %s: %w", wt.Path, err)
	}

	return nil
}

// check refuses a path that git would read from another folder, and a path that's taken, even
// by an empty folder, which git would take.
func (wt Worktree) check() error {
	if !filepath.IsAbs(wt.Path) {
		return fmt.Errorf("worktree %q: want an absolute path", wt.Path)
	}

	if _, err := os.Lstat(wt.Path); !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("worktree %s: %w", wt.Path, cmp.Or(err, fs.ErrExist))
	}

	return nil
}

// Undo undoes Add, as Add does when it fails: it removes the worktree, with any changes in it,
// unless git didn't get as far as its folder, and deletes the branch, even if no other branch has
// merged it. It reports what it couldn't undo. A worktree that git was checking out when it
// stopped is locked, and only a second -f (--force) removes it (fact 23).
func (wt Worktree) Undo(ctx context.Context, dir string) error {
	var removed error

	if _, err := os.Lstat(wt.Path); !errors.Is(err, fs.ErrNotExist) {
		_, removed = git(ctx, dir, "worktree", "remove", "-f", "-f", endOfOptions, wt.Path)
	}

	_, deleted := git(ctx, dir, "branch", "-D", endOfOptions, wt.Branch)

	return errors.Join(removed, deleted)
}
