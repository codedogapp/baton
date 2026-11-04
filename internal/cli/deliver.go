package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/codedogapp/baton/internal/flow"
	"github.com/codedogapp/baton/internal/github"
	"github.com/codedogapp/baton/internal/repo"
	"github.com/codedogapp/baton/internal/state"
)

// deliver delivers a ready feature, on the user's word: it pushes the feature's branch to origin,
// then opens the branch's pull request on GitHub, with the title and the lead's final report as
// its body, or comments the report on the pull request that an earlier delivery opened, and
// records it (decision 61). Only the orchestrator delivers features, so it refuses a role's pane.
func deliver(ctx context.Context, env Env, args []string) error {
	slug, title, err := parseDeliver(env, args)
	if err != nil {
		return err
	}

	if role := flow.Role(env.Getenv(envRole)); role != "" && role != flow.RoleOrchestrator {
		return fmt.Errorf("only the orchestrator delivers a feature, and %s names the %s",
			envRole, role)
	}

	store, err := openStore(ctx, env.Dir)
	if err != nil {
		return err
	}

	feature, err := store.Read(slug)
	if err == nil {
		err = deliverable(feature, title)
	}

	if err == nil {
		err = repo.Push(ctx, env.Dir, feature.Branch)
	}

	if err != nil {
		return err
	}

	pr := github.PullRequest{
		Branch: feature.Branch, Title: title, BodyFile: store.NotePath(slug, feature.Work.Note),
	}

	url, did, err := publish(ctx, env.Dir, feature.PullRequest, pr)
	if err != nil {
		return err
	}

	entry := state.Entry{Event: state.Event{
		Kind: state.EventDeliver, Role: flow.RoleOrchestrator, Text: url,
	}}
	if _, err := store.Update(slug, entry, delivered(url)); err != nil {
		return fmt.Errorf("record the pull request %s: %w", url, err)
	}

	_, err = fmt.Fprintf(env.Stdout, "Delivered %s: pushed %s to origin and %s its pull request, "+
		"%s.\n", slug, feature.Branch, did, url)

	return err
}

// parseDeliver parses baton deliver's command line: the feature's slug, and the pull request's
// title, trimmed, if given.
func parseDeliver(env Env, args []string) (slug, title string, err error) {
	flags := newFlags(env, "deliver")
	titled := flags.String("title", "", "the pull request's `title`, for the first delivery")

	slug, err = parseSlug(flags, args)
	if err == nil && slug == "" {
		err = errors.New("want the feature's slug: baton deliver <slug> [--title <title>]")
	}

	return slug, strings.TrimSpace(*titled), err
}

// deliverable returns an error if the feature isn't ready, or if the title doesn't suit the
// delivery: the first opens the feature's pull request, with the title, and the later ones
// comment on it, which keeps its title.
func deliverable(feature state.Feature, title string) error {
	if _, err := feature.Flow.Deliver(); err != nil {
		return fmt.Errorf("%s: %w", feature.Slug, err)
	}

	switch {
	case feature.PullRequest == "" && title == "":
		return fmt.Errorf("want the pull request's title for the first delivery of %s: "+
			"baton deliver %[1]s --title <title>", feature.Slug)
	case feature.PullRequest != "" && title != "":
		return fmt.Errorf("%s has its pull request already, %s, which keeps its title: "+
			"deliver it again without --title", feature.Slug, feature.PullRequest)
	}

	return nil
}

// publish opens the pull request, or comments its body on the one at the URL, if any, in the
// folder's repo. It returns the pull request's URL, and what it did.
func publish(ctx context.Context, dir, url string, request github.PullRequest) (
	link, did string, err error,
) {
	if url != "" {
		return url, "commented the lead's report on",
			github.Comment(ctx, dir, url, request.BodyFile)
	}

	link, err = github.Open(ctx, dir, request)

	return link, "opened", err
}

// delivered returns the change that records the feature's delivery to its pull request at the
// URL.
func delivered(url string) func(*state.Feature) error {
	return func(feature *state.Feature) error {
		next, err := feature.Flow.Deliver()
		if err != nil {
			return fmt.Errorf("%s: %w", feature.Slug, err)
		}

		feature.Flow, feature.PullRequest = next, url

		return nil
	}
}
