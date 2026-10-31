package state

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// Store keeps the features' state in a folder. In a repo, it's baton's folder in the git common
// folder, which the repo's worktrees share and git never commits.
type Store struct {
	Dir string
}

// Read returns the feature with the slug, once checked.
func (s Store) Read(slug string) (Feature, error) {
	root, err := s.open(slug)
	if err != nil {
		return Feature{}, err
	}
	defer func() { _ = root.Close() }()

	rec, err := load(root, slug)
	if err != nil {
		return Feature{}, err
	}

	return rec.Feature, nil
}

// open checks the slug and opens baton's folder.
func (s Store) open(slug string) (*os.Root, error) {
	if err := checkSlug(slug); err != nil {
		return nil, err
	}

	root, err := os.OpenRoot(s.Dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, noFeature(slug)
	}

	if err != nil {
		return nil, fmt.Errorf("open baton's folder: %w", err)
	}

	return root, nil
}
