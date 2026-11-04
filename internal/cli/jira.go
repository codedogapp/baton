package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode"

	"github.com/codedogapp/baton/internal/config"
	"github.com/codedogapp/baton/internal/jira"
	"github.com/codedogapp/baton/internal/keychain"
)

// keychainService is the service of the keychain's item that holds the user's Jira API token,
// whose account is the user's email (decision 60).
const keychainService = "baton-jira"

// showTicket prints a Jira ticket and its comments as Markdown, for the orchestrator to plan from:
// it reads the ticket from the site in the config file, as the user, with their API token from
// the keychain (decision 60).
func showTicket(ctx context.Context, env Env, args []string) error {
	keys, err := parseTexts(newFlags(env, "jira"), args, "a ticket's key, such as DEMO-1")
	if err != nil {
		return err
	}

	if err := jira.CheckKey(keys[0]); err != nil {
		return err
	}

	site, err := jiraSite(ctx, env.Dir)
	if err != nil {
		return err
	}

	token, err := env.Keychain(ctx, keychainService, site.Email)
	if errors.Is(err, keychain.ErrNotFound) {
		return fmt.Errorf("no Jira API token for %s in the keychain: add it with `security "+
			"add-generic-password -s %s -a %s -w`, which asks for it", site.Email,
			keychainService, quoted(site.Email))
	}

	if err != nil {
		return err
	}

	client := jira.Client{HTTP: env.HTTP, Site: site.Site, Email: site.Email, Token: token}

	ticket, err := client.Ticket(ctx, keys[0])
	if _, refused := errors.AsType[jira.LoginError](err); refused {
		return fmt.Errorf("the Jira login as %s failed (%w): check that email, and if the token "+
			"has expired or lacks the scope read:jira-work, make a new one and replace it with "+
			"`security add-generic-password -U -s %s -a %s -w`, which asks for it", site.Email,
			err, keychainService, quoted(site.Email))
	}

	if err != nil {
		return err
	}

	_, err = io.WriteString(env.Stdout, printable(ticket.Markdown()))

	return err
}

// jiraSite returns the Jira site of the folder's repo, from the config file, and refuses one
// without its host name or the user's email.
func jiraSite(ctx context.Context, dir string) (config.Jira, error) {
	settings, err := loadSettings(ctx, dir)
	if err == nil && (settings.Jira.Site == "" || settings.Jira.Email == "") {
		err = fmt.Errorf(`no Jira site for this repo: set "jira" in %s to the site's host name `+
			`and your email address at Atlassian, such as {"site": "example.atlassian.net", `+
			`"email": "you@example.com"}`, configFile())
	}

	return settings.Jira, err
}

// quoted returns the text in single quotes, as the shell reads it.
func quoted(text string) string {
	return "'" + strings.ReplaceAll(text, "'", `'\''`) + "'"
}

// printable returns the text without its control characters, but for newlines and tabs, so that a
// ticket can't send escape sequences to the terminal.
func printable(text string) string {
	return strings.Map(func(char rune) rune {
		if unicode.IsControl(char) && char != '\n' && char != '\t' {
			return -1
		}

		return char
	}, text)
}
