package state_test

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/codedogapp/baton/internal/flow"
	"github.com/codedogapp/baton/internal/state"
)

// TestARoleWaitsWhileItsMarkIsHeld reports a role as waiting in its feature, and the orchestrator
// in baton's folder, while a holder of its wait mark hasn't released it. Holders share the mark,
// and a look at it leaves it free.
func TestARoleWaitsWhileItsMarkIsHeld(t *testing.T) {
	marks := map[string]struct {
		waiter waiter
		file   string // The mark's file in baton's folder.
	}{
		"a role": {waiter{slug, flow.RoleQA}, filepath.Join("features", slug, "qa.wait")},
		"the orchestrator, whatever the slug": {
			waiter{"demo-2", flow.RoleOrchestrator}, "orchestrator.wait",
		},
	}

	for name, test := range marks {
		t.Run(name, func(t *testing.T) {
			store := created(t)
			wantWaiting(t, store, test.waiter, false)

			first := mark(t, store, test.waiter)
			wantWaiting(t, store, test.waiter, true)
			wantWaiting(t, store, waiter{slug, flow.RoleLead}, false)
			wantShared(t, filepath.Join(store.Dir, test.file))

			second := mark(t, store, test.waiter)

			first()
			wantWaiting(t, store, test.waiter, true)

			second()
			wantWaiting(t, store, test.waiter, false)
			wantWaiting(t, store, test.waiter, false)
		})
	}
}

func TestNoRoleWaitsWithoutItsMark(t *testing.T) {
	stores := map[string]state.Store{
		"a feature's folder without marks": created(t),
		"no feature's folder":              {Dir: t.TempDir()},
		"no baton folder":                  {Dir: filepath.Join(t.TempDir(), "baton")},
	}

	for name, store := range stores {
		for _, role := range []flow.Role{flow.RoleQA, flow.RoleOrchestrator} {
			if waiting, err := store.Waiting(slug, role); waiting || err != nil {
				t.Errorf("%s: Waiting(%s) = %t, %v; want false", name, role, waiting, err)
			}
		}
	}
}

// TestRemoveKeepsTheWaitMarks keeps a feature's wait marks, like its lock, so that a role that
// waits while baton removes the feature stays marked as waiting.
func TestRemoveKeepsTheWaitMarks(t *testing.T) {
	store, inQA := created(t), waiter{slug, flow.RoleQA}
	release := mark(t, store, inQA)

	if err := store.Remove(slug); err != nil {
		t.Fatal(err)
	}

	wantFolder(t, store, "lock", "qa.wait")
	wantWaiting(t, store, inQA, true)

	release()
	wantWaiting(t, store, inQA, false)
}

func TestMarkWaitingReportsAMarkItCantTake(t *testing.T) {
	file := filepath.Join(t.TempDir(), "baton")
	writeFile(t, file, "")

	stores := map[string]struct {
		store state.Store
		want  string // The end of the error's message.
	}{
		"no feature's folder": {
			state.Store{
				Dir: t.TempDir(),
			},
			": openat features/demo-1/qa.wait: no such file or directory",
		},
		"no baton folder": {
			state.Store{
				Dir: filepath.Join(t.TempDir(), "baton"),
			},
			"/baton: no such file or directory",
		},
		"a file in baton folder's place": {state.Store{Dir: file}, "/baton: not a directory"},
	}

	for name, test := range stores {
		release, err := test.store.MarkWaiting(slug, flow.RoleQA)
		if err != nil && strings.HasPrefix(err.Error(), "mark the qa waiting: ") &&
			strings.HasSuffix(err.Error(), test.want) {
			continue
		}

		if err == nil {
			release()
		}

		t.Errorf("%s: error = %v, want one that ends with %q", name, err, test.want)
	}

	_, err := state.Store{Dir: file}.Waiting(slug, flow.RoleQA)
	wantError(t, err, "look for the qa's wait: ")
}

// waiter is a role that waits, in the feature with the slug.
type waiter struct {
	slug string
	role flow.Role
}

// mark marks the waiter as waiting in the store, and returns the release.
func mark(t *testing.T, store state.Store, waiter waiter) func() {
	t.Helper()

	release, err := store.MarkWaiting(waiter.slug, waiter.role)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(release)

	return release
}

// wantWaiting fails the test unless the store reports the waiter as waiting, or not, as want
// says.
func wantWaiting(t *testing.T, store state.Store, waiter waiter, want bool) {
	t.Helper()

	if got, err := store.Waiting(waiter.slug, waiter.role); got != want || err != nil {
		t.Errorf("Waiting(%+v) = %t, %v; want %t", waiter, got, err, want)
	}
}

// wantShared fails the test unless another process could take a shared lock on the mark's file,
// as another wait of the role does, while a holder holds it.
func wantShared(t *testing.T, path string) {
	t.Helper()

	file, err := os.Open(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()

	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_SH|syscall.LOCK_NB); err != nil {
		t.Fatalf("%s: %v; want the mark shared", path, err)
	}
}
