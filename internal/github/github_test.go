package github_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/codedogapp/baton/internal/github"
)

// pullRequest is the URL of the pull request that the stand-in gh opens.
const pullRequest = "https://github.com/example/demo/pull/7"

// TestMain keeps the user's own gh out of the tests' reach: each test puts a stand-in on PATH.
func TestMain(m *testing.M) {
	if err := os.Setenv("PATH", ""); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

// standIn puts a stand-in gh on PATH, which runs the shell script after it writes its folder and
// its arguments, a line each, to the file that it returns.
func standIn(t *testing.T, text string) (calls string) {
	t.Helper()

	bin, executable := t.TempDir(), os.FileMode(0o700)
	calls = filepath.Join(bin, "calls")
	record := fmt.Sprintf("{ printf '%%s\\n' \"$PWD\"; for arg; do printf '%%s\\n' \"$arg\"; "+
		"done; } > %q\n", calls)

	err := os.WriteFile(filepath.Join(bin, "gh"), []byte("#!/bin/sh\n"+record+text), executable)
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", bin)

	return calls
}

// called returns the folder and the arguments that the stand-in gh recorded.
func called(t *testing.T, calls string) string {
	t.Helper()

	out, err := os.ReadFile(filepath.Clean(calls))
	if err != nil {
		t.Fatal(err)
	}

	return string(out)
}

func TestOpenOpensThePullRequestAndGivesItsURL(t *testing.T) {
	calls, dir := standIn(t, "echo "+pullRequest+"\n"), t.TempDir()

	got, err := github.Open(t.Context(), dir, github.PullRequest{
		Branch: "baton/demo", Title: "-Add a demo", BodyFile: "/notes/05-lead-to-orchestrator.md",
	})
	if err != nil || got != pullRequest {
		t.Errorf("Open = %q, %v; want %q", got, err, pullRequest)
	}

	want := dir + "\npr\ncreate\n--head=baton/demo\n--title=-Add a demo\n" +
		"--body-file=/notes/05-lead-to-orchestrator.md\n"
	if got := called(t, calls); got != want {
		t.Errorf("gh ran in and with:\n%s\nwant:\n%s", got, want)
	}
}

func TestOpenReportsWhyGhFailed(t *testing.T) {
	standIn(t, "echo 'a pull request for branch \"baton/demo\" into branch \"main\" already "+
		"exists:' >&2\necho "+pullRequest+" >&2\nexit 1\n")

	_, err := github.Open(t.Context(), t.TempDir(), github.PullRequest{Branch: "baton/demo"})

	want := "open the pull request: a pull request for branch \"baton/demo\" into branch \"main\" " +
		"already exists:\n" + pullRequest + " (exit status 1)"
	if err == nil || err.Error() != want {
		t.Errorf("Open = %v; want %q", err, want)
	}
}

func TestOpenReportsAMissingGh(t *testing.T) {
	_, err := github.Open(t.Context(), t.TempDir(), github.PullRequest{Branch: "baton/demo"})
	if want := `open the pull request: exec: "gh": executable file not found in $PATH`; err == nil ||
		err.Error() != want {
		t.Errorf("Open = %v; want %q", err, want)
	}
}

func TestCommentCommentsOnThePullRequest(t *testing.T) {
	calls, dir := standIn(t, "echo "+pullRequest+"#issuecomment-1\n"), t.TempDir()

	err := github.Comment(t.Context(), dir, pullRequest, "/notes/09.md")
	if err != nil {
		t.Fatal(err)
	}

	want := dir + "\npr\ncomment\n--body-file=/notes/09.md\n--\n" + pullRequest + "\n"
	if got := called(t, calls); got != want {
		t.Errorf("gh ran in and with:\n%s\nwant:\n%s", got, want)
	}
}

func TestCommentReportsAGhThatFailsWithoutAReason(t *testing.T) {
	standIn(t, "exit 4\n")

	err := github.Comment(t.Context(), t.TempDir(), pullRequest, "/x.md")
	if want := "comment on the pull request " + pullRequest + ": exit status 4"; err == nil ||
		err.Error() != want {
		t.Errorf("Comment = %v; want %q", err, want)
	}
}

func TestOpenTrimsWhatGhPrints(t *testing.T) {
	standIn(t, "printf '\\n https://github.com/example/demo/pull/8 \\n\\n'\n")

	got, err := github.Open(t.Context(), t.TempDir(), github.PullRequest{Branch: "baton/demo"})
	if want := "https://github.com/example/demo/pull/8"; err != nil || got != want {
		t.Errorf("Open = %q, %v; want %q", got, err, want)
	}
}
