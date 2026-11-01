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

// Create stores a new feature, with its first entry in its journal, whose note is the holder's
// work. It fails if the slug is taken.
func (s Store) Create(feature Feature, entry Entry) error {
	slug := feature.Slug
	if err := checkSlug(slug); err != nil {
		return err
	}

	if err := entry.Event.check(); err != nil {
		return fmt.Errorf("new feature %s: %w", slug, err)
	}

	if len(entry.Note) == 0 {
		return fmt.Errorf("new feature %s: want a note, %s's work", slug, feature.Flow.Holder)
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

	return commit(root, &record{Feature: feature}, entry)
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

// List returns the features in baton's folder, by slug. Like Read, it takes no lock. It skips
// what isn't a feature's folder, and the folder of a feature that Create hasn't written yet
// (decision 30).
func (s Store) List() ([]Feature, error) {
	root, err := os.OpenRoot(s.Dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("open baton's folder: %w", err)
	}

	defer func() { _ = root.Close() }()

	entries, err := fs.ReadDir(root.FS(), featuresFolder)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("list the features: %w", err)
	}

	var features []Feature

	for _, entry := range entries {
		if !entry.IsDir() || checkSlug(entry.Name()) != nil {
			continue
		}

		rec, err := load(root, entry.Name())
		if errors.Is(err, errNoFeature) {
			continue
		}

		if err != nil {
			return nil, err
		}

		features = append(features, rec.Feature)
	}

	return features, nil
}

// Update changes the feature with the slug under its lock, so no other change comes between its
// read and its write, adds the entry to its journal, and returns the feature as changed. The
// change gets the feature as its file holds it, once checked. Nothing is written when the change
// fails, leaves an invalid feature, or moves it to another holder without a note, the holder's
// work. The fields of the file that this baton doesn't know stay as they are.
func (s Store) Update(slug string, entry Entry, change func(*Feature) error) (Feature, error) {
	if err := entry.Event.check(); err != nil {
		return Feature{}, fmt.Errorf("change feature %s: %w", slug, err)
	}

	root, done, err := s.locked(slug)
	if err != nil {
		return Feature{}, err
	}
	defer done()

	rec, err := load(root, slug)
	if err != nil {
		return Feature{}, err
	}

	holder := rec.Flow.Holder
	if err := change(&rec.Feature); err != nil {
		return Feature{}, err
	}

	if err := rec.check(slug); err != nil {
		return Feature{}, fmt.Errorf("the change leaves feature %s invalid: %w", slug, err)
	}

	if rec.Flow.Holder != holder && len(entry.Note) == 0 {
		return Feature{}, fmt.Errorf(
			"change feature %s: want a note, %s's work",
			slug,
			rec.Flow.Holder,
		)
	}

	if err := commit(root, &rec, entry); err != nil {
		return Feature{}, err
	}

	return rec.Feature, nil
}

// Remove removes the feature with the slug, under its lock: its file first, which ends the
// feature, then its event log and its notes. It keeps the lock file, which baton never replaces,
// so a process that waits for the lock finds no feature once it gets it, and a new feature can
// take the slug (decision 40), and the roles' wait marks, which a wait may hold (decision 47).
func (s Store) Remove(slug string) error {
	root, done, err := s.locked(slug)
	if err != nil {
		return err
	}
	defer done()

	dir := folder(slug)

	err = root.Remove(filepath.Join(dir, featureFile))
	if errors.Is(err, fs.ErrNotExist) {
		return noFeature(slug)
	}

	if err == nil {
		err = errors.Join(root.RemoveAll(filepath.Join(dir, eventsFile)),
			root.RemoveAll(filepath.Join(dir, notesFolder)))
	}

	if err != nil {
		return fmt.Errorf("remove feature %s: %w", slug, err)
	}

	return nil
}

// NotePath returns the path of the feature's note with the name.
func (s Store) NotePath(slug, name string) string {
	return filepath.Join(s.Dir, folder(slug), notesFolder, name)
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
