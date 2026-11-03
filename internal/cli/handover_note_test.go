package cli_test

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/codedogapp/baton/internal/flow"
)

// TestHandoverRefusesANoteInBatonsFolder hands over a note that lies in baton's folder, where the
// live run's reviewer wrote its own (fact 29), by each kind of path, from a role's pane and from
// the orchestrator's. baton refuses each one, says where the note goes, and changes nothing.
func TestHandoverRefusesANoteInBatonsFolder(t *testing.T) {
	mainDir, linked := repos(t)
	store := started(t, mainDir, fresh())

	name := "02-lead-to-engineer.md"
	stray := write(t, filepath.Join(store.Dir, "features", slug, "notes"), name, "Done.\n")
	links := t.TempDir()
	fileLink := link(t, stray, filepath.Join(links, "note.md"))
	throughLink := filepath.Join(link(t, store.Dir, filepath.Join(links, "baton")), "features",
		slug, "notes", name)
	before := snapshot(t, store.Dir)

	lead := paneOf(flow.RoleLead)
	cases := []struct {
		name string
		dir  string
		env  map[string]string
		args []string // Before --note: the orchestrator's feature and next role.
		note string   // After --note.
		path string   // The note's path in the reason.
	}{
		{"by its path", linked, lead, nil, stray, stray},
		{
			"by a path from the working folder", linked, lead, nil,
			filepath.Join("..", "main", ".git", "baton", "features", slug, "notes", name), stray,
		},
		{"through a link to it", linked, lead, nil, fileLink, fileLink},
		{"through a link to baton's folder", linked, lead, nil, throughLink, throughLink},
		{
			"from the orchestrator's pane", mainDir,
			map[string]string{"BATON_ROLE": string(flow.RoleOrchestrator)},
			[]string{slug, "--to", "lead"},
			stray, stray,
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			args := slices.Concat([]string{"handover"}, test.args, []string{"--note", test.note})
			got := run(t, test.dir, test.env, args...)

			want := result{code: 1, stderr: "baton handover: " +
				inBatonsFolder("note", test.path, store.Dir)}
			if got != want {
				t.Errorf("got %+v; want %+v", got, want)
			}

			if after := snapshot(t, store.Dir); !maps.Equal(after, before) {
				t.Errorf("baton's folder changed to %v", after)
			}
		})
	}
}

// TestHandoverRefusesANoteByAPathFromItsOwnFolder runs baton as main does, in the process's own
// working folder, and hands over a note in baton's folder by a relative path from there.
func TestHandoverRefusesANoteByAPathFromItsOwnFolder(t *testing.T) {
	mainDir, linked := repos(t)
	store := started(t, mainDir, fresh())
	stray := write(t, filepath.Join(store.Dir, "features", slug, "notes"), "02-lead-to-engineer.md",
		"Done.\n")
	t.Chdir(linked)

	note := filepath.Join("..", "main", ".git", "baton", "features", slug, "notes",
		"02-lead-to-engineer.md")
	got := run(t, "", paneOf(flow.RoleLead), "handover", "--note", note)

	want := result{code: 1, stderr: "baton handover: " + inBatonsFolder("note", stray, store.Dir)}
	if got != want {
		t.Errorf("got %+v; want %+v", got, want)
	}
}

// TestHandoverResolvesBatonsFolder makes baton's folder a link to a folder elsewhere, and hands
// over a note there, by the path that the link leads to.
func TestHandoverResolvesBatonsFolder(t *testing.T) {
	mainDir, linked := repos(t)
	elsewhere := t.TempDir()
	batonDir := link(t, elsewhere, filepath.Join(mainDir, ".git", "baton"))
	started(t, mainDir, fresh())
	note := write(t, elsewhere, "note.md", "Done.\n")

	got := run(t, linked, paneOf(flow.RoleLead), "handover", "--note", note)

	want := result{code: 1, stderr: "baton handover: " + inBatonsFolder("note", note, batonDir)}
	if got != want {
		t.Errorf("got %+v; want %+v", got, want)
	}
}

// TestHandoverTakesANoteBesideBatonsFolder hands over a note in the git folder, in a file whose
// name starts with the name of baton's folder, but which lies outside it.
func TestHandoverTakesANoteBesideBatonsFolder(t *testing.T) {
	mainDir, linked := repos(t)
	store := started(t, mainDir, fresh())
	note := write(t, filepath.Dir(store.Dir), "baton.md", "From the lead.\n")

	got := run(t, linked, paneOf(flow.RoleLead), "handover", "--note", note)

	want := result{stdout: "Handed demo-1 over to engineer: running, round 1 of 3.\n"}
	if got != want {
		t.Errorf("got %+v; want %+v", got, want)
	}
}

// inBatonsFolder returns why baton refuses the note or the brief, by its flag's name, at the path
// in baton's folder.
func inBatonsFolder(name, path, batonDir string) string {
	return name + " " + path + " is in baton's folder " + batonDir +
		", which only baton writes: move it out, to your session's files/ folder\n"
}

// link makes a symlink with the name to the target, and returns the name.
func link(t *testing.T, target, name string) string {
	t.Helper()

	if err := os.Symlink(target, name); err != nil {
		t.Fatal(err)
	}

	return name
}
