package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codedogapp/baton/internal/cli"
	"github.com/codedogapp/baton/internal/flow"
	"github.com/codedogapp/baton/internal/plugin"
)

// TestGuardHookKeepsTheFileToolsOutOfBatonsFolder denies a role's create and edit of a file in
// baton's folder, which only baton writes (decision 48), however the path names it: through a
// symlink, with .., from the copilot's folder, in a new folder, or, on a disk that ignores case,
// in other case. It lets them write anywhere else, even beside baton's folder.
func TestGuardHookKeepsTheFileToolsOutOfBatonsFolder(t *testing.T) {
	mainDir, linked := repos(t)
	store := started(t, mainDir, fresh())

	link := filepath.Join(linked, "notes")
	if err := os.Symlink(filepath.Join(store.Dir, "features"), link); err != nil {
		t.Fatal(err)
	}

	// On a disk that ignores case, the path names baton's folder; on another, a folder that isn't
	// there.
	upper, inOtherCase := filepath.Join(mainDir, ".git", "BATON"), ""
	if _, err := os.Stat(upper); err == nil {
		inOtherCase = upper + "/x.md"
	}

	feature := filepath.Join(store.Dir, "features", slug, "feature.json")
	cases := []struct {
		name string
		path string // The path in the tool's arguments.
		want string // The path that the reason names, or "" to let the tool run.
	}{
		{"baton's folder", store.Dir, store.Dir},
		{"a file in it", feature, feature},
		{"a new note", feature + ".md", feature + ".md"},
		{"a file in a new folder", store.Dir + "/a/b/c.md", store.Dir + "/a/b/c.md"},
		{"a path with ..", linked + "/../main/.git/baton/x.md", store.Dir + "/x.md"},
		{"a relative path", "../main/.git/baton/x.md", store.Dir + "/x.md"},
		{"a symlink", link + "/demo-1/feature.json", link + "/demo-1/feature.json"},
		{"a new file behind a symlink", link + "/x.md", link + "/x.md"},
		{"a path in other case", upper + "/x.md", inOtherCase},
		{"the worktree", linked + "/main.go", ""},
		{"git's folder", mainDir + "/.git/config", ""},
		{"a folder beside baton's", mainDir + "/.git/baton-old/x.md", ""},
		{"a session's folder", t.TempDir() + "/files/note.md", ""},
	}

	for _, test := range cases {
		for _, tool := range []string{"create", "edit"} {
			t.Run(tool+" "+test.name, func(t *testing.T) {
				args := map[string]any{"path": test.path, "old_str": "a", "new_str": "b"}

				want := ""
				if test.want != "" {
					want = guardStopped + test.want + " is in baton's folder " + store.Dir +
						", which only baton writes: write your notes in your session's files/ folder."
				}

				input := guardInput(t, linked, tool, args)
				wantGuard(t, guardHook(t, linked, paneOf(flow.RoleReviewer), input), want)
			})
		}
	}
}

// TestGuardHookKeepsTheFileToolsOutOfBatonsCache denies a role's create and edit of a file in
// baton's cache folder, which holds the plugin whose hook runs the guard, however the path names
// it (decision 53). It lets them write beside it.
func TestGuardHookKeepsTheFileToolsOutOfBatonsCache(t *testing.T) {
	_, cache := standIn(t)
	_, linked := repos(t)

	dir, err := plugin.Write(cache)
	if err != nil {
		t.Fatal(err)
	}

	link := filepath.Join(linked, "cache")
	if err := os.Symlink(cache, link); err != nil {
		t.Fatal(err)
	}

	hooks := filepath.Join(dir, hooksFile)
	cases := []struct {
		name string
		path string // The path in the tool's arguments.
		want string // The path that the reason names, or "" to let the tool run.
	}{
		{"the plugin's hooks", hooks, hooks},
		{"baton's cache folder", cache, cache},
		{"a file in a new folder", cache + "/plugin-0/x.md", cache + "/plugin-0/x.md"},
		{"a symlink", link + "/x.md", link + "/x.md"},
		{"a folder beside it", cache + "-old/x.md", ""},
	}

	for _, test := range cases {
		for _, tool := range []string{"create", "edit"} {
			t.Run(tool+" "+test.name, func(t *testing.T) {
				want := ""
				if test.want != "" {
					want = guardStopped + test.want + " is in baton's cache folder " + cache +
						", which only baton writes: it holds the plugin whose hook runs this guard."
				}

				input := guardInput(t, linked, tool, map[string]any{"path": test.path})
				wantGuard(t, guardHook(t, linked, paneOf(flow.RoleEngineer), input), want)
			})
		}
	}
}

// TestGuardHookFailsWithoutTheCacheFolder fails, and so denies a file tool, when it can't find
// the user's cache folder, which holds the plugin.
func TestGuardHookFailsWithoutTheCacheFolder(t *testing.T) {
	_, linked := repos(t)
	t.Setenv("HOME", "")
	t.Setenv("XDG_CACHE_HOME", "")

	input := guardInput(t, linked, "create", map[string]any{"path": linked + "/x.md"})
	got := guardHook(t, linked, paneOf(flow.RoleLead), input)

	want := "baton hook guard: find the cache folder: "
	if got.code != 1 || got.stdout != "" || !strings.HasPrefix(got.stderr, want) {
		t.Errorf("got %+v; want exit code 1 and %q", got, want)
	}
}

// TestGuardHookFindsBatonsFolderFromItsOwnFolder finds baton's folder from the hook's own folder
// when the input names no folder, and a relative path starts there.
func TestGuardHookFindsBatonsFolderFromItsOwnFolder(t *testing.T) {
	mainDir, linked := repos(t)
	store := started(t, mainDir, fresh())

	input := guardInput(t, "", "create", map[string]any{"path": "../main/.git/baton/x.md"})
	wantGuard(
		t,
		guardHook(t, linked, paneOf(flow.RoleLead), input),
		guardStopped+store.Dir+"/x.md is in baton's folder "+
			store.Dir+", which only baton writes: write your notes in your session's files/ folder.",
	)
}

// TestGuardHookLetsTheFileToolsWriteWhereBatonHasNoFolder lets a role's create and edit write in a
// repo without baton's folder, even where the folder would be, since baton keeps nothing there.
func TestGuardHookLetsTheFileToolsWriteWhereBatonHasNoFolder(t *testing.T) {
	other := repoNamed(t, "other")
	input := guardInput(t, other, "create", map[string]any{"path": other + "/.git/baton/x.md"})
	wantGuard(t, guardHook(t, other, paneOf(flow.RoleLead), input), "")
}

// TestGuardHookFailsAndDenies fails, with the reason on stderr and nothing on stdout, when it
// can't tell whether to deny the tool; copilot then denies it.
func TestGuardHookFailsAndDenies(t *testing.T) {
	mainDir, linked := repos(t)
	started(t, mainDir, fresh())

	outside := t.TempDir()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(outside))

	inLead := paneOf(flow.RoleLead)
	push := guardInput(t, linked, "bash", map[string]any{"command": "git push"})
	cases := []struct {
		name  string
		args  []string
		env   map[string]string
		input string
		want  string // The start of the reason, after "baton hook guard: ".
	}{
		{"with an argument", []string{"now"}, inLead, push, `unexpected argument "now"`},
		{"without input", nil, inLead, "", "read the hook's input: EOF"},
		{"with input that isn't JSON", nil, inLead, "bash", "read the hook's input: jsontext: "},
		{"with input that isn't an object", nil, inLead, "[]", "read the hook's input: json: "},
		{
			"outside a pane", nil, nil, push,
			"want BATON_ROLE: run it in a pane that baton started, which names its role there\n",
		},
		{
			"with bash's arguments a string", nil, inLead,
			guardInput(t, linked, "bash", "git push"), "read the arguments of bash: json: ",
		},
		{
			"without bash's arguments", nil, inLead,
			`{"cwd":"` + linked + `","toolName":"bash"}`,
			"read the arguments of bash: jsontext: unexpected EOF",
		},
		{
			"with a path that isn't a string", nil, inLead,
			guardInput(t, linked, "edit", map[string]any{"path": 1}),
			"read the arguments of edit: json: ",
		},
		{
			"outside a git repo", nil, inLead,
			guardInput(t, outside, "create", map[string]any{"path": outside + "/x.md"}),
			"find the git repo: fatal: not a git repository",
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			hook := guardEnv(linked, test.env, test.input)
			hook.Args = append(hook.Args, test.args...)
			got := runEnv(t.Context(), hook)

			want := "baton hook guard: " + test.want
			if got.code != 1 || got.stdout != "" || !strings.HasPrefix(got.stderr, want) {
				t.Errorf("got %+v; want exit code 1 and %q", got, want)
			}
		})
	}
}

// TestGuardHookReportsAnOutputItCantWrite fails when it can't write its answer, which copilot
// then doesn't get, so that copilot denies the tool.
func TestGuardHookReportsAnOutputItCantWrite(t *testing.T) {
	var stderr strings.Builder

	hook := guardEnv(t.TempDir(), paneOf(flow.RoleQA),
		guardInput(t, "", "bash", map[string]any{"command": "git push"}))
	hook.Stdout, hook.Stderr = failing{}, &stderr

	code := cli.Run(t.Context(), hook)
	if got := stderr.String(); code != 1 ||
		got != "baton hook guard: jsontext: write error: output closed\n" {
		t.Errorf("exit code %d and %q; want 1 and the output's error", code, got)
	}
}
