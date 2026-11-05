package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/codedogapp/baton/internal/bitbucket"
	"github.com/codedogapp/baton/internal/config"
	"github.com/codedogapp/baton/internal/github"
	"github.com/codedogapp/baton/internal/keychain"
	"github.com/codedogapp/baton/internal/repo"
)

// bitbucketService is the service of the keychain's item that holds the user's Bitbucket API
// token, whose account is the user's email (decision 65).
const bitbucketService = "baton-bitbucket"

// bitbucketCloud is Bitbucket Cloud, where baton opens the pull requests of the repo, and
// comments on them, as the user (decision 65).
type bitbucketCloud struct {
	client bitbucket.Client
	repo   bitbucket.Repo
}

// hostOf returns the host of the pull requests of the folder's repo: Bitbucket Cloud if the URL
// of its origin remote names bitbucket.org, and GitHub otherwise (decision 65).
func hostOf(ctx context.Context, env Env) (host, error) {
	remote, err := repo.Remote(ctx, env.Dir)
	if err != nil {
		return nil, err
	}

	where, onBitbucket := bitbucket.RepoOf(config.Key(remote))
	if !onBitbucket {
		return gitHub{dir: env.Dir}, nil
	}

	client, err := bitbucketClient(ctx, env, remote)
	if err != nil {
		return nil, err
	}

	return bitbucketCloud{client: client, repo: where}, nil
}

// bitbucketClient returns a client for Bitbucket Cloud as the user: with their account's email,
// from the config file for the repo whose origin remote has the URL, and their API token, from
// the keychain (decision 65).
func bitbucketClient(ctx context.Context, env Env, remote string) (bitbucket.Client, error) {
	settings, err := settingsOf(remote)
	if err == nil && settings.Bitbucket.Email == "" {
		err = fmt.Errorf(`no Bitbucket account for this repo: set "bitbucket" in %s to your `+
			`email address at Atlassian, such as {"email": "you@example.com"}`, configFile())
	}

	if err != nil {
		return bitbucket.Client{}, err
	}

	email := settings.Bitbucket.Email

	token, err := env.Keychain(ctx, bitbucketService, email)
	if errors.Is(err, keychain.ErrNotFound) {
		return bitbucket.Client{}, fmt.Errorf("no Bitbucket API token for %s in the keychain: "+
			"add it with `security add-generic-password -s %s -a %s -w`, which asks for it",
			email, bitbucketService, quoted(email))
	}

	if err != nil {
		return bitbucket.Client{}, err
	}

	return bitbucket.Client{HTTP: env.HTTP, Email: email, Token: token}, nil
}

func (b bitbucketCloud) open(ctx context.Context, request github.PullRequest) (string, error) {
	report, err := readReport(request.BodyFile)
	if err != nil {
		return "", err
	}

	link, err := b.client.Open(ctx, bitbucket.PullRequest{
		Repo: b.repo, Branch: request.Branch, Title: request.Title, Description: report,
	})

	return link, b.explained(err)
}

func (b bitbucketCloud) comment(ctx context.Context, url, bodyFile string) error {
	report, err := readReport(bodyFile)
	if err != nil {
		return err
	}

	return b.explained(b.client.Comment(ctx, url, report))
}

// explained returns the error, with what to check if Bitbucket refused the user's login.
func (b bitbucketCloud) explained(err error) error {
	if _, refused := errors.AsType[bitbucket.LoginError](err); !refused {
		return err
	}

	email := b.client.Email

	return fmt.Errorf("the Bitbucket login as %s failed (%w): check that email, and if the "+
		"token has expired, make a new one with the scopes read:pullrequest:bitbucket and "+
		"write:pullrequest:bitbucket, and replace it with `security add-generic-password -U "+
		"-s %s -a %s -w`, which asks for it", email, err, bitbucketService, quoted(email))
}

// readReport returns the lead's report in the file, which a pull request on Bitbucket takes as
// its text, where gh reads the file itself.
func readReport(file string) (string, error) {
	report, err := os.ReadFile(filepath.Clean(file))
	if err != nil {
		return "", fmt.Errorf("read the lead's report: %w", err)
	}

	return string(report), nil
}
