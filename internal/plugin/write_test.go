package plugin

import (
	"bytes"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
)

func TestWriteWritesThePlugin(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "baton")

	dir, err := Write(cache)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	if filepath.Dir(dir) != cache ||
		!regexp.MustCompile(`^plugin-[0-9a-f]{16}$`).MatchString(filepath.Base(dir)) {
		t.Errorf("Write = %s, want %s/plugin-<16 hex digits>", dir, cache)
	}

	info, err := os.Stat(cache)
	if err != nil {
		t.Fatal(err)
	}

	if info.Mode().Perm() != 0o700 {
		t.Errorf("the cache folder's mode is %v, want 0700: only the user's", info.Mode().Perm())
	}

	same(t, dir, thePlugin(t))
	only(t, cache, filepath.Base(dir))
}

func TestWriteKeepsTheFolderThatHoldsThePlugin(t *testing.T) {
	cache := t.TempDir()
	first := take(Write(cache))(t)

	before, err := os.Stat(first)
	if err != nil {
		t.Fatal(err)
	}

	if again := take(Write(cache))(t); again != first {
		t.Errorf("the second Write = %s, want the first's, %s", again, first)
	}

	if after, err := os.Stat(first); err != nil || !os.SameFile(after, before) {
		t.Errorf("the second Write replaced the folder: %v", err)
	}

	same(t, first, thePlugin(t))
	only(t, cache, filepath.Base(first))
}

func TestWriteByManyPanesAtOnce(t *testing.T) {
	cache := t.TempDir()
	dir := atOnce(t, cache)

	same(t, dir, thePlugin(t))
	only(t, cache, filepath.Base(dir))
}

func TestWriteNamesTheFolderByItsFiles(t *testing.T) {
	cache := t.TempDir()

	first := take(write(cache, tree("a.md", "one")))(t)
	same(t, first, fstest.MapFS{"a.md": {Data: []byte("one")}})

	if again := take(write(cache, tree("a.md", "one")))(t); again != first {
		t.Errorf("the same files went to %s, then to %s", first, again)
	}

	// Each pair has other files, which a hash of their text alone would mix up, since the text
	// would be the same if one of the parts that ends a path, a length or a file were missing.
	for number, pair := range [][2]fstest.MapFS{
		{tree("a.md", "one"), tree("a.md", "two")},                       // Other contents.
		{tree("a.md", "one"), tree("b.md", "one")},                       // Another path.
		{tree("a1", "xx", "b", "zzzzzzz"), tree("a", "xxb7\x00zzzzzzz")}, // A path's end.
		{tree("a", "2", "bcd", "yyyyyyy"), tree("a", "bcd\x007yyyyyyy")}, // A length's end.
		{tree("a", "x", "b", "y"), tree("a", "", "xb", "y")},             // A file's end.
	} {
		if one, other := take(
			write(cache, pair[0]),
		)(
			t,
		), take(
			write(cache, pair[1]),
		)(
			t,
		); one == other {
			t.Errorf("both trees of pair %d went to %s", number, one)
		}
	}
}

// atOnce has many panes write the plugin into the cache at once, and returns the folder that each
// of them got: the same for each.
func atOnce(t *testing.T, cache string) string {
	t.Helper()

	const panes = 8

	dirs, errs := make([]string, panes), make([]error, panes)

	var group sync.WaitGroup
	for pane := range panes {
		group.Go(func() { dirs[pane], errs[pane] = Write(cache) })
	}

	group.Wait()

	for pane := range panes {
		if errs[pane] != nil || dirs[pane] != dirs[0] {
			t.Errorf("Write %d = %s, %v; want %s, nil", pane, dirs[pane], errs[pane], dirs[0])
		}
	}

	return dirs[0]
}

// thePlugin returns the plugin's files, as the binary holds them.
func thePlugin(t *testing.T) fs.FS {
	t.Helper()

	plugin, err := fs.Sub(files, root)
	if err != nil {
		t.Fatal(err)
	}

	return plugin
}

// must fails the test on the error.
func must(t *testing.T, err error) {
	t.Helper()

	if err != nil {
		t.Fatal(err)
	}
}

// tree returns a tree with the files in its root folder, given as pairs of a path and a text.
func tree(pairs ...string) fstest.MapFS {
	files := fstest.MapFS{}
	for pair := range slices.Chunk(pairs, 2) {
		files[root+"/"+pair[0]] = &fstest.MapFile{Data: []byte(pair[1])}
	}

	return files
}

// take returns a test helper that fails the test on the error, or returns the value.
func take(value string, err error) func(t *testing.T) string {
	return func(t *testing.T) string {
		t.Helper()

		if err != nil {
			t.Fatal(err)
		}

		return value
	}
}

// same fails the test unless the folder holds the tree's files, byte for byte, and no others.
func same(t *testing.T, dir string, tree fs.FS) {
	t.Helper()

	if got, want := contents(t, os.DirFS(dir)), contents(t, tree); !maps.EqualFunc(
		got, want, bytes.Equal) {
		t.Errorf("%s holds %v, want %v", dir, slices.Sorted(maps.Keys(got)),
			slices.Sorted(maps.Keys(want)))
	}
}

// contents returns the tree's files and their contents.
func contents(t *testing.T, tree fs.FS) map[string][]byte {
	t.Helper()

	found := map[string][]byte{}

	err := fs.WalkDir(tree, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}

		found[path], err = fs.ReadFile(tree, path)

		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	return found
}

// only fails the test unless the folder holds the names and nothing else.
func only(t *testing.T, dir string, names ...string) {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	got := make([]string, 0, len(entries))
	for _, entry := range entries {
		got = append(got, entry.Name())
	}

	if !slices.Equal(got, names) {
		t.Errorf("%s holds %s, want %s", dir, strings.Join(got, ", "), strings.Join(names, ", "))
	}
}
