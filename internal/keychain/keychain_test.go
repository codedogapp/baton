package keychain_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/codedogapp/baton/internal/keychain"
)

// The item that the tests read: baton's Jira token, for the user's account.
const (
	service = "baton-jira"
	account = "me@example.com"
)

// TestSecurityIsTheMacsOwn checks that baton runs the Mac's security tool by its full path, so
// that no other program on the PATH gets the item's names (decision 60).
func TestSecurityIsTheMacsOwn(t *testing.T) {
	if keychain.Security != "/usr/bin/security" {
		t.Errorf("Security = %s, want /usr/bin/security", keychain.Security)
	}
}

// standIn writes a stand-in for security that writes its arguments, one a line, to a file next to
// it, then runs the shell script. It returns the stand-in and the file.
func standIn(t *testing.T, text string) (keychain.Tool, string) {
	t.Helper()

	dir, executable := t.TempDir(), os.FileMode(0o700)
	program, args := filepath.Join(dir, "security"), filepath.Join(dir, "args")
	text = "#!/bin/sh\nprintf '%s\\n' \"$@\" > '" + args + "'\n" + text

	if err := os.WriteFile(program, []byte(text), executable); err != nil {
		t.Fatal(err)
	}

	return keychain.Tool(program), args
}

func TestPasswordReadsTheItem(t *testing.T) {
	tool, args := standIn(t, "printf ' a password \\n'\n")

	got, err := tool.Password(t.Context(), service, account)
	if err != nil || got != " a password " {
		t.Errorf("Password = %q, %v; want %q", got, err, " a password ")
	}

	want := "find-generic-password\n-s\nbaton-jira\n-a\nme@example.com\n-w\n"
	if data, err := os.ReadFile(filepath.Clean(args)); err != nil || string(data) != want {
		t.Errorf("security got the arguments %q (%v); want %q", data, err, want)
	}
}

func TestPasswordFindsNoItem(t *testing.T) {
	tool, _ := standIn(t, "echo 'security: SecKeychainSearchCopyNext: The specified item could "+
		"not be found in the keychain.' >&2\nexit 44\n")

	got, err := tool.Password(t.Context(), service, account)
	if !errors.Is(err, keychain.ErrNotFound) || got != "" {
		t.Errorf("Password = %q, %v; want %v", got, err, keychain.ErrNotFound)
	}
}

func TestPasswordFails(t *testing.T) {
	failing, _ := standIn(t, "printf 'half'\necho ' security: Interaction is not allowed. ' >&2\n"+
		"exit 36\n")
	missing := filepath.Join(t.TempDir(), "security")

	cases := map[string]struct {
		tool keychain.Tool
		want string
		exit bool // whether the error carries security's exit, for a caller that tells them apart
	}{
		"when security fails": {
			tool: failing, exit: true,
			want: "read the keychain: exit status 36: security: Interaction is not allowed.",
		},
		"without security": {
			tool: keychain.Tool(missing),
			want: "read the keychain: fork/exec " + missing + ": no such file or directory",
		},
	}

	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := test.tool.Password(t.Context(), service, account)
			if err == nil || err.Error() != test.want || got != "" {
				t.Errorf("Password = %q, %v; want the error %q", got, err, test.want)
			}

			if errors.Is(err, keychain.ErrNotFound) {
				t.Errorf("Password says %v, which isn't ErrNotFound", err)
			}

			if _, exited := errors.AsType[*exec.ExitError](err); exited != test.exit {
				t.Errorf("Password's error carries security's exit: %t, want %t", exited, test.exit)
			}
		})
	}
}

func TestPasswordStopsWithItsContext(t *testing.T) {
	stopped, cancel := context.WithCancel(t.Context())
	cancel()

	tool, _ := standIn(t, "printf 'a password\\n'\n")

	got, err := tool.Password(stopped, service, account)
	if want := "read the keychain: context canceled"; !errors.Is(err, context.Canceled) ||
		err.Error() != want || got != "" {
		t.Errorf("Password = %q, %v; want the error %q", got, err, want)
	}
}
