package plugin

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func TestWriteRefusals(t *testing.T) {
	plugin := fstest.MapFS{root + "/plugin.json": {Data: []byte("{}")}}

	for _, test := range []struct {
		name  string
		tree  fs.FS
		cache func(t *testing.T, tree fs.FS) string // Makes the cache that write gets.
		want  []string                              // The parts of write's error.
	}{{
		name:  "a tree without the plugin",
		tree:  fstest.MapFS{"other/plugin.json": {Data: []byte("{}")}},
		cache: emptyCache,
		want:  []string{"hash the plugin: open files: file does not exist"},
	}, {
		name:  "a file it can't read",
		tree:  fstest.MapFS{root + "/agent.md": {Mode: fs.ModeSymlink, Data: []byte("gone")}},
		cache: emptyCache,
		want:  []string{"hash the plugin: open files/agent.md: file does not exist"},
	}, {
		name: "a file in the cache's place",
		tree: plugin,
		cache: func(t *testing.T, _ fs.FS) string {
			file := filepath.Join(t.TempDir(), "file")
			put(t, file)

			return filepath.Join(file, "baton")
		},
		want: []string{"make the cache folder: mkdir ", "/file: not a directory"},
	}, {
		name: "a cache it can't write in",
		tree: plugin,
		cache: func(t *testing.T, _ fs.FS) string {
			if os.Geteuid() == 0 {
				t.Skip("root writes in any folder")
			}

			cache := t.TempDir()
			chmod(t, cache, 0o500)
			t.Cleanup(func() { chmod(t, cache, 0o700) })

			return cache
		},
		want: []string{"write the plugin: mkdir ", "/plugin-", ".tmp: permission denied"},
	}, {
		name:  "a file it can't copy",
		tree:  fstest.MapFS{root + "/hook": {Mode: fs.ModeNamedPipe}},
		cache: emptyCache,
		want:  []string{"write the plugin: CopyFS files/hook: invalid argument"},
	}, {
		name: "a file in the folder's place",
		tree: plugin,
		cache: func(t *testing.T, tree fs.FS) string {
			cache := t.TempDir()
			put(t, filepath.Join(cache, filepath.Base(take(write(t.TempDir(), tree))(t))))

			return cache
		},
		want: []string{"write the plugin: rename ", "/plugin-", ": not a directory"},
	}} {
		t.Run(test.name, func(t *testing.T) {
			cache := test.cache(t, test.tree)

			dir, err := write(cache, test.tree)
			refused(t, dir, err, test.want)

			if left, _ := filepath.Glob(filepath.Join(cache, "plugin-*.tmp")); len(left) > 0 {
				t.Errorf("write left %v", left)
			}
		})
	}
}

// refused fails the test unless write failed with an error that holds each part.
func refused(t *testing.T, dir string, err error, parts []string) {
	t.Helper()

	if err == nil {
		t.Fatalf("write = %s, want an error", dir)
	}

	for _, part := range parts {
		if !strings.Contains(err.Error(), part) {
			t.Errorf("write's error = %v, want %q in it", err, part)
		}
	}
}

// emptyCache returns a new, empty cache folder.
func emptyCache(t *testing.T, _ fs.FS) string {
	return t.TempDir()
}

// put makes an empty file.
func put(t *testing.T, path string) {
	t.Helper()

	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
}

// chmod sets the folder's mode.
func chmod(t *testing.T, dir string, mode fs.FileMode) {
	t.Helper()

	if err := os.Chmod(dir, mode); err != nil {
		t.Error(err)
	}
}
