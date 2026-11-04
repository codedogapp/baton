package cli_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/codedogapp/baton/internal/flow"
)

// TestGuardHookKeepsTheFileToolsOutOfBatonsConfig denies a role's create and edit of a file in
// baton's config folder, whose file holds the gate that the role's handovers run, however the path
// names it (decision 56). It lets them write beside it.
func TestGuardHookKeepsTheFileToolsOutOfBatonsConfig(t *testing.T) {
	withGate(t, "true")
	_, linked := repos(t)

	file := configPath(t)
	folder := filepath.Dir(file)

	link := filepath.Join(linked, "config")
	if err := os.Symlink(folder, link); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		path string // The path in the tool's arguments.
		want string // The path that the reason names, or "" to let the tool run.
	}{
		{"the config file", file, file},
		{"baton's config folder", folder, folder},
		{"a file in a new folder", folder + "/old/config.json", folder + "/old/config.json"},
		{"a symlink", link + "/config.json", link + "/config.json"},
		{"a folder beside it", folder + "-old/config.json", ""},
	}

	for _, test := range cases {
		for _, tool := range []string{"create", "edit"} {
			t.Run(tool+" "+test.name, func(t *testing.T) {
				want := ""
				if test.want != "" {
					want = guardStopped + test.want + " is in baton's config folder " + folder +
						", which only the user writes: its file holds the gate that your " +
						"handovers run."
				}

				input := guardInput(t, linked, tool, map[string]any{"path": test.path})
				wantGuard(t, guardHook(t, linked, paneOf(flow.RoleEngineer), input), want)
			})
		}
	}
}
