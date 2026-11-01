package cli_test

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/codedogapp/baton/internal/flow"
	"github.com/codedogapp/baton/internal/state"
)

// The ask tests run in synctest's bubbles, as the wait tests do (wait_test.go).

// The lead's question in the tests, what its ask prints once the orchestrator answers it, and what
// the ask and the orchestrator's wait print when they stop waiting.
const (
	question    = "Which file?"
	answered    = "The orchestrator's answer:\ndemo.txt\n"
	stopped     = "baton ask: stopped waiting: context deadline exceeded\n"
	waitStopped = "baton wait: stopped waiting: context deadline exceeded\n"
)

// TestAskGetsTheOrchestratorsAnswer has the lead ask from the linked worktree, and the
// orchestrator pick up the question, once, and answer it. The lead's ask prints the answer within
// a second, though the answer comes just after the ask last looked for it.
func TestAskGetsTheOrchestratorsAnswer(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		mainDir, linked := repos(t)
		store := started(t, mainDir, fresh())

		asked := make(chan result, 1)
		go func() { asked <- leadAsks(t, linked, question) }()

		synctest.Wait()

		got, want := runFor(t, mainDir, orchestratorPane(), "wait"), result{
			stdout: "Picked up the lead's question in demo-1:\nWhich file?\n" +
				"Answer it with baton answer demo-1 \"<answer>\".\n",
		}
		if got != want {
			t.Errorf("the orchestrator's wait: got %+v; want %+v", got, want)
		}

		got, want = orchestratorWaitsAMinute(t, mainDir), result{code: 1, stderr: waitStopped}
		if got != want {
			t.Errorf("the next wait: got %+v; want %+v", got, want)
		}

		synctest.Wait()

		select {
		case got := <-asked:
			t.Fatalf("the ask ended before its answer came: %+v", got)
		default:
		}

		answer(t, mainDir, "demo.txt")

		answeredAt := time.Now()

		if got := <-asked; got != (result{stdout: answered}) {
			t.Errorf("the lead's ask: got %+v; want the answer", got)
		}

		if waited := time.Since(answeredAt); waited > time.Second {
			t.Errorf("the lead got the answer %v after it came; want a second", waited)
		}

		wantQuestion(t, store, state.Question{
			From: flow.RoleLead, Text: question, Picked: true, Answer: "demo.txt",
		})
		wantJournal(t, store, []logged{
			{"start orchestrator  01-orchestrator-to-lead.md", "Build the demo.\n"},
			{"question lead  ", question},
			{"pickup orchestrator  ", ""},
			{"answer orchestrator  ", "demo.txt"},
		})
	})
}

// TestAskAgainGetsTheAnswer stops the lead's ask, as a crash would, then asks again: the lead
// can't hand over or ask another question meanwhile, and the same question waits for the answer
// that the first one waited for, without asking it again. Once the lead has the answer, the same
// question prints it at once, and another one is asked anew.
func TestAskAgainGetsTheAnswer(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		mainDir, _ := repos(t)
		store := started(t, mainDir, fresh())
		lead := paneOf(flow.RoleLead)
		refusals := []struct {
			got  result
			want string
		}{
			{askAMinute(t, mainDir, question), stopped},
			{
				run(t, mainDir, lead, "handover", "--note", write(t, mainDir, "note.md", "Done.")),
				"baton handover: the lead's question \"Which file?\" waits for its answer: " +
					"wait for it, then hand over\n",
			},
			{
				askAMinute(t, mainDir, "Which test?"),
				"baton ask: your question \"Which file?\" waits for its answer: " +
					"ask one question at a time\n",
			},
		}

		for _, refusal := range refusals {
			if want := (result{code: 1, stderr: refusal.want}); refusal.got != want {
				t.Errorf("got %+v; want %+v", refusal.got, want)
			}
		}

		asked := make(chan result, 1)
		go func() { asked <- leadAsks(t, mainDir, question) }()

		synctest.Wait()
		answer(t, mainDir, "demo.txt")

		for _, got := range []result{<-asked, leadAsks(t, mainDir, question)} {
			if got != (result{stdout: answered}) {
				t.Errorf("the lead's ask: got %+v; want the answer", got)
			}
		}

		if got := askAMinute(t, mainDir, "Which test?"); got != (result{code: 1, stderr: stopped}) {
			t.Errorf("the lead's next question: got %+v; want it to wait", got)
		}

		wantQuestion(t, store, state.Question{From: flow.RoleLead, Text: "Which test?"})
		wantJournal(t, store, []logged{
			{"start orchestrator  01-orchestrator-to-lead.md", "Build the demo.\n"},
			{"question lead  ", question},
			{"answer orchestrator  ", "demo.txt"},
			{"question lead  ", "Which test?"},
		})
	})
}

// TestAskGivesWayToTheNextQuestion has the lead's next question take the place of one whose
// answer came before its ask read it, as two asks at once could: the first ask fails, and the
// answer goes to the next one.
func TestAskGivesWayToTheNextQuestion(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		mainDir, _ := repos(t)
		started(t, mainDir, fresh())

		first, next := make(chan result, 1), make(chan result, 1)
		go func() { first <- leadAsks(t, mainDir, question) }()

		synctest.Wait()
		answer(t, mainDir, "demo.txt")

		go func() { next <- leadAsks(t, mainDir, "Which test?") }()

		synctest.Wait()

		want := result{code: 1, stderr: "baton ask: the lead's question \"Which test?\" " +
			"has taken the place of yours\n"}
		if got := <-first; got != want {
			t.Errorf("the first ask: got %+v; want %+v", got, want)
		}

		answer(t, mainDir, "demo_test.go")

		want = result{stdout: "The orchestrator's answer:\ndemo_test.go\n"}
		if got := <-next; got != want {
			t.Errorf("the next ask: got %+v; want %+v", got, want)
		}
	})
}

// TestWaitSkipsAnAnsweredQuestion has the orchestrator answer the lead's question before its wait
// picks the question up: the wait has no question to pick up then.
func TestWaitSkipsAnAnsweredQuestion(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		mainDir, _ := repos(t)
		store := started(t, mainDir, fresh())

		asked := make(chan result, 1)
		go func() { asked <- leadAsks(t, mainDir, question) }()

		synctest.Wait()
		answer(t, mainDir, "demo.txt")

		if got := <-asked; got != (result{stdout: answered}) {
			t.Errorf("the lead's ask: got %+v; want the answer", got)
		}

		want := result{code: 1, stderr: waitStopped}
		if got := orchestratorWaitsAMinute(t, mainDir); got != want {
			t.Errorf("the orchestrator's wait: got %+v; want %+v", got, want)
		}

		wantQuestion(t, store, state.Question{
			From: flow.RoleLead, Text: question, Answer: "demo.txt",
		})
	})
}

// askAMinute runs baton ask in the lead's pane, from the folder, in a bubble, and stops it after a
// minute of the bubble's time.
func askAMinute(t *testing.T, dir, text string) result {
	t.Helper()

	return askAMinuteAs(t, dir, flow.RoleLead, text)
}

// askAMinuteAs runs baton ask like askAMinute, in the role's pane.
func askAMinuteAs(t *testing.T, dir string, role flow.Role, text string) result {
	t.Helper()

	ctx, stop := context.WithTimeout(t.Context(), time.Minute)
	defer stop()

	return runUntil(ctx, dir, paneOf(role), "ask", text)
}

// orchestratorWaitsAMinute runs baton wait in the orchestrator's pane, from the folder, in a
// bubble, and stops it after a minute of the bubble's time.
func orchestratorWaitsAMinute(t *testing.T, dir string) result {
	t.Helper()

	ctx, stop := context.WithTimeout(t.Context(), time.Minute)
	defer stop()

	return runUntil(ctx, dir, orchestratorPane(), "wait")
}

// leadAsks runs baton ask in the lead's pane, from the folder, in a bubble.
func leadAsks(t *testing.T, dir, text string) result {
	t.Helper()

	return runFor(t, dir, paneOf(flow.RoleLead), "ask", text)
}

// answer answers the open question of demo-1 in the orchestrator's pane, from the folder.
func answer(t *testing.T, dir, text string) {
	t.Helper()

	got := run(t, dir, orchestratorPane(), "answer", slug, text)
	if want := (result{stdout: "Answered the lead's question in demo-1.\n"}); got != want {
		t.Fatalf("the orchestrator's answer: got %+v; want %+v", got, want)
	}
}

// wantQuestion checks the holder's last question in the feature's file.
func wantQuestion(t *testing.T, store state.Store, want state.Question) {
	t.Helper()

	feature, err := store.Read(slug)
	if err != nil {
		t.Fatal(err)
	}

	if feature.Question != want {
		t.Errorf("question: got %+v; want %+v", feature.Question, want)
	}
}
