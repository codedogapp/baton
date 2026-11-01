// Package state keeps the features' state on disk, in baton's folder, where every baton process
// of the repo finds it and a run survives a restart.
package state

import (
	"fmt"
	"strings"

	"github.com/codedogapp/baton/internal/flow"
)

// slugChars are the characters of a slug's words.
const slugChars = "abcdefghijklmnopqrstuvwxyz0123456789"

// Feature is a feature's state.
type Feature struct {
	// Slug names the feature: its folder in baton's folder, and its branch and its tmux session.
	Slug string `json:"slug"`
	// Branch is the feature's branch, which its worktree holds.
	Branch string `json:"branch,omitempty"`
	// Worktree is the absolute path of the feature's worktree, where its roles work.
	Worktree string `json:"worktree,omitempty"`
	// Flow is where the feature stands in the flow.
	Flow flow.State `json:"flow"`
	// Work is the holder's work: the last note, since each note goes to the holder after the
	// change that brought it (decision 31).
	Work Work `json:"work,omitzero"`
}

// Work is the note that the holder works from.
type Work struct {
	Note    string       `json:"note"`             // The note's name in the notes folder.
	From    flow.Role    `json:"from"`             // The role that wrote it.
	Verdict flow.Verdict `json:"verdict,omitzero"` // That role's verdict, if it gave one.
	Picked  bool         `json:"picked"`           // Whether the holder's baton wait picked it up.
}

// check returns an error if the feature can't be the one in the folder of the slug.
func (f Feature) check(slug string) error {
	if f.Slug != slug {
		return fmt.Errorf("slug %q: want %q, the feature's folder", f.Slug, slug)
	}

	if err := f.Flow.Validate(); err != nil {
		return fmt.Errorf("flow: %w", err)
	}

	return nil
}

// checkSlug returns an error if the slug isn't words of lowercase letters and digits joined by
// hyphens, such as demo-1: it names a folder, a branch and a tmux session.
func checkSlug(slug string) error {
	for word := range strings.SplitSeq(slug, "-") {
		if word == "" || strings.Trim(word, slugChars) != "" {
			return fmt.Errorf(
				"slug %q: want words of lowercase letters and digits joined by hyphens, "+
					"such as demo-1", slug)
		}
	}

	return nil
}
