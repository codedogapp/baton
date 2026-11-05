package state

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// format is the layout of the feature file that this baton reads and writes. It changes only
// when an older baton would misread the new layout. A new field at the top of the file doesn't
// change it: an older baton ignores it, and keeps it when it writes the file.
const format = 1

// The files in baton's folder, which only the user can read and write.
const (
	featuresFolder = "features"
	featureFile    = "feature.json"
	lockFile       = "lock"

	privateFolder fs.FileMode = 0o700
	privateFile   fs.FileMode = 0o600
)

// record is the feature file's content: the file's format, the feature, and the fields at the
// top of the file that this baton doesn't know, which a newer baton wrote.
type record struct {
	Format int `json:"format"`
	Feature
	Unknown jsontext.Value `json:",embed"`
}

// folder returns the feature's folder in baton's folder.
func folder(slug string) string {
	return filepath.Join(featuresFolder, slug)
}

// errNoFeature is the error of a feature that baton's folder doesn't hold.
var errNoFeature = errors.New("no feature")

// noFeature reports that baton's folder holds no feature with the slug.
func noFeature(slug string) error {
	return fmt.Errorf("%w %s", errNoFeature, slug)
}

// load reads the feature's file and checks it.
func load(root *os.Root, slug string) (record, error) {
	name := filepath.Join(folder(slug), featureFile)

	data, err := root.ReadFile(name)
	if errors.Is(err, fs.ErrNotExist) {
		return record{}, noFeature(slug)
	}

	if err != nil {
		return record{}, fmt.Errorf("read feature %s: %w", slug, err)
	}

	rec, err := decode[record](data, format)
	if err == nil {
		err = rec.check(slug)
	}

	if err != nil {
		return record{}, fmt.Errorf("feature file %s: %w", filepath.Join(root.Name(), name), err)
	}

	return rec, nil
}

// decode returns the value in a file's content, in the format that this baton wants. It reads the
// format first, so a newer layout gets a clear error, not a confusing one about its fields.
func decode[T any](data []byte, want int) (T, error) {
	var head struct {
		Format int `json:"format"`
	}

	var value T

	if err := json.Unmarshal(data, &head); err != nil {
		return value, err
	}

	switch {
	case head.Format > want:
		return value, fmt.Errorf(
			"format %d is newer than this baton's, %d: run the newer baton", head.Format, want)
	case head.Format != want:
		return value, fmt.Errorf("format %d: want %d", head.Format, want)
	}

	err := json.Unmarshal(data, &value)

	return value, err
}

// save writes the record, in this baton's format, over the feature's file. The caller holds the
// lock.
func save(root *os.Root, rec record) error {
	rec.Format = format

	if err := replace(root, filepath.Join(folder(rec.Slug), featureFile), rec); err != nil {
		return fmt.Errorf("write feature %s: %w", rec.Slug, err)
	}

	return nil
}

// replace writes the value as JSON to a new temporary file, then renames it over the file with
// the name, so a reader sees the old file or the new one, never part of one. The caller holds the
// file's lock.
func replace(root *os.Root, name string, value any) error {
	data, err := json.Marshal(value, jsontext.WithIndent("  "))
	if err != nil {
		return err
	}

	temp := name + ".tmp"

	// A crash can leave a temporary file behind: it goes first, so the new one gets mode 0600.
	_ = root.Remove(temp)

	err = write(root, temp, append(data, '\n'))
	if err == nil {
		err = root.Rename(temp, name)
	}

	return err
}

// write writes the data to a new file, with mode 0600, and syncs it to disk.
func write(root *os.Root, name string, data []byte) error {
	file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, privateFile)
	if err != nil {
		return err
	}

	_, err = file.Write(data)
	if err == nil {
		err = file.Sync()
	}

	return errors.Join(err, file.Close())
}
