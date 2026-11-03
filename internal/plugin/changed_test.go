package plugin

import (
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"testing"
)

// hooksFile and agentsFolder are paths in the plugin's folder.
var (
	hooksFile    = filepath.Join("com.github.copilot", "hooks", "hooks.json")
	agentsFolder = filepath.Join("com.github.copilot", "agents")
)

// TestWriteTakesTheNextNameForAChangedFolder has Write find the plugin's folder changed, as a role
// could change it (risk 12), and write the plugin under the next name, which serves from then on,
// and leave the changed folder as it is (decision 53).
func TestWriteTakesTheNextNameForAChangedFolder(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(t *testing.T, dir string) // Changes the plugin's folder.
	}{
		{"a changed file", func(t *testing.T, dir string) {
			must(t, os.WriteFile(filepath.Join(dir, hooksFile), []byte("{}"), 0o600))
		}},
		{"a file more", func(t *testing.T, dir string) {
			put(t, filepath.Join(dir, agentsFolder, "helper.agent.md"))
		}},
		{"a file less", func(t *testing.T, dir string) {
			must(t, os.Remove(filepath.Join(dir, hooksFile)))
		}},
		{"a file more that it can't read", func(t *testing.T, dir string) {
			if os.Geteuid() == 0 {
				t.Skip("root reads any file")
			}

			put(t, filepath.Join(dir, "notes.md"))
			chmod(t, filepath.Join(dir, "notes.md"), 0)
		}},
		{"a symlink to a copy of a file", func(t *testing.T, dir string) {
			moveAway(t, filepath.Join(dir, hooksFile), filepath.Join(t.TempDir(), "hooks.json"))
		}},
		{"a symlink to a copy of the folder", func(t *testing.T, dir string) {
			moveAway(t, dir, filepath.Join(t.TempDir(), "plugin"))
		}},
		{"an empty folder", func(t *testing.T, dir string) {
			must(t, os.RemoveAll(dir))
			must(t, os.Mkdir(dir, 0o700))
		}},
		{"a file in the folder's place", func(t *testing.T, dir string) {
			must(t, os.RemoveAll(dir))
			put(t, dir)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			cache := t.TempDir()
			first := take(Write(cache))(t)
			test.change(t, first)
			before := listing(t, first)

			next := take(Write(cache))(t)
			if next != first+"-1" {
				t.Errorf("Write = %s, want %s-1", next, first)
			}

			if again := take(Write(cache))(t); again != next {
				t.Errorf("the next Write = %s, want %s", again, next)
			}

			same(t, next, thePlugin(t))
			only(t, cache, filepath.Base(first), filepath.Base(next))

			if after := listing(t, first); !maps.Equal(after, before) {
				t.Errorf("Write changed %s to %v, from %v", first, after, before)
			}
		})
	}
}

func TestWriteByManyPanesAtOnceAfterAChange(t *testing.T) {
	cache := t.TempDir()
	first := take(Write(cache))(t)
	must(t, os.WriteFile(filepath.Join(first, hooksFile), []byte("{}"), 0o600))

	if dir := atOnce(t, cache); dir != first+"-1" {
		t.Errorf("the panes' Write = %s, want %s-1", dir, first)
	}

	only(t, cache, filepath.Base(first), filepath.Base(first)+"-1")
}

// moveAway moves the file or folder to the new path, and puts a symlink to it in its place.
func moveAway(t *testing.T, path, to string) {
	t.Helper()

	must(t, os.Rename(path, to))
	must(t, os.Symlink(to, path))
}

// listing returns the paths in the folder, or the file, without following a symlink, each with its
// mode, size and time of change, which change if anything writes there.
func listing(t *testing.T, path string) map[string]string {
	t.Helper()

	found := map[string]string{}

	err := filepath.WalkDir(path, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}

		found[path] = fmt.Sprint(info.Mode(), info.Size(), info.ModTime())

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	return found
}
