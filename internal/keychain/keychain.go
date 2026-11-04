// Package keychain reads a password from the user's keychains through the Mac's security tool,
// which needs nothing else installed (decision 60).
package keychain

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Tool is a program that reads the user's keychains as the Mac's security tool does: that tool,
// or a stand-in in tests.
type Tool string

// Security is the Mac's keychain tool, by its full path, so that no other program on the user's
// PATH gets the item's names.
const Security Tool = "/usr/bin/security"

// ErrNotFound is the error for an item that the user's keychains don't hold.
var ErrNotFound = errors.New("the keychain holds no such item")

// notFound is security's exit code for an item that the keychains don't hold (fact 42).
const notFound = 44

// Password returns the password of the generic item for the service and the account, from the
// user's keychains, or ErrNotFound if they hold none. security gets it without asking the user
// when security added the item (fact 42).
func (t Tool) Password(ctx context.Context, service, account string) (string, error) {
	// gosec's G204 wants constant arguments, but the item is the caller's: its service and its
	// account reach security as the values of its options, which no shell reads (decision 34).
	//nolint:gosec // G204: the tool's command is fixed, and the item's names are option values.
	cmd := exec.CommandContext(ctx, string(t), "find-generic-password", "-s", service, "-a",
		account, "-w")

	out, err := cmd.Output()
	exit, failed := errors.AsType[*exec.ExitError](err)

	switch {
	case failed && exit.ExitCode() == notFound:
		return "", ErrNotFound
	case failed:
		return "", fmt.Errorf("read the keychain: %w: %s", err, bytes.TrimSpace(exit.Stderr))
	case err != nil:
		return "", fmt.Errorf("read the keychain: %w", err)
	}

	// security ends the password with a newline.
	return strings.TrimSuffix(string(out), "\n"), nil
}
