package state

import (
	"encoding/json/v2"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// format is the layout of the feature file that this baton reads. It changes only when an older
// baton would misread the new layout. A new field at the top of the file doesn't change it: an
// older baton ignores it.
const format = 1

// The files in baton's folder.
const (
	featuresFolder = "features"
	featureFile    = "feature.json"
)

// record is the feature file's content: the file's format and the feature.
type record struct {
	Format int `json:"format"`
	Feature
}

// folder returns the feature's folder in baton's folder.
func folder(slug string) string {
	return filepath.Join(featuresFolder, slug)
}

// noFeature reports that baton's folder holds no feature with the slug.
func noFeature(slug string) error {
	return fmt.Errorf("no feature %s", slug)
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

	rec, err := decode(data)
	if err == nil {
		err = rec.check(slug)
	}

	if err != nil {
		return record{}, fmt.Errorf("feature file %s: %w", filepath.Join(root.Name(), name), err)
	}

	return rec, nil
}

// decode returns the record in a feature file's content. It reads the format first, so a newer
// layout gets a clear error, not a confusing one about its fields.
func decode(data []byte) (record, error) {
	var head struct {
		Format int `json:"format"`
	}

	if err := json.Unmarshal(data, &head); err != nil {
		return record{}, err
	}

	switch {
	case head.Format > format:
		return record{}, fmt.Errorf(
			"format %d is newer than this baton's, %d: run the newer baton", head.Format, format)
	case head.Format != format:
		return record{}, fmt.Errorf("format %d: want %d", head.Format, format)
	}

	var rec record
	if err := json.Unmarshal(data, &rec); err != nil {
		return record{}, err
	}

	return rec, nil
}
