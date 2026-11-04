package repo

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strings"
)

// RemoteURL is a URL of a remote in the repo's config: one of its urls, or of its pushurls.
type RemoteURL struct {
	Remote string // The remote's name.
	URL    string // The URL as the config holds it, before any of git's insteadOf rules.
	Push   bool   // A pushurl, which git's pushInsteadOf rules skip (fact 38).
}

// noMatch is git config's exit status when no setting matches.
const noMatch = 1

// RemoteURLs returns the URLs of the remotes in the config of the repo that holds the folder, its
// own and the user's, in the order that git reads them, as the config holds them: before any of
// git's insteadOf rules, which git's pushInsteadOf rules match too (fact 38).
func RemoteURLs(ctx context.Context, dir string) ([]RemoteURL, error) {
	out, err := git(ctx, dir, "config", "--null", "--get-regexp", `^remote\..*\.(push)?url$`)
	if exit, ok := errors.AsType[*exec.ExitError](err); ok && exit.ExitCode() == noMatch {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("read the remotes' URLs: %w", err)
	}

	var urls []RemoteURL

	// Each setting is its name, a newline and its value, ended by a NUL: remote.<name>.url or
	// remote.<name>.pushurl, where the remote's name may hold dots.
	for setting := range strings.SplitSeq(strings.TrimSuffix(out, "\x00"), "\x00") {
		key, url, _ := strings.Cut(setting, "\n")
		remote, kind, _ := strings.CutLast(strings.TrimPrefix(key, "remote."), ".")
		urls = append(urls, RemoteURL{Remote: remote, URL: url, Push: kind == "pushurl"})
	}

	return urls, nil
}

// clearingMajor and clearingMinor are the first git version in which an empty pushurl clears the
// remote's pushurls before it, rather than adding one (fact 38).
const clearingMajor, clearingMinor = 2, 46

// ClearsPushURLs reports whether the user's git clears a remote's pushurls with an empty one, as
// git 2.46 and later do. It takes a version that it can't read for an older one.
func ClearsPushURLs(ctx context.Context) (bool, error) {
	out, err := git(ctx, "", "version")
	if err != nil {
		return false, fmt.Errorf("find git's version: %w", err)
	}

	var major, minor int
	if read, _ := fmt.Sscanf(out, "git version %d.%d", &major, &minor); read < 2 {
		return false, nil
	}

	return slices.Compare([]int{major, minor}, []int{clearingMajor, clearingMinor}) >= 0, nil
}
