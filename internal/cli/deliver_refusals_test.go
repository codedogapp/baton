package cli_test

import (
	"fmt"
	"maps"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codedogapp/baton/internal/flow"
	"github.com/codedogapp/baton/internal/state"
)

// TestDeliverRefusesAndChangesNothing checks that a refusal pushes nothing, runs no gh, and leaves
// baton's folder as it was.
func TestDeliverRefusesAndChangesNothing(t *testing.T) {
	ready := readyToDeliver(t)

	cases := []struct {
		name  string
		args  []string          // After baton deliver.
		env   map[string]string // The pane's environment.
		setup func(t *testing.T)
		want  string // The reason, after "baton deliver: ".
	}{
		{
			name: "without a slug", args: []string{"--title", "Add the demo"},
			want: "want the feature's slug: baton deliver <slug> [--title <title>]",
		},
		{
			name: "with an argument after the slug", args: []string{slug, "now"},
			want: `unexpected argument "now"`,
		},
		{
			name: "in a role's pane", args: []string{slug, "--title", "Add it"},
			env:  paneOf(flow.RoleLead),
			want: "only the orchestrator delivers a feature, and BATON_ROLE names the lead",
		},
		{
			name: "for a feature that isn't there", args: []string{"demo-9", "--title", "Add it"},
			want: "no feature demo-9",
		},
		{
			name: "for a feature that runs", args: []string{"demo-2", "--title", "Add it"},
			setup: func(t *testing.T) {
				add(t, ready.store, state.Feature{Slug: "demo-2", Branch: "linked", Flow: fresh()})
			},
			want: "demo-2: a feature that is running isn't ready to deliver: it is once the " +
				"lead's final report comes",
		},
		{
			name: "without a title", args: []string{slug, "--title", " "},
			want: "want the pull request's title for the first delivery of demo-1: " +
				"baton deliver demo-1 --title <title>",
		},
		{
			name: "with a title for a later delivery", args: []string{slug, "--title", "Add it"},
			setup: func(t *testing.T) {
				edit(t, ready.store, func(feature *state.Feature) {
					feature.PullRequest = pullRequest
				})
			},
			want: "demo-1 has its pull request already, " + pullRequest + ", which keeps its " +
				"title: deliver it again without --title",
		},
		{
			name: "once delivered", args: []string{slug},
			setup: func(t *testing.T) {
				edit(t, ready.store, func(feature *state.Feature) {
					feature.Flow.Status = flow.StatusDelivered
				})
			},
			want: "demo-1: a feature that is delivered isn't ready to deliver: it is once the " +
				"lead's final report comes",
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if test.setup != nil {
				test.setup(t)
			}

			calls, before := standInGh(t, "echo "+pullRequest+"\n"), snapshot(t, ready.store.Dir)

			got := run(t, ready.mainDir, test.env, append([]string{"deliver"}, test.args...)...)
			if want := (result{code: 1, stderr: "baton deliver: " + test.want + "\n"}); got != want {
				t.Errorf("got %+v; want %+v", got, want)
			}

			wantPushed(t, ready.origin, "")
			wantGh(t, calls, "")

			if after := snapshot(t, ready.store.Dir); !maps.Equal(after, before) {
				t.Errorf("baton's folder changed:\n%v\nwant:\n%v", after, before)
			}
		})
	}
}

// TestDeliverRefusesOutsideARepo refuses a delivery that has no git repo, and so no feature.
func TestDeliverRefusesOutsideARepo(t *testing.T) {
	outside := t.TempDir()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(outside))
	calls := standInGh(t, "")

	got := run(t, outside, orchestratorPane(), "deliver", slug, "--title", "Add it")

	want := "baton deliver: find the git repo: fatal: not a git repository"
	if got.code != 1 || got.stdout != "" || !strings.HasPrefix(got.stderr, want) {
		t.Errorf("got %+v; want exit code 1 and %q", got, want)
	}

	wantGh(t, calls, "")
}

// TestDeliverReportsWhatFails checks that a delivery that fails leaves the feature ready, without
// a pull request, so that a later delivery tries again, and says what it did.
func TestDeliverReportsWhatFails(t *testing.T) {
	cases := []struct {
		name   string
		gh     string // The stand-in gh's script.
		pushed bool   // Whether origin has the branch afterwards.
		ran    bool   // Whether gh ran.
		want   string // The reason's start, after "baton deliver: ".
		// setup changes the repo first, if it isn't nil.
		setup func(t *testing.T, ready delivery)
	}{
		{
			name: "a push that git refuses", gh: "echo " + pullRequest + "\n",
			setup: func(t *testing.T, ready delivery) {
				gitIn(t, ready.mainDir, exec.CommandContext(t.Context(), "git", "remote", "remove",
					"origin"))
			},
			want: "push linked to origin: fatal: 'origin' does not appear to be a git repository\n",
		},
		{
			name: "a gh that fails", gh: "echo 'HTTP 401: Bad credentials' >&2\nexit 1\n",
			pushed: true, ran: true,
			want: "open the pull request: HTTP 401: Bad credentials (exit status 1)\n",
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			ready := readyToDeliver(t)
			if test.setup != nil {
				test.setup(t, ready)
			}

			calls := standInGh(t, test.gh)

			got := run(t, ready.mainDir, orchestratorPane(), "deliver", slug, "--title", "Add it")
			if got.code != 1 || got.stdout != "" ||
				!strings.HasPrefix(got.stderr, "baton deliver: "+test.want) {
				t.Errorf("got %+v; want exit code 1 and a reason that starts with %q", got,
					test.want)
			}

			wantPushed(t, ready.origin, map[bool]string{true: head(t, ready.mainDir)}[test.pushed])
			wantGh(t, calls, map[bool]string{true: "pr\ncreate\n--head=linked\n--title=Add it\n" +
				"--body-file=" + ready.report + "\n"}[test.ran])

			if feature, err := ready.store.Read(slug); err != nil ||
				feature.Flow.Status != flow.StatusReady || feature.PullRequest != "" {
				t.Errorf("Read = %+v, %v; want it ready, without a pull request", feature, err)
			}
		})
	}
}

// TestDeliverReportsAPullRequestThatItCannotRecord has the feature change while gh opens its pull
// request, as if the lead held it again: the delivery then names the pull request, which the
// feature doesn't keep.
func TestDeliverReportsAPullRequestThatItCannotRecord(t *testing.T) {
	ready := readyToDeliver(t)
	file := filepath.Join(ready.store.Dir, "features", slug, "feature.json")
	edits := `-e 's/"holder": "orchestrator"/"holder": "lead"/' ` +
		`-e 's/"status": "ready"/"status": "running"/'`
	standInGh(t, fmt.Sprintf("sed -i.old %s %q\necho %s\n", edits, file, pullRequest))

	got := run(t, ready.mainDir, orchestratorPane(), "deliver", slug, "--title", "Add it")
	if want := (result{code: 1, stderr: "baton deliver: record the pull request " + pullRequest +
		": demo-1: a feature that is running isn't ready to deliver: it is once the lead's " +
		"final report comes\n"}); got != want {
		t.Errorf("got %+v; want %+v", got, want)
	}

	if feature, err := ready.store.Read(slug); err != nil || feature.PullRequest != "" {
		t.Errorf("Read = %+v, %v; want it without a pull request", feature, err)
	}
}
