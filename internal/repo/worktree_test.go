package repo_test

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/codedogapp/baton/internal/repo"
)

// state returns the repo's branches and worktrees, and what's at the path.
func state(t *testing.T, dir, path string) string {
	t.Helper()

	found := "nothing"
	if info, err := os.Lstat(path); err == nil {
		found = info.Mode().Type().String()
	}

	return fmt.Sprintf("%s\n\n%s\n\nat %s: %s",
		run(t, dir, "for-each-ref", "--format=%(refname) %(objectname)"),
		run(t, dir, "worktree", "list", "--porcelain"),
		path, found)
}

func TestWorktreeAddMakesTheBranchAndChecksItOut(t *testing.T) {
	mainDir, _ := repos(t)
	run(t, mainDir, "branch", "base")
	run(t, mainDir, "commit", "--quiet", "--allow-empty", "--message", "Next")
	// Without --no-track, the new branch would track base.
	run(t, mainDir, "config", "branch.autoSetupMerge", "always")

	path := filepath.Join(filepath.Dir(mainDir), "main-demo-1")

	err := repo.Worktree{Path: path, Branch: "demo-1", Base: "base"}.Add(t.Context(), mainDir)
	if err != nil {
		t.Fatal(err)
	}

	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}

	base := run(t, mainDir, "rev-parse", "base")
	want := fmt.Sprintf("worktree %s\nHEAD %s\nbranch refs/heads/demo-1", resolved, base)

	listed := strings.Split(run(t, mainDir, "worktree", "list", "--porcelain"), "\n\n")
	if !slices.Contains(listed, want) {
		t.Errorf("the worktrees are %q; want an unlocked one %q", listed, want)
	}

	upstream := run(t, mainDir, "for-each-ref", "--format=%(upstream)", "refs/heads/demo-1")
	if upstream != "" {
		t.Errorf("demo-1 tracks %s; want no upstream", upstream)
	}
}

func TestWorktreeAddRefusesAndChangesNothing(t *testing.T) {
	for _, test := range []struct {
		name, at, branch, base, want string
	}{
		{
			"a relative path", "relative", "demo-1", "HEAD",
			`worktree "main-demo-1": want an absolute path`,
		},
		{"a file at the path", "file", "demo-1", "HEAD", "worktree <path>: file already exists"},
		{
			"an empty folder at the path", "folder", "demo-1", "HEAD",
			"worktree <path>: file already exists",
		},
		{
			"a link to nowhere at the path", "link", "demo-1", "HEAD",
			"worktree <path>: file already exists",
		},
		{
			"a file above the path", "below a file", "demo-1", "HEAD",
			"worktree <path>/demo-1: lstat <path>/demo-1: not a directory",
		},
		{
			"a branch that's taken", "", "taken", "HEAD",
			"make the branch taken: fatal: a branch named 'taken' already exists",
		},
		{
			"an unknown base", "", "demo-1", "nosuch",
			"make the branch demo-1: fatal: not a valid object name: 'nosuch'",
		},
		{
			// Taken for an option, -D would delete the branch.
			"a base like an option", "", "taken", "-D",
			"make the branch taken: fatal: a branch named 'taken' already exists",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			mainDir, _ := repos(t)
			run(t, mainDir, "branch", "taken")

			path := filepath.Join(filepath.Dir(mainDir), "main-demo-1")
			worktree := repo.Worktree{Path: path, Branch: test.branch, Base: test.base}
			take(t, test.at, &worktree)

			before := state(t, mainDir, path)

			err := worktree.Add(t.Context(), mainDir)
			if want := strings.ReplaceAll(test.want, "<path>", path); fmt.Sprint(err) != want {
				t.Errorf("Add = %v; want %s", err, want)
			}

			if after := state(t, mainDir, path); after != before {
				t.Errorf("Add changed\n%s\nto\n%s", before, after)
			}
		})
	}
}

// take puts a file, an empty folder or a link to nowhere at the worktree's path, puts the path below
// a file, or makes the path relative.
func take(t *testing.T, what string, worktree *repo.Worktree) {
	t.Helper()

	var err error

	switch what {
	case "relative":
		worktree.Path = filepath.Base(worktree.Path)
	case "file":
		err = os.WriteFile(worktree.Path, []byte("mine\n"), 0o600)
	case "below a file":
		err = os.WriteFile(worktree.Path, []byte("mine\n"), 0o600)
		worktree.Path = filepath.Join(worktree.Path, "demo-1")
	case "folder":
		err = os.Mkdir(worktree.Path, 0o700)
	case "link":
		err = os.Symlink("nowhere", worktree.Path)
	}

	if err != nil {
		t.Fatal(err)
	}
}
