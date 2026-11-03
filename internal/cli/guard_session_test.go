package cli_test

import (
	"encoding/json/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGuardHookKeepsTheOrchestratorsWritesInItsSessionsFolder lets the orchestrator's create and
// edit write in its copilot session's folder, in the copilot home folder that COPILOT_HOME names,
// however the path names it, and denies them anywhere else: in the repo, its worktrees, baton's
// folder, or another session's folder (decision 54).
func TestGuardHookKeepsTheOrchestratorsWritesInItsSessionsFolder(t *testing.T) {
	mainDir, linked := repos(t)
	store := started(t, mainDir, fresh())

	home := t.TempDir()
	folder := sessionIn(t, home)

	link := filepath.Join(t.TempDir(), "session")
	if err := os.Symlink(folder, link); err != nil {
		t.Fatal(err)
	}

	other := filepath.Join(home, "session-state", "9f1e7a3c")
	cases := []struct {
		name string
		path string // The path in the tool's arguments.
		want string // The path that the reason names, or "" to let the tool run.
	}{
		{"the session's plan", folder + "/plan.md", ""},
		{"a brief", folder + "/files/brief.md", ""},
		{"a note in a new folder", folder + "/files/demo-1/note.md", ""},
		{"a symlink", link + "/files/brief.md", ""},
		{"the repo", mainDir + "/main.go", mainDir + "/main.go"},
		{"a relative path", "main.go", mainDir + "/main.go"},
		{"a worktree", linked + "/main.go", linked + "/main.go"},
		{"baton's folder", store.Dir + "/x.md", store.Dir + "/x.md"},
		{"another session's folder", other + "/files/x.md", other + "/files/x.md"},
		{"copilot's own file", home + "/config.json", home + "/config.json"},
		{"a path with ..", folder + "/../9f1e7a3c/x.md", other + "/x.md"},
		{"a folder beside it", folder + "-old/x.md", folder + "-old/x.md"},
	}

	pane := map[string]string{"BATON_ROLE": "orchestrator", "COPILOT_HOME": home}

	for _, test := range cases {
		for _, tool := range []string{"create", "edit"} {
			t.Run(tool+" "+test.name, func(t *testing.T) {
				want := ""
				if test.want != "" {
					want = outsideSession(test.want, folder)
				}

				input := guardInput(t, mainDir, tool, map[string]any{"path": test.path})
				wantGuard(t, guardHook(t, mainDir, pane, input), want)
			})
		}
	}
}

// TestGuardHookFindsTheSessionsFolderInTheHomeFolder finds the session's folder in the user's
// home folder's .copilot when the pane has no COPILOT_HOME, as copilot does, and holds nothing in
// a session folder that isn't there.
func TestGuardHookFindsTheSessionsFolderInTheHomeFolder(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	mainDir := repoNamed(t, "main")
	folder := sessionIn(t, filepath.Join(home, ".copilot"))
	brief := guardInput(t, mainDir, "create", map[string]any{"path": folder + "/files/brief.md"})
	inRepo := guardInput(t, mainDir, "create", map[string]any{"path": mainDir + "/x.md"})

	wantGuard(t, guardHook(t, mainDir, orchestratorPane(), brief), "")
	wantGuard(t, guardHook(t, mainDir, orchestratorPane(), inRepo),
		outsideSession(mainDir+"/x.md", folder))

	elsewhere := map[string]string{"BATON_ROLE": "orchestrator", "COPILOT_HOME": t.TempDir()}
	missing := filepath.Join(elsewhere["COPILOT_HOME"], "session-state", session)
	wantGuard(t, guardHook(t, mainDir, elsewhere, brief),
		outsideSession(folder+"/files/brief.md", missing))
}

// TestGuardHookFailsWithoutTheSessionsFolder fails, and so denies the orchestrator's file tool,
// when it can't find the session's folder: the input names no session, or one that isn't a
// folder's name, or the user has no home folder.
func TestGuardHookFailsWithoutTheSessionsFolder(t *testing.T) {
	mainDir := repoNamed(t, "main")
	pane := map[string]string{"BATON_ROLE": "orchestrator", "COPILOT_HOME": t.TempDir()}

	for _, name := range []string{"", ".", "..", "a/b", "/tmp"} {
		t.Run(name, func(t *testing.T) {
			input, err := json.Marshal(map[string]any{
				"sessionId": name, "cwd": mainDir, "toolName": "edit", "toolArgs": map[string]any{
					"path": mainDir + "/x.md",
				},
			})
			if err != nil {
				t.Fatal(err)
			}

			wantFailure(t, guardHook(t, mainDir, pane, string(input)),
				`read the hook's input: sessionId "`+name+`": want a session's ID`)
		})
	}

	t.Setenv("HOME", "")

	input := guardInput(t, mainDir, "create", map[string]any{"path": mainDir + "/x.md"})
	wantFailure(t, guardHook(t, mainDir, orchestratorPane(), input), "find the home folder: ")
}

// TestGuardHookReadsTheSettingsOnlyForAFeatureStart reads the repo's settings only for a script
// that starts a feature, and fails, and so denies it, when it can't: for the config file's error,
// or a folder outside a git repo.
func TestGuardHookReadsTheSettingsOnlyForAFeatureStart(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	mainDir := repoNamed(t, "main")
	outside := t.TempDir()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(outside))
	writeConfig(t, `{"planApproval": "no"}`)

	status := guardInput(t, mainDir, "bash", map[string]any{"command": "git status"})
	wantGuard(t, guardHook(t, mainDir, orchestratorPane(), status), "")

	start := map[string]any{"command": "baton feature start demo-1 --brief /tmp/brief.md"}
	wantFailure(t, guardHook(t, mainDir, orchestratorPane(), guardInput(t, mainDir, "bash", start)),
		"config file ")
	wantFailure(t, guardHook(t, outside, orchestratorPane(), guardInput(t, outside, "bash", start)),
		"find the origin remote: ")
}

// sessionIn makes the folder of the hook input's session, with its files folder, in the copilot
// home folder, and returns it.
func sessionIn(t *testing.T, home string) string {
	t.Helper()

	folder := filepath.Join(home, "session-state", session)
	if err := os.MkdirAll(filepath.Join(folder, "files"), 0o700); err != nil {
		t.Fatal(err)
	}

	return folder
}

// outsideSession returns the reason that denies the orchestrator's write of the path, outside
// the session's folder.
func outsideSession(path, folder string) string {
	return guardStopped + path + " is outside your session's folder " + folder + ": the team " +
		"writes the repo's files, and you write your briefs and notes in its files/ folder."
}

// wantFailure checks that baton hook guard failed with the reason, and so said nothing on stdout,
// which has copilot deny the tool.
func wantFailure(t *testing.T, got result, reason string) {
	t.Helper()

	want := "baton hook guard: " + reason
	if got.code != 1 || got.stdout != "" || !strings.HasPrefix(got.stderr, want) {
		t.Errorf("got %+v; want exit code 1 and %q", got, want)
	}
}
