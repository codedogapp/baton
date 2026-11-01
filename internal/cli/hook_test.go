package cli_test

import (
	"context"
	"encoding/json/v2"
	"maps"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/codedogapp/baton/internal/cli"
	"github.com/codedogapp/baton/internal/flow"
	"github.com/codedogapp/baton/internal/state"
)

// The reasons that baton hook stop sends a copilot back to work with (decision 47).
const (
	toWait = "No `baton wait` of yours is running, so no work can reach you. Start `baton wait` " +
		"with the bash tool in async mode, then end your turn: it ends, and wakes you, when your " +
		"next work comes, or at once if it's there already."
	toAskAgain = "Your question waits for its answer, but no `baton ask` of yours is running, so " +
		"the answer can't reach you. Ask it again, with the same text, with the bash tool in " +
		"async mode, then end your turn: baton doesn't ask it twice, and the ask ends, and wakes " +
		"you, when the answer comes. Your question:\n" + question
	toHandOver = "You hold the feature and haven't handed it over, and no `baton wait` of yours is " +
		"running. Finish your part and hand over with `baton handover`, then start `baton wait` " +
		"with the bash tool in async mode and end your turn. If you need an answer to go on, ask " +
		"the orchestrator with `baton ask` in async mode instead. If a command of yours runs in " +
		"the background, just end your turn: its end wakes you."
)

// TestStopHookSendsBackACopilotThatStopsTooSoon has the stop hook send back to work a pane's
// copilot that ends its turn while no baton wait or baton ask of its role waits: a role that work
// can come to, to wait for it, even while the holder's question waits; a holder whose question
// waits for its answer, to ask it again; and a holder at work, or one whose work a hand edit lost,
// to finish its part and hand over. The orchestrator waits while a role holds a feature or work
// waits for it, and may stop once it holds every feature with its work picked up, since those
// wait on the user. The hook changes nothing in baton's folder.
func TestStopHookSendsBackACopilotThatStopsTooSoon(t *testing.T) {
	ready := flow.State{Holder: flow.RoleLead, Status: flow.StatusRunning, Round: 2, Cap: 3}
	leadPicks := func(t *testing.T, _ state.Store, linked string) { waitAs(t, linked, flow.RoleLead) }
	leadAsks := func(t *testing.T, _ state.Store, linked string) {
		waitAs(t, linked, flow.RoleLead)
		askAMinute(t, linked, question)
	}
	readied := func(t *testing.T, _ state.Store, linked string) {
		handOver(t, linked, flow.RoleLead, flow.VerdictNone)
	}

	cases := []struct {
		name  string
		now   flow.State                                        // Where demo-1 stands.
		setup func(t *testing.T, store state.Store, dir string) // What ran before, if anything.
		env   map[string]string
		want  string // The reason, or "" to let the copilot stop.
	}{
		{
			name: "a role that work can come to",
			now:  fresh(),
			env:  paneOf(flow.RoleQA),
			want: toWait,
		},
		{
			name: "a role while the holder's question waits for its answer", now: fresh(),
			setup: leadAsks, env: paneOf(flow.RoleQA), want: toWait,
		},
		{
			name: "a holder with work that it hasn't picked up", now: fresh(),
			env: paneOf(flow.RoleLead), want: toWait,
		},
		{
			name: "a holder at work", now: fresh(), setup: leadPicks,
			env: paneOf(flow.RoleLead), want: toHandOver,
		},
		{
			name: "a holder whose work a hand edit lost", now: fresh(),
			setup: func(t *testing.T, store state.Store, _ string) {
				edit(t, store, func(feature *state.Feature) { feature.Work = state.Work{} })
			},
			env: paneOf(flow.RoleLead), want: toHandOver,
		},
		{
			name: "a holder whose question waits for its answer", now: fresh(),
			setup: leadAsks, env: paneOf(flow.RoleLead), want: toAskAgain,
		},
		{
			name: "a holder whose question has its answer", now: fresh(),
			setup: func(t *testing.T, store state.Store, linked string) {
				waitAs(t, linked, flow.RoleLead)
				edit(t, store, func(feature *state.Feature) {
					feature.Question = state.Question{
						From: flow.RoleLead, Text: question, Answer: "demo.txt",
					}
				})
			},
			env: paneOf(flow.RoleLead), want: toHandOver,
		},
		{
			name: "the orchestrator while a role holds a feature", now: fresh(),
			env: orchestratorPane(), want: toWait,
		},
		{
			name: "the orchestrator with work that it hasn't picked up", now: ready,
			setup: readied, env: orchestratorPane(), want: toWait,
		},
		{
			name: "the orchestrator with every feature's work picked up", now: ready,
			setup: func(t *testing.T, store state.Store, linked string) {
				readied(t, store, linked)
				runFor(t, linked, orchestratorPane(), "wait")
			},
			env: orchestratorPane(),
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				mainDir, linked := repos(t)
				store := started(t, mainDir, test.now)

				if test.setup != nil {
					test.setup(t, store, linked)
				}

				before := snapshot(t, store.Dir)
				wantStop(t, stopHook(t, linked, test.env, hookInput(t, linked, false)), test.want)

				if after := snapshot(t, store.Dir); !maps.Equal(after, before) {
					t.Errorf("baton's folder changed to %v", after)
				}
			})
		})
	}
}

// TestStopHookLetsAWaitingCopilotStop lets a pane's copilot stop while a baton wait or a baton ask
// of its role waits, since their end wakes it, and sends it back to work once they've stopped.
func TestStopHookLetsAWaitingCopilotStop(t *testing.T) {
	cases := []struct {
		name  string
		env   map[string]string
		args  []string
		after string // The reason once the command has stopped.
	}{
		{"a role's wait", paneOf(flow.RoleQA), []string{"wait"}, toWait},
		{"a holder's ask", paneOf(flow.RoleLead), []string{"ask", question}, toAskAgain},
		{"the orchestrator's wait", orchestratorPane(), []string{"wait"}, toWait},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				mainDir, linked := repos(t)
				started(t, mainDir, fresh())
				waitAs(t, linked, flow.RoleLead)

				ctx, stop := context.WithCancel(t.Context())
				waited := make(chan result, 1)

				go func() { waited <- runUntil(ctx, linked, test.env, test.args...) }()

				time.Sleep(time.Minute)
				wantStop(t, stopHook(t, linked, test.env, hookInput(t, linked, false)), "")
				stop()

				if got := <-waited; !strings.HasSuffix(got.stderr, ": stopped waiting: "+
					"context canceled\n") {
					t.Errorf("the %s: got %+v; want it stopped", test.name, got)
				}

				wantStop(t, stopHook(t, linked, test.env, hookInput(t, linked, false)), test.after)
			})
		})
	}
}

// stopHook runs baton hook stop in the pane, from the folder, with the hook's input.
func stopHook(t *testing.T, dir string, env map[string]string, input string) result {
	t.Helper()

	return runEnv(t.Context(), hookEnv(dir, env, input))
}

// hookEnv returns the environment of baton hook stop in the pane, from the folder, with the hook's
// input.
func hookEnv(dir string, env map[string]string, input string) cli.Env {
	return cli.Env{
		Args:   []string{"hook", "stop"},
		Getenv: func(name string) string { return env[name] },
		Dir:    dir,
		Stdin:  strings.NewReader(input),
	}
}

// hookInput returns the input that copilot gives its agentStop hook when a pane's copilot ends
// its turn in the folder, if any, and that says whether the hook sent the turn back already.
func hookInput(t *testing.T, dir string, active bool) string {
	t.Helper()

	input := map[string]any{
		"sessionId": "0b5d2c4e", "timestamp": 1767225600000, "transcriptPath": "/tmp/events.jsonl",
		"stopReason": "end_turn", "stop_hook_active": active,
	}
	if dir != "" {
		input["cwd"] = dir
	}

	data, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}

	return string(data)
}

// wantStop checks that baton hook stop let the copilot stop, without a word, if reason is "", and
// otherwise sent it back to work with the reason: a JSON object, on a line of its own.
func wantStop(t *testing.T, got result, reason string) {
	t.Helper()

	if reason == "" {
		if got != (result{}) {
			t.Errorf("got %+v; want exit code 0 and no output", got)
		}

		return
	}

	var answer struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
	}

	if got.code != 0 || got.stderr != "" || !strings.HasSuffix(got.stdout, "}\n") ||
		json.Unmarshal([]byte(got.stdout), &answer, json.RejectUnknownMembers(true)) != nil {
		t.Fatalf("got %+v; want exit code 0 and a JSON object", got)
	}

	if answer.Decision != "block" || answer.Reason != reason {
		t.Errorf("got %+v; want the decision block, with the reason %q", answer, reason)
	}
}
