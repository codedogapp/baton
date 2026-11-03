package cli_test

import (
	"os"
	"path/filepath"
	"testing"
	"testing/synctest"

	"github.com/codedogapp/baton/internal/flow"
	"github.com/codedogapp/baton/internal/state"
)

// hooksFile is the plugin's hooks file, in the plugin's folder.
var hooksFile = filepath.Join("com.github.copilot", "hooks", "hooks.json")

// TestPaneChecksThePluginAgainAfterTheWait has the engineer's pane, whose plugin a role changes
// while it waits, start the engineer's copilot with the plugin written again, under the folder's
// next name, and leave the changed folder as it is (decision 53).
func TestPaneChecksThePluginAgainAfterTheWait(t *testing.T) {
	copilot, cache := standIn(t)

	synctest.Test(t, func(t *testing.T) {
		var first string

		got, store := handedOverAfter(t, func() {
			first = pluginIn(t, cache)
			write(t, first, hooksFile, "{}")
		})

		wantCopilot(t, got, flow.RoleEngineer, execed{
			path: copilot,
			args: copilotLine(flow.RoleEngineer, first+"-1",
				"Picked up demo-1 as engineer: round 1 of 3, from lead.\nNote: "+
					notePath(store, "02-lead-to-engineer.md")+"\n"),
			env: copilotEnv(),
		})

		hooks := filepath.Clean(filepath.Join(first, hooksFile))
		if data, err := os.ReadFile(hooks); string(data) != "{}" {
			t.Errorf("the changed hooks file holds %q, %v; want it as the role left it", data, err)
		}
	})
}

// TestPaneFailsWhenItCantWriteThePluginAfterTheWait has the engineer's pane, whose cache folder
// a file takes the place of while it waits, fail after it picks up the engineer's work, and start
// no copilot: the work stays picked up, as when copilot fails to start (decision 38).
func TestPaneFailsWhenItCantWriteThePluginAfterTheWait(t *testing.T) {
	_, cache := standIn(t)

	synctest.Test(t, func(t *testing.T) {
		got, store := handedOverAfter(t, func() { fileFor(t, cache) })

		wantRefused(t, got, "engineer in demo-1: waiting for work.\n",
			"make the cache folder: mkdir "+cache+": not a directory\n")
		wantWork(t, store, state.Work{
			Note: "02-lead-to-engineer.md", From: flow.RoleLead, Picked: true,
		})
	})
}

// handedOverAfter starts the engineer's pane, which waits for its work, and makes the change while
// it waits; the lead then picks up its own work and hands over to the engineer. It returns what
// the engineer's pane did, and the feature's store.
func handedOverAfter(t *testing.T, change func()) (ran, state.Store) {
	t.Helper()

	mainDir, linked := repos(t)
	store := started(t, mainDir, fresh())

	env, _ := inPane(t, linked, paneOf(flow.RoleEngineer))
	engineer := make(chan ran, 1)

	go func() { engineer <- paneFor(t, env) }()

	synctest.Wait()
	change()

	if got := waitAs(t, mainDir, flow.RoleLead); got.code != 0 {
		t.Fatalf("the lead's wait: %+v", got)
	}

	handOver(t, mainDir, flow.RoleLead, flow.VerdictNone)

	return <-engineer, store
}
