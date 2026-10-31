// Package plugin holds baton's plugin for Copilot CLI, the roles' agents, which the binary embeds
// and writes to its cache for --plugin-dir (decisions 11, 36 and 37).
package plugin

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/codedogapp/baton/internal/flow"
)

// Name is the plugin's name, which its agents' IDs start with (fact 3).
const Name = "baton"

const (
	root          = "files" // The folder in the binary that holds the plugin's files.
	hashSize      = 8       // The bytes of the files' hash that name the plugin's folder.
	privateFolder = 0o700   // The cache folder's mode: only the user's.
)

//go:embed files
var files embed.FS

// Agent returns the ID of the role's agent, which --agent takes: the plugin's name and the name of
// the agent's file (fact 24).
func Agent(role flow.Role) string {
	return Name + ":" + string(role)
}

// Write writes the plugin into baton's cache folder, unless it's there already, and returns the
// plugin's folder (decision 36).
func Write(cache string) (string, error) {
	return write(cache, files)
}

// write writes the files in the tree's root folder into the cache, in a folder named by their
// hash, so that each version of the plugin gets a folder of its own. The folder appears whole:
// the files go into a new temporary folder, which then takes the folder's name, unless another
// baton has given it that name first.
func write(cache string, tree fs.FS) (string, error) {
	sum, err := digest(tree)
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

	// A folder with the name already holds the same files: another baton has written them.
	dir := filepath.Join(cache, "plugin-"+sum)
	if err := os.Rename(filepath.Join(temp, root), dir); err != nil &&
		!errors.Is(err, fs.ErrExist) {
		return "", fmt.Errorf("write the plugin: %w", err)
	}

	return dir, nil
}

// digest returns a hash of the files in the tree's root folder: their paths and their contents.
func digest(tree fs.FS) (string, error) {
	hash := sha256.New()

	err := fs.WalkDir(tree, root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}

		data, err := fs.ReadFile(tree, path)
		if err != nil {
			return err
		}

		_, _ = fmt.Fprintf(hash, "%s\x00%d\x00", path, len(data))
		_, _ = hash.Write(data)

		return nil
	})
	if err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)[:hashSize]), nil
}
