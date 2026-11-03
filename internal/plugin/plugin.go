// Package plugin holds baton's plugin for Copilot CLI, which the binary embeds and writes to its
// cache for --plugin-dir: the agents of the orchestrator and the roles, the preToolUse hook, which
// runs baton hook guard, and the agentStop hook, which runs baton hook stop (decisions 11, 36, 37,
// 47, 52 and 53).
package plugin

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/codedogapp/baton/internal/flow"
)

// Name is the plugin's name, which its agents' IDs start with (fact 3).
const Name = "baton"

const (
	root          = "files" // The folder in the binary that holds the plugin's files.
	hashSize      = 8       // The bytes of the files' hash that name the plugin's folder.
	privateFolder = 0o700   // The cache folder's mode: only the user's.
	names         = 8       // The names that the plugin's folder may take: plugin-<hash>, -1 to -7.
)

//go:embed files
var files embed.FS

// Agent returns the ID of the role's agent, which --agent takes: the plugin's name and the name of
// the agent's file (fact 24).
func Agent(role flow.Role) string {
	return Name + ":" + string(role)
}

// Write writes the plugin into baton's cache folder, unless it's there already, unchanged, and
// returns the plugin's folder (decisions 36 and 53).
func Write(cache string) (string, error) {
	return write(cache, files)
}

// write writes the files in the tree's root folder into the cache, in a folder named by their
// hash, so that each version of the plugin gets a folder of its own. The folder appears whole:
// the files go into a new temporary folder, which then takes the folder's name, unless another
// baton has given it that name first.
//
// A folder that has the name already serves only if it holds the same files, since a role runs as
// the user and could have changed them, and with them the hooks that guard the panes (risk 12).
// Else the files take the next name: write never replaces a folder, which a copilot may be
// loading (decision 53).
func write(cache string, tree fs.FS) (string, error) {
	sum, err := digest(tree, root)
	if err != nil {
		return "", fmt.Errorf("hash the plugin: %w", err)
	}

	if err := os.MkdirAll(cache, privateFolder); err != nil {
		return "", fmt.Errorf("make the cache folder: %w", err)
	}

	temp, err := os.MkdirTemp(cache, "plugin-*.tmp")
	if err != nil {
		return "", fmt.Errorf("write the plugin: %w", err)
	}

	defer func() { _ = os.RemoveAll(temp) }()

	if err := os.CopyFS(temp, tree); err != nil {
		return "", fmt.Errorf("write the plugin: %w", err)
	}

	return place(filepath.Join(temp, root), filepath.Join(cache, "plugin-"+sum), sum)
}

// place gives the folder the name, or else the first of the next names, name-1 to name-7, that it
// can take, and returns that name. A name whose folder holds the files whose hash is the sum
// serves as it is.
func place(folder, name, sum string) (string, error) {
	var err error

	for try := range names {
		dir := name
		if try > 0 {
			dir += "-" + strconv.Itoa(try)
		}

		if err = os.Rename(folder, dir); err == nil || intact(dir, sum) {
			return dir, nil
		}
	}

	return "", fmt.Errorf("write the plugin under any of %d names: %w", names, err)
}

// intact reports whether the folder holds the files whose hash is the sum, and is a folder, not
// a symlink to one, which could lead anywhere.
func intact(dir, sum string) bool {
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() {
		return false
	}

	found, err := digest(os.DirFS(dir), ".")

	return err == nil && found == sum
}

// digest returns a hash of the files in the tree's top folder: their paths in that folder and their
// contents. It fails on an entry that is neither a folder nor a regular file, such as a symlink,
// which could lead anywhere, or a named pipe, which could keep it waiting.
func digest(tree fs.FS, top string) (string, error) {
	hash := sha256.New()

	err := fs.WalkDir(tree, top, func(path string, entry fs.DirEntry, err error) error {
		switch {
		case err != nil || entry.IsDir():
			return err
		case !entry.Type().IsRegular():
			return fmt.Errorf("%s isn't a regular file", path)
		}

		data, err := fs.ReadFile(tree, path)
		if err != nil {
			return err
		}

		_, _ = fmt.Fprintf(hash, "%s\x00%d\x00", strings.TrimPrefix(path, top+"/"), len(data))
		_, _ = hash.Write(data)

		return nil
	})
	if err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)[:hashSize]), nil
}
