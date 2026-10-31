package state

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Store keeps the features' state in a folder. In a repo, it's baton's folder in the git common
// folder, which the repo's worktrees share and git never commits.
type Store struct {
	Dir string
}

// Create stores a new feature. It fails if the slug is taken.
func (s Store) Create(feature Feature) error {
	slug := feature.Slug
	if err := checkSlug(slug); err != nil {
		return err
	}

	if err := feature.check(slug); err != nil {
		return fmt.Errorf("new feature %s: %w", slug, err)
	}

	if err := os.MkdirAll(filepath.Join(s.Dir, folder(slug)), privateFolder); err != nil {
		return fmt.Errorf("make the folder of feature %s: %w", slug, err)
	}

	root, done, err := s.locked(slug)
	if err != nil {
		return err
	}
	defer done()

	_, err = root.Lstat(filepath.Join(folder(slug), featureFile))
	if err == nil {
		return fmt.Errorf("feature %s exists", slug)
	}

	if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("look for feature %s: %w", slug, err)
	}

	return save(root, record{Feature: feature})
}

// Read returns the feature with the slug, once checked. It takes no lock: a change renames a
// whole new file over the old one, so a reader gets one or the other.
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

// Update changes the feature with the slug under its lock, so no other change comes between its
// read and its write, and returns the feature as changed. The change gets the feature as its file
// holds it, once checked. Nothing is written when the change fails or leaves an invalid feature.
// The fields of the file that this baton doesn't know stay as they are.
func (s Store) Update(slug string, change func(*Feature) error) (Feature, error) {
	root, done, err := s.locked(slug)
	if err != nil {
		return Feature{}, err
	}
	defer done()

	rec, err := load(root, slug)
	if err != nil {
		return Feature{}, err
	}

	if err := change(&rec.Feature); err != nil {
		return Feature{}, err
	}

	if err := rec.check(slug); err != nil {
		return Feature{}, fmt.Errorf("the change leaves feature %s invalid: %w", slug, err)
	}

	if err := save(root, rec); err != nil {
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

// locked opens baton's folder and takes the feature's lock. The caller calls done to release
// both.
func (s Store) locked(slug string) (root *os.Root, done func(), err error) {
	root, err = s.open(slug)
	if err != nil {
		return nil, nil, err
	}

	unlock, err := lock(root, slug)
	if err != nil {
		_ = root.Close()

		return nil, nil, err
	}

	return root, func() { unlock(); _ = root.Close() }, nil
}
